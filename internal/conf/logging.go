package conf

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/cockroachdb/errors"
	"unknwon.dev/ini/v2"

	"gogs.io/gogs/internal/logx"
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

// loggerConf is one configured sink. Buffer is kept so the admin page still
// shows the value from app.ini. It is not applied: charmlog writes synchronously.
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
		secName := "log." + modes[i]
		sec, err := cfg.GetSection(secName)
		if err != nil {
			return nil, hasConsole, errors.Errorf("missing configuration section [%s] for %q logger", secName, modes[i])
		}

		c := &loggerConf{
			Name:   modes[i],
			Level:  strings.ToLower(sec.Key("LEVEL").MustString(parentLevel)),
			Buffer: sec.Key("BUFFER_LEN").MustInt64(cfg.Section("log").Key("BUFFER_LEN").MustInt64(100)),
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

	modes := make([]logx.Mode, 0, len(logConf.Modes))
	for _, c := range logConf.Configs {
		mode, err := modeFromConf(c)
		if err != nil {
			return nil, errors.Wrapf(err, "init %s logger", c.Name)
		}
		modes = append(modes, mode)
		logger.Tracef("Log mode: %s (%s)", strings.Title(c.Name), strings.Title(c.Level))
	}

	logger, err = logx.NewFromOptions(logx.Options{
		Prod:  IsProdMode(),
		Level: logx.ParseLevel(File.Section("log").Key("LEVEL").MustString("trace")),
		Modes: modes,
	})
	if err != nil {
		return nil, errors.Wrap(err, "init logging")
	}
	return logger, nil
}

func modeFromConf(c *loggerConf) (logx.Mode, error) {
	mode := logx.Mode{
		Name:     c.Name,
		Level:    logx.ParseLevel(c.Level),
		URL:      c.URL,
		Username: c.Username,
	}
	if c.Filename == "" {
		if c.Name == ModeConsole {
			mode.Writer = os.Stderr
			mode.Color = true
		}
		return mode, nil
	}

	w, err := logx.NewRotatingWriter(c.Filename, logx.RotationConfig{
		Rotate:   c.Rotation.Rotate,
		Daily:    c.Rotation.Daily,
		MaxSize:  c.Rotation.MaxSize,
		MaxLines: c.Rotation.MaxLines,
		MaxDays:  c.Rotation.MaxDays,
	})
	if err != nil {
		return mode, err
	}
	mode.Writer = w
	return mode, nil
}

// InitHookLogging returns a rotating file logger for an SSH hook command. It
// has no console sink, so the Git client does not see server logs.
func InitHookLogging(filename string) (*logx.Logger, error) {
	level := logx.LevelTrace
	if IsProdMode() {
		level = slog.LevelError
	}
	w, err := logx.NewRotatingWriter(filename, logx.RotationConfig{
		Rotate:  true,
		Daily:   true,
		MaxDays: 3,
	})
	if err != nil {
		return nil, errors.Wrap(err, "create rotating writer")
	}
	return logx.NewFromOptions(logx.Options{
		Prod: false,
		Modes: []logx.Mode{{
			Name:   ModeFile,
			Level:  level,
			Writer: w,
		}},
	})
}

// NewConsoleLogger returns the logger a command uses before InitLogging builds
// the configured one.
func NewConsoleLogger() *logx.Logger {
	return logx.NewWriter(os.Stderr, slog.LevelDebug, false, os.Getenv("TTY_FORCE") != "")
}
