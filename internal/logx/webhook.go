package logx

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/cockroachdb/errors"
)

// slackColors follows the order Trace, Info, Warn, Error, Fatal.
var slackColors = []string{"", "#3aa3e3", "warning", "danger", "#ff0200"}

// discordTitles and discordColors follow the same order.
var (
	discordTitles = []string{"Trace", "Information", "Warning", "Error", "Fatal"}
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
