package conf

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	charmlog "charm.land/log/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/cockroachdb/errors"
	"unknwon.dev/ini/v2"
	"unknwon.dev/x/logx"
)

// ModeConsole, ModeFile, ModeSlack, and ModeDiscord are the logging modes
// accepted by [log] MODE.
const (
	ModeConsole = "console"
	ModeFile    = "file"
	ModeSlack   = "slack"
	ModeDiscord = "discord"
)

// FileRotation is the rotation applied to a file sink.
type FileRotation struct {
	Rotate   bool
	Daily    bool
	MaxSize  int64
	MaxLines int64
	MaxDays  int64
}

// loggerConf is one configured sink. Buffer is the queue length of the Slack
// and Discord sinks. Console and file sinks write synchronously.
type loggerConf struct {
	Name     string
	Level    string
	Buffer   int64
	Filename string        `json:",omitempty"`
	URL      string        `json:",omitempty"`
	Username string        `json:",omitempty"`
	Rotation *FileRotation `json:",omitempty"`
}

type logConf struct {
	RootPath string
	Modes    []string
	Configs  []*loggerConf
}

// Log settings.
var Log *logConf

// initLogConf returns parsed logging configuration from the given INI file.
// When hookMode is true, it only initializes the root path for log files.
func initLogConf(cfg *ini.File, hookMode bool) (_ *logConf, hasConsole bool, _ error) {
	rootPath := cfg.Section("log").Key("ROOT_PATH").MustString(filepath.Join(WorkDir(), "log"))
	if hookMode {
		return &logConf{
			RootPath: ensureAbs(rootPath),
		}, false, nil
	}

	modes := strings.Split(cfg.Section("log").Key("MODE").MustString("console"), ",")
	parentLevel := cfg.Section("log").Key("LEVEL").MustString("trace")
	lc := &logConf{
		RootPath: ensureAbs(rootPath),
		Modes:    make([]string, 0, len(modes)),
		Configs:  make([]*loggerConf, 0, len(modes)),
	}

	for i := range modes {
		modes[i] = strings.ToLower(strings.TrimSpace(modes[i]))
		if slices.Contains(lc.Modes, modes[i]) {
			continue
		}
		secName := "log." + modes[i]
		sec, err := cfg.GetSection(secName)
		if err != nil {
			return nil, hasConsole, errors.Errorf("missing configuration section [%s] for %q logger", secName, modes[i])
		}

		c := &loggerConf{
			Name:   modes[i],
			Level:  strings.ToLower(sec.Key("LEVEL").MustString(parentLevel)),
			Buffer: max(sec.Key("BUFFER_LEN").MustInt64(cfg.Section("log").Key("BUFFER_LEN").MustInt64(100)), 0),
		}
		switch modes[i] {
		case ModeConsole:
			hasConsole = true
		case ModeFile:
			c.Filename = filepath.Join(lc.RootPath, "gogs.log")
			c.Rotation = &FileRotation{
				Rotate:   sec.Key("LOG_ROTATE").MustBool(true),
				Daily:    sec.Key("DAILY_ROTATE").MustBool(true),
				MaxSize:  1 << uint(sec.Key("MAX_SIZE_SHIFT").MustInt(28)),
				MaxLines: sec.Key("MAX_LINES").MustInt64(1000000),
				MaxDays:  sec.Key("MAX_DAYS").MustInt64(7),
			}
		case ModeSlack:
			c.URL = sec.Key("URL").String()
		case ModeDiscord:
			c.URL = sec.Key("URL").String()
			c.Username = sec.Key("USERNAME").String()
		default:
			continue
		}
		if (modes[i] == ModeSlack || modes[i] == ModeDiscord) && c.URL == "" {
			return nil, hasConsole, errors.Errorf("missing URL in configuration section [%s] for %q logger", secName, modes[i])
		}

		lc.Modes = append(lc.Modes, modes[i])
		lc.Configs = append(lc.Configs, c)
	}

	return lc, hasConsole, nil
}

// InitLogging builds the process logger from the loaded configuration. When
// hookMode is true, it only stores the root path and returns the given logger,
// so SSH clients do not receive server logs until the caller installs a hook
// file logger.
func InitLogging(logger *logx.Logger, hookMode bool) (*logx.Logger, error) {
	logConf, _, err := initLogConf(File, hookMode)
	if err != nil {
		return nil, errors.Wrap(err, "init logging configuration")
	}
	Log = logConf
	if hookMode {
		return logger, nil
	}

	if err := os.MkdirAll(logConf.RootPath, os.ModePerm); err != nil {
		return nil, errors.Wrap(err, "create log directory")
	}

	handlers := make([]slog.Handler, 0, len(logConf.Configs))
	for _, c := range logConf.Configs {
		h, err := newLogHandler(c)
		if err != nil {
			return nil, errors.Wrapf(err, "init %s logger", c.Name)
		}
		handlers = append(handlers, h)
	}
	if len(handlers) == 0 {
		handlers = append(handlers, newConsoleHandler(os.Stdout, slog.LevelDebug, IsProdMode()))
	}
	configured := newLogger(handlers...)
	for _, c := range logConf.Configs {
		configured.Debug("Log mode enabled", "mode", c.Name, "logLevel", c.Level)
	}
	return configured, nil
}

func newLogHandler(c *loggerConf) (slog.Handler, error) {
	level := parseLevel(c.Level)
	switch c.Name {
	case ModeConsole:
		return newConsoleHandler(os.Stdout, level, IsProdMode()), nil
	case ModeFile:
		w, err := newRotatingWriter(c.Filename, *c.Rotation)
		if err != nil {
			return nil, errors.Wrap(err, "create rotating writer")
		}
		return newCharmHandler(w, level, IsProdMode()), nil
	case ModeSlack:
		return newWebhookHandler(level, newSlackHandler(c.URL), c.Buffer), nil
	case ModeDiscord:
		return newWebhookHandler(level, newDiscordHandler(c.URL, c.Username), c.Buffer), nil
	}
	return nil, errors.Newf("unknown mode %q", c.Name)
}

// newLogger fans records out to every handler. Each handler applies its own
// level.
func newLogger(handlers ...slog.Handler) *logx.Logger {
	if len(handlers) == 1 {
		return logx.New(fatalLevelHandler{handlers[0]})
	}
	return logx.New(fatalLevelHandler{slog.NewMultiHandler(handlers...)})
}

// levelFatal is the level of records logged by logx.FatalContext, which charmlog
// shows as fatal.
const levelFatal = slog.LevelError + 4

// fatalFuncName is the function slog records as the caller of a record logged
// by logx.FatalContext.
var fatalFuncName = runtime.FuncForPC(reflect.ValueOf((*logx.Logger).FatalContext).Pointer()).Name()

// fatalLevelHandler raises records logged by logx.FatalContext, which logs at
// the error level because slog has no fatal level, to levelFatal. Sinks then
// label them as fatal, and sinks with LEVEL = Fatal receive only them.
type fatalLevelHandler struct {
	slog.Handler
}

func (h fatalLevelHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.Handler.Enabled(ctx, level) ||
		(level == slog.LevelError && h.Handler.Enabled(ctx, levelFatal))
}

func (h fatalLevelHandler) Handle(ctx context.Context, r slog.Record) error {
	if r.Level == slog.LevelError && runtime.FuncForPC(r.PC).Name() == fatalFuncName {
		r.Level = levelFatal
	}
	if !h.Handler.Enabled(ctx, r.Level) {
		return nil
	}
	return h.Handler.Handle(ctx, r)
}

func (h fatalLevelHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return fatalLevelHandler{h.Handler.WithAttrs(attrs)}
}

func (h fatalLevelHandler) WithGroup(name string) slog.Handler {
	return fatalLevelHandler{h.Handler.WithGroup(name)}
}

// newConsoleHandler returns a handler for a terminal. Process managers like moon
// are not a TTY, so TTY_FORCE opts back in to colors.
func newConsoleHandler(w io.Writer, level slog.Level, prod bool) slog.Handler {
	handler := newCharmLogger(w, level, prod)
	if os.Getenv("TTY_FORCE") != "" {
		handler.SetColorProfile(colorprofile.Detect(w, append(os.Environ(), "CLICOLOR_FORCE=1")))
	}
	return withComponentPrefix(handler, prod)
}

// newCharmHandler returns a handler that writes text, or JSON in production.
func newCharmHandler(w io.Writer, level slog.Level, prod bool) slog.Handler {
	return withComponentPrefix(newCharmLogger(w, level, prod), prod)
}

// newCharmLogger follows setupLogging in pgrok and git-lfs-server. Production
// output is JSON.
func newCharmLogger(w io.Writer, level slog.Level, prod bool) *charmlog.Logger {
	opts := charmlog.Options{
		TimeFormat:      time.DateTime,
		Level:           charmlog.Level(level),
		ReportTimestamp: true,
	}
	if prod {
		opts.Formatter = charmlog.JSONFormatter
		// Production logs are read by other tools, which need the time zone.
		opts.TimeFormat = time.RFC3339
	}
	handler := charmlog.NewWithOptions(w, opts)

	// Override warn level color to amber so it is less visually "green"-ish.
	styles := charmlog.DefaultStyles()
	styles.Levels[charmlog.WarnLevel] = lipgloss.NewStyle().
		SetString("WARN").
		Bold(true).
		Foreground(lipgloss.Color("226"))
	handler.SetStyles(styles)
	return handler
}

// parseLevel maps [log] LEVEL names to slog levels. "trace", the historical
// default, is debug.
func parseLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "info":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	case "fatal":
		return levelFatal
	default:
		return slog.LevelDebug
	}
}

// InitHookLogging returns a rotating file logger for an SSH hook command. It
// has no console sink, so the Git client does not see server logs.
func InitHookLogging(filename string) (*logx.Logger, error) {
	level := slog.LevelDebug
	if IsProdMode() {
		level = slog.LevelError
	}
	w, err := newRotatingWriter(filename, FileRotation{
		Rotate:  true,
		Daily:   true,
		MaxDays: 3,
	})
	if err != nil {
		return nil, errors.Wrap(err, "create rotating writer")
	}
	return newLogger(newCharmHandler(w, level, false)), nil
}

// NewConsoleLogger returns the logger a command uses before InitLogging builds
// the configured one. Commands whose stdout carries the Git protocol pass
// os.Stderr.
func NewConsoleLogger(w io.Writer) *logx.Logger {
	return newLogger(newConsoleHandler(w, slog.LevelDebug, false))
}

// withComponentPrefix shows the component of logx.Logger.Scoped as a "[gorm] "
// message prefix in text output, which people read. JSON output, which log
// explorers read, keeps it as a field to filter on.
func withComponentPrefix(h slog.Handler, json bool) slog.Handler {
	if json {
		return h
	}
	return &componentPrefixHandler{Handler: h}
}

type componentPrefixHandler struct {
	slog.Handler
	prefix string
}

func (h *componentPrefixHandler) Handle(ctx context.Context, r slog.Record) error {
	r.Message = h.prefix + r.Message
	return h.Handler.Handle(ctx, r)
}

func (h *componentPrefixHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	clone := *h
	if prefix, ok := componentPrefix(attrs); ok {
		clone.prefix = prefix
		attrs = slices.DeleteFunc(slices.Clone(attrs), isComponentAttr)
	}
	if len(attrs) > 0 {
		clone.Handler = h.Handler.WithAttrs(attrs)
	}
	return &clone
}

func (h *componentPrefixHandler) WithGroup(name string) slog.Handler {
	clone := *h
	clone.Handler = h.Handler.WithGroup(name)
	return &clone
}

// componentPrefix returns the message prefix for the component in attrs.
// logx.Logger.Scoped adds the component once, before any other attribute or
// group, so it never has to be found inside a group.
func componentPrefix(attrs []slog.Attr) (string, bool) {
	i := slices.IndexFunc(attrs, isComponentAttr)
	if i < 0 {
		return "", false
	}
	return "[" + attrs[i].Value.String() + "] ", true
}

func isComponentAttr(a slog.Attr) bool {
	return a.Key == logx.ComponentKey
}
