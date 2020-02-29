// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package conf

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"gopkg.in/ini.v1"
	log "unknwon.dev/clog/v2"
)

func Test_initLogConf(t *testing.T) {
	t.Run("missing configuration section", func(t *testing.T) {
		f, err := ini.Load([]byte(`
[log]
MODE = console
`))
		if err != nil {
			t.Fatal(err)
		}

		got, hasConsole, err := initLogConf(f)
		assert.NotNil(t, err)
		assert.Equal(t, `missing configuration section [log.console] for "console" logger`, err.Error())
		assert.False(t, hasConsole)
		assert.Nil(t, got)
	})

	t.Run("no console logger", func(t *testing.T) {
		f, err := ini.Load([]byte(`
[log]
MODE = file

[log.file]
`))
		if err != nil {
			t.Fatal(err)
		}

		got, hasConsole, err := initLogConf(f)
		if err != nil {
			t.Fatal(err)
		}

		assert.False(t, hasConsole)
		assert.NotNil(t, got)
	})

	f, err := ini.Load([]byte(`
[log]
ROOT_PATH = log
MODE = console, file, slack, discord
BUFFER_LEN = 50
LEVEL = trace

[log.console]
BUFFER_LEN = 10

[log.file]
LEVEL = INFO
LOG_ROTATE = true
DAILY_ROTATE = true
MAX_SIZE_SHIFT = 20
MAX_LINES = 1000
MAX_DAYS = 3

[log.slack]
LEVEL = Warn
URL = https://slack.com

[log.discord]
LEVEL = error
URL = https://discordapp.com
USERNAME = yoyo
`))
	if err != nil {
		t.Fatal(err)
	}

	got, hasConsole, err := initLogConf(f)
	if err != nil {
		t.Fatal(err)
	}

	logConf := &logConf{
		RootPath: filepath.Join(WorkDir(), "log"),
		Modes: []string{
			log.DefaultConsoleName,
			log.DefaultFileName,
			log.DefaultSlackName,
			log.DefaultDiscordName,
		},
		Configs: []*loggerConf{
			{
				Buffer: 10,
				Config: log.ConsoleConfig{
					Level: log.LevelTrace,
				},
			}, {
				Buffer: 50,
				Config: log.FileConfig{
					Level:    log.LevelInfo,
					Filename: filepath.Join(WorkDir(), "log", "gogs.log"),
					FileRotationConfig: log.FileRotationConfig{
						Rotate:   true,
						Daily:    true,
						MaxSize:  1 << 20,
						MaxLines: 1000,
						MaxDays:  3,
					},
				},
			}, {
				Buffer: 50,
				Config: log.SlackConfig{
					Level: log.LevelWarn,
					URL:   "https://slack.com",
				},
			}, {
				Buffer: 50,
				Config: log.DiscordConfig{
					Level:    log.LevelError,
					URL:      "https://discordapp.com",
					Username: "yoyo",
				},
			},
		},
	}
	assert.True(t, hasConsole)
	assert.Equal(t, logConf, got)
}
