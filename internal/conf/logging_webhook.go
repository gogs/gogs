package conf

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"reflect"
	"runtime"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"unknwon.dev/x/logx"
)

// slackColors follows the order Debug, Info, Warn, Error, Fatal.
var slackColors = []string{"", "#3aa3e3", "warning", "danger", "#ff0200"}

// discordTitles and discordColors follow the same order.
var (
	discordTitles = []string{"Debug", "Information", "Warning", "Error", "Fatal"}
	discordColors = []int{0, 3843043, 16761600, 13041721, 9440319}
)

func levelIndex(level string) int {
	switch level {
	case "INFO":
		return 1
	case "WARN":
		return 2
	case "ERRO", "ERROR":
		return 3
	case "FATA", "FATAL":
		return 4
	default:
		return 0
	}
}

type slackHandler struct {
	url    string
	client *http.Client
}

func newSlackHandler(url string) *slackHandler {
	return &slackHandler{
		url:    url,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

func (h *slackHandler) post(level, message string) error {
	payload := struct {
		Attachments []struct {
			Text  string `json:"text"`
			Color string `json:"color"`
		} `json:"attachments"`
	}{
		Attachments: []struct {
			Text  string `json:"text"`
			Color string `json:"color"`
		}{{
			Text:  message,
			Color: slackColors[levelIndex(level)],
		}},
	}
	return postJSON(h.client, h.url, payload)
}

type discordHandler struct {
	url      string
	username string
	client   *http.Client
}

func newDiscordHandler(url, username string) *discordHandler {
	return &discordHandler{
		url:      url,
		username: username,
		client:   &http.Client{Timeout: 10 * time.Second},
	}
}

func (h *discordHandler) post(level, message string) error {
	idx := levelIndex(level)
	payload := struct {
		Username string `json:"username,omitempty"`
		Embeds   []struct {
			Title       string `json:"title"`
			Description string `json:"description"`
			Timestamp   string `json:"timestamp"`
			Color       int    `json:"color"`
		} `json:"embeds"`
	}{
		Username: h.username,
		Embeds: []struct {
			Title       string `json:"title"`
			Description string `json:"description"`
			Timestamp   string `json:"timestamp"`
			Color       int    `json:"color"`
		}{{
			Title:       discordTitles[idx],
			Description: message,
			Timestamp:   time.Now().Format(time.RFC3339),
			Color:       discordColors[idx],
		}},
	}

	const retryTimes = 3
	for i := 1; i <= retryTimes; i++ {
		retryAfter, err := postDiscord(h.client, h.url, payload)
		if err != nil {
			return err
		}
		if retryAfter > 0 {
			time.Sleep(time.Duration(retryAfter) * time.Millisecond)
			continue
		}
		return nil
	}
	return errors.Newf("gave up after %d retries", retryTimes)
}

func postJSON(client *http.Client, url string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return errors.Wrap(err, "marshal payload")
	}
	resp, err := client.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return errors.Wrap(err, "HTTP request")
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		data, _ := io.ReadAll(resp.Body)
		return errors.Newf("non-success response status code %d with body: %s", resp.StatusCode, data)
	}
	return nil
}

func postDiscord(client *http.Client, url string, payload any) (int64, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return -1, errors.Wrap(err, "marshal payload")
	}
	resp, err := client.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return -1, errors.Wrap(err, "HTTP request")
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		var rateLimit struct {
			RetryAfter int64 `json:"retry_after"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&rateLimit); err != nil {
			return -1, errors.Wrap(err, "decode rate limit message")
		}
		return rateLimit.RetryAfter, nil
	}
	if resp.StatusCode/100 != 2 {
		data, _ := io.ReadAll(resp.Body)
		return -1, errors.Newf("non-success response status code %d with body: %s", resp.StatusCode, data)
	}
	return -1, nil
}

type webhookPoster interface {
	post(level, message string) error
}

type webhookMessage struct {
	level string
	text  string
	// posted is closed once the message is posted, when the logger waits for it.
	posted chan struct{}
}

// fatalFuncName is the function slog records as the caller of a record logged
// by logx.FatalContext, which exits the process right after logging it.
var fatalFuncName = runtime.FuncForPC(reflect.ValueOf((*logx.Logger).FatalContext).Pointer()).Name()

// fatalFlushTimeout bounds how long a fatal record delays the exit while the
// queue is posted.
const fatalFlushTimeout = 10 * time.Second

// webhookHandler queues one formatted line per record for Slack or Discord.
// A single goroutine posts them, so callers only wait when the queue is full,
// or for a fatal record until it and everything queued before it are posted.
type webhookHandler struct {
	level slog.Level
	queue chan<- webhookMessage
	attrs []slog.Attr
}

func newWebhookHandler(level slog.Level, poster webhookPoster, bufferLen int64) *webhookHandler {
	queue := make(chan webhookMessage, bufferLen)
	go func() {
		for m := range queue {
			if err := poster.post(m.level, m.text); err != nil {
				// Logging this through the logger could loop back into the webhook.
				fmt.Fprintf(os.Stderr, "Failed to post log message to webhook: %v\n", err)
			}
			if m.posted != nil {
				close(m.posted)
			}
		}
	}()
	return &webhookHandler{level: level, queue: queue}
}

func (h *webhookHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level
}

func (h *webhookHandler) Handle(_ context.Context, r slog.Record) error {
	msg := r.Message
	if h.attrs != nil || r.NumAttrs() > 0 {
		var b strings.Builder
		b.WriteString(msg)
		for _, a := range h.attrs {
			fmt.Fprintf(&b, " %s=%v", a.Key, a.Value.Any())
		}
		r.Attrs(func(a slog.Attr) bool {
			fmt.Fprintf(&b, " %s=%v", a.Key, a.Value.Any())
			return true
		})
		msg = b.String()
	}
	m := webhookMessage{level: r.Level.String(), text: msg}
	if runtime.FuncForPC(r.PC).Name() == fatalFuncName {
		m.posted = make(chan struct{})
	}
	h.queue <- m
	if m.posted != nil {
		select {
		case <-m.posted:
		case <-time.After(fatalFlushTimeout):
		}
	}
	return nil
}

func (h *webhookHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	clone := *h
	clone.attrs = append(append([]slog.Attr{}, h.attrs...), attrs...)
	return &clone
}

func (h *webhookHandler) WithGroup(string) slog.Handler {
	return h
}
