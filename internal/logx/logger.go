// Package logx is the application logger. charm.land/log/v2 is the slog
// handler. unknwon.dev/x/logx is the logger type. Printf-style methods exist
// so existing call sites can move without changing every format string.
package logx

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"charm.land/lipgloss/v2"
	charmlog "charm.land/log/v2"
	"github.com/cockroachdb/errors"
	xlog "unknwon.dev/x/logx"
)

// Logger is the process logger. Message methods come from the embedded slog
// logger. The printf methods format first, then log the result as the message.
type Logger struct {
	*xlog.Logger
}

// New wraps a charm or slog handler.
func New(h slog.Handler) *Logger {
	return &Logger{Logger: xlog.New(h)}
}

// NewWriter builds a single-destination logger. color is only applied to text
// output. Production JSON ignores it.
func NewWriter(w io.Writer, level slog.Level, prod, color bool) *Logger {
	return New(charmHandler(w, level, prod, color))
}

// NewNoop returns a logger that discards every record. Tests use it to silence
// output without a terminal.
func NewNoop() *Logger {
	return &Logger{Logger: xlog.NewNoopLogger()}
}

// Scoped returns a child logger whose messages are prefixed with "[name] ".
func (l *Logger) Scoped(name string) *Logger {
	if l == nil || l.Logger == nil {
		return l
	}
	return &Logger{Logger: l.Logger.Scoped(name)}
}

// With returns a child logger that includes the given attributes.
func (l *Logger) With(args ...any) *Logger {
	return &Logger{Logger: l.Logger.With(args...)}
}

func (l *Logger) logf(ctx context.Context, level slog.Level, format string, args ...any) {
	if l == nil || l.Logger == nil || !l.Enabled(ctx, level) {
		return
	}
	msg := format
	if len(args) > 0 {
		msg = fmt.Sprintf(format, args...)
	}
	l.Log(ctx, level, strings.TrimRight(msg, "\n"))
}

// Tracef logs at the trace level, below debug.
func (l *Logger) Tracef(format string, args ...any) {
	l.logf(context.Background(), LevelTrace, format, args...)
}

// Debugf logs at the debug level.
func (l *Logger) Debugf(format string, args ...any) {
	l.logf(context.Background(), slog.LevelDebug, format, args...)
}

// Infof logs at the info level.
func (l *Logger) Infof(format string, args ...any) {
	l.logf(context.Background(), slog.LevelInfo, format, args...)
}

// Warnf logs at the warn level.
func (l *Logger) Warnf(format string, args ...any) {
	l.logf(context.Background(), slog.LevelWarn, format, args...)
}

// Errorf logs at the error level.
func (l *Logger) Errorf(format string, args ...any) {
	l.logf(context.Background(), slog.LevelError, format, args...)
}

// Fatalf logs at the error level and exits the process.
func (l *Logger) Fatalf(format string, args ...any) {
	l.logf(context.Background(), slog.LevelError, format, args...)
	os.Exit(1)
}

// FatalContext logs at the error level and exits the process.
func (l *Logger) FatalContext(ctx context.Context, msg string, args ...any) {
	l.Logger.FatalContext(ctx, msg, args...)
}

// LevelTrace sits below slog.LevelDebug so a debug-level handler still drops it.
const LevelTrace slog.Level = -8

// ParseLevel maps the app.ini level names. Unknown values are trace, which is
// the historical default.
func ParseLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	case "fatal":
		return slog.LevelError + 4
	case "trace", "":
		return LevelTrace
	default:
		return LevelTrace
	}
}

// Mode is one configured output.
type Mode struct {
	Name  string
	Level slog.Level
	// Writer is the console or file destination. Empty URL means this is not a webhook.
	Writer io.Writer
	// Color is set for the console handler only.
	Color bool
	// URL and Username configure a Slack or Discord webhook.
	URL      string
	Username string
}

// Options controls setup.
type Options struct {
	// Prod switches text output to JSON, matching pgrok and git-lfs-server.
	Prod bool
	// Level is the default when a mode does not set one.
	Level slog.Level
	Modes []Mode
}

// NewFromOptions builds the application logger. Each mode is a handler with its
// own level, and slog.MultiHandler fans a record out to all of them.
func NewFromOptions(opts Options) (*Logger, error) {
	if len(opts.Modes) == 0 {
		return New(charmHandler(os.Stderr, opts.Level, opts.Prod, true)), nil
	}

	handlers := make([]slog.Handler, 0, len(opts.Modes))
	for _, mode := range opts.Modes {
		level := mode.Level
		switch {
		case mode.URL != "":
			wh := &webhookHandler{level: level}
			if mode.Name == "slack" {
				wh.slack = newSlackHandler(mode.URL)
			} else {
				wh.discord = newDiscordHandler(mode.URL, mode.Username)
			}
			handlers = append(handlers, wh)
		default:
			w := mode.Writer
			if w == nil {
				w = os.Stderr
			}
			handlers = append(handlers, charmHandler(w, level, opts.Prod, mode.Color))
		}
	}
	if len(handlers) == 1 {
		return New(handlers[0]), nil
	}
	return New(slog.NewMultiHandler(handlers...)), nil
}

func charmHandler(w io.Writer, level slog.Level, prod, color bool) *charmlog.Logger {
	opts := charmlog.Options{
		TimeFormat:      timeDateTime,
		Level:           charmlog.Level(level),
		ReportTimestamp: true,
	}
	if prod {
		opts.Formatter = charmlog.JSONFormatter
	}
	handler := charmlog.NewWithOptions(w, opts)
	if color && !prod {
		styles := charmlog.DefaultStyles()
		styles.Levels[charmlog.WarnLevel] = lipgloss.NewStyle().
			SetString("WARN").
			Bold(true).
			Foreground(lipgloss.Color("226"))
		handler.SetStyles(styles)
	}
	return handler
}

// timeDateTime matches pgrok's time.DateTime without importing time here twice.
const timeDateTime = "2006-01-02 15:04:05"

// webhookHandler posts one formatted line to Slack or Discord.
type webhookHandler struct {
	level   slog.Level
	slack   *slackHandler
	discord *discordHandler
	attrs   []slog.Attr
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
	level := r.Level.String()
	var err error
	if h.slack != nil {
		err = h.slack.post(level, msg)
	} else {
		err = h.discord.post(level, msg)
	}
	if err != nil {
		return errors.Wrap(err, "post webhook")
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
