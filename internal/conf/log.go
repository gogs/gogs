// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package conf

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	"gopkg.in/ini.v1"
	log "unknwon.dev/clog/v2"
)

type loggerConf struct {
	Buffer int64
	Config interface{}
}

type logConf struct {
	RootPath string
	Modes    []string
	Configs  []*loggerConf
}

// Log settings
var Log *logConf

// initLogConf returns parsed logging configuration from given INI file. When the
// argument "hookMode" is true, it only initializes the root path for log files.
// NOTE: Because we always create a console logger as the primary logger at init time,
// we need to remove it in case the user doesn't configure to use it after the logging
// service is initalized.
func initLogConf(cfg *ini.File, hookMode bool) (_ *logConf, hasConsole bool, _ error) {
	rootPath := cfg.Section("log").Key("ROOT_PATH").MustString(filepath.Join(WorkDir(), "log"))
	if hookMode {
		return &logConf{
			RootPath: ensureAbs(rootPath),
		}, false, nil
	}

	modes := strings.Split(cfg.Section("log").Key("MODE").MustString("console"), ",")
	lc := &logConf{
		RootPath: ensureAbs(rootPath),
		Modes:    make([]string, 0, len(modes)),
		Configs:  make([]*loggerConf, 0, len(modes)),
	}

	// Iterate over [log.*] sections to initialize individual logger.
	levelMappings := map[string]log.Level{
		"trace": log.LevelTrace,
		"info":  log.LevelInfo,
		"warn":  log.LevelWarn,
		"error": log.LevelError,
		"fatal": log.LevelFatal,
	}

	for i := range modes {
		modes[i] = strings.ToLower(strings.TrimSpace(modes[i]))
		secName := "log." + modes[i]
		sec, err := cfg.GetSection(secName)
		if err != nil {
			return nil, hasConsole, errors.Errorf("missing configuration section [%s] for %q logger", secName, modes[i])
		}

		level := levelMappings[strings.ToLower(sec.Key("LEVEL").MustString("trace"))]
		buffer := sec.Key("BUFFER_LEN").MustInt64(100)
		var c *loggerConf
		switch modes[i] {
		case log.DefaultConsoleName:
			hasConsole = true
			c = &loggerConf{
				Buffer: buffer,
				Config: log.ConsoleConfig{
					Level: level,
				},
			}

		case log.DefaultFileName:
			logPath := filepath.Join(lc.RootPath, "gogs.log")
			c = &loggerConf{
				Buffer: buffer,
				Config: log.FileConfig{
					Level:    level,
					Filename: logPath,
					FileRotationConfig: log.FileRotationConfig{
						Rotate:   sec.Key("LOG_ROTATE").MustBool(true),
						Daily:    sec.Key("DAILY_ROTATE").MustBool(true),
						MaxSize:  1 << uint(sec.Key("MAX_SIZE_SHIFT").MustInt(28)),
						MaxLines: sec.Key("MAX_LINES").MustInt64(1000000),
						MaxDays:  sec.Key("MAX_DAYS").MustInt64(7),
					},
				},
			}

		case log.DefaultSlackName:
			c = &loggerConf{
				Buffer: buffer,
				Config: log.SlackConfig{
					Level: level,
					URL:   sec.Key("URL").String(),
				},
			}

		case log.DefaultDiscordName:
			c = &loggerConf{
				Buffer: buffer,
				Config: log.DiscordConfig{
					Level:    level,
					URL:      sec.Key("URL").String(),
					Username: sec.Key("USERNAME").String(),
				},
			}

		default:
			continue
		}

		lc.Modes = append(lc.Modes, modes[i])
		lc.Configs = append(lc.Configs, c)
	}

	return lc, hasConsole, nil
}

// InitLogging initializes the logging service of the application. When the
// "hookMode" is true, it only initializes the root path for log files without
// creating any logger. It will also not remove the primary logger in "hookMode"
// and is up to the caller to decide when to remove it.
func InitLogging(hookMode bool) {
	logConf, hasConsole, err := initLogConf(File, hookMode)
	if err != nil {
		log.Fatal("Failed to init logging configuration: %v", err)
	}
	defer func() {
		Log = logConf
	}()

	if hookMode {
		return
	}

	err = os.MkdirAll(logConf.RootPath, os.ModePerm)
	if err != nil {
		log.Fatal("Failed to create log directory: %v", err)
	}

	for i, mode := range logConf.Modes {
		c := logConf.Configs[i]

		var err error
		var level log.Level
		switch mode {
		case log.DefaultConsoleName:
			level = c.Config.(log.ConsoleConfig).Level
			err = log.NewConsole(c.Buffer, c.Config)
		case log.DefaultFileName:
			level = c.Config.(log.FileConfig).Level
			err = log.NewFile(c.Buffer, c.Config)
		case log.DefaultSlackName:
			level = c.Config.(log.SlackConfig).Level
			err = log.NewSlack(c.Buffer, c.Config)
		case log.DefaultDiscordName:
			level = c.Config.(log.DiscordConfig).Level
			err = log.NewDiscord(c.Buffer, c.Config)
		default:
			panic("unreachable")
		}

		if err != nil {
			log.Fatal("Failed to init %s logger: %v", mode, err)
			return
		}
		log.Trace("Log mode: %s (%s)", strings.Title(mode), strings.Title(strings.ToLower(level.String())))
	}

	// ⚠️ WARNING: It is only safe to remove the primary logger until
	// there are other loggers that are initialized. Otherwise, the
	// application will print errors to nowhere.
	if !hasConsole {
		log.Remove(log.DefaultConsoleName)
	}
}
