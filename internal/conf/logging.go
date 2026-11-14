package conf

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	charmlog "charm.land/log/v2"
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

// InitLogging builds the process logger from the loaded configuration. The
// given logger reports progress. When hookMode is true, it only stores the root
// path and returns the given logger, so SSH clients do not receive server logs
// until the caller installs a hook file logger.
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
		logger.Debug("Log mode enabled", "mode", c.Name, "level", c.Level)
	}
	if len(handlers) == 0 {
		handlers = append(handlers, newCharmHandler(os.Stderr, slog.LevelDebug, IsProdMode()))
	}
	return newLogger(handlers...), nil
}

func newLogHandler(c *loggerConf) (slog.Handler, error) {
	level := parseLevel(c.Level)
	switch c.Name {
	case ModeConsole:
		return newCharmHandler(os.Stderr, level, IsProdMode()), nil
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
		return logx.New(handlers[0])
	}
	return logx.New(slog.NewMultiHandler(handlers...))
}

// newCharmHandler follows setupLogging in pgrok and git-lfs-server. Production
// output is JSON.
func newCharmHandler(w io.Writer, level slog.Level, prod bool) *charmlog.Logger {
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
		// logx.FatalContext logs at the error level before exiting.
		return slog.LevelError
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
// the configured one.
func NewConsoleLogger() *logx.Logger {
	return newLogger(newCharmHandler(os.Stderr, slog.LevelDebug, false))
}
