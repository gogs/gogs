// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package conf

import (
	"os"
	"path/filepath"
	"strings"

	log "unknwon.dev/clog/v2"
)

// Log settings
var Log struct {
	RootPath string
	Modes    []string
	Configs  []interface{}
}

// InitLogging initializes the logging service of the application.
func InitLogging() {
	Log.RootPath = File.Section("log").Key("ROOT_PATH").MustString(filepath.Join(WorkDir(), "log"))

	// Because we always create a console logger as the primary logger at init time,
	// we need to remove it in case the user doesn't configure to use it after the
	// logging service is initalized.
	hasConsole := false

	// Iterate over [log.*] sections to initialize individual logger.
	Log.Modes = strings.Split(File.Section("log").Key("MODE").MustString("console"), ",")
	Log.Configs = make([]interface{}, 0, len(Log.Modes))
	levelMappings := map[string]log.Level{
		"trace": log.LevelTrace,
		"info":  log.LevelInfo,
		"warn":  log.LevelWarn,
		"error": log.LevelError,
		"fatal": log.LevelFatal,
	}

	type config struct {
		Buffer int64
		Config interface{}
	}
	for _, mode := range Log.Modes {
		mode = strings.ToLower(strings.TrimSpace(mode))
		secName := "log." + mode
		sec, err := File.GetSection(secName)
		if err != nil {
			log.Fatal("Missing configuration section [%s] for %q logger", secName, mode)
			return
		}

		level := levelMappings[strings.ToLower(sec.Key("LEVEL").MustString("trace"))]
		buffer := sec.Key("BUFFER_LEN").MustInt64(100)
		var c *config
		switch mode {
		case log.DefaultConsoleName:
			hasConsole = true
			c = &config{
				Buffer: buffer,
				Config: log.ConsoleConfig{
					Level: level,
				},
			}
			err = log.NewConsole(c.Buffer, c.Config)

		case log.DefaultFileName:
			logPath := filepath.Join(Log.RootPath, "gogs.log")
			logDir := filepath.Dir(logPath)
			err = os.MkdirAll(logDir, os.ModePerm)
			if err != nil {
				log.Fatal("Failed to create log directory %q: %v", logDir, err)
				return
			}

			c = &config{
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
			err = log.NewFile(c.Buffer, c.Config)

		case log.DefaultSlackName:
			c = &config{
				Buffer: buffer,
				Config: log.SlackConfig{
					Level: level,
					URL:   sec.Key("URL").String(),
				},
			}
			err = log.NewSlack(c.Buffer, c.Config)

		case log.DefaultDiscordName:
			c = &config{
				Buffer: buffer,
				Config: log.DiscordConfig{
					Level:    level,
					URL:      sec.Key("URL").String(),
					Username: sec.Key("USERNAME").String(),
				},
			}
			err = log.NewDiscord(c.Buffer, c.Config)

		default:
			continue
		}

		if err != nil {
			log.Fatal("Failed to init %s logger: %v", mode, err)
			return
		}

		Log.Configs = append(Log.Configs, c)
		log.Trace("Log mode: %s (%s)", strings.Title(mode), strings.Title(strings.ToLower(level.String())))
	}

	if !hasConsole {
		log.Remove(log.DefaultConsoleName)
	}
}
