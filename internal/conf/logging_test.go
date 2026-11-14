package conf

import (
	"bytes"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"unknwon.dev/ini/v2"
)

func Test_initLogConf(t *testing.T) {
	t.Run("missing configuration section", func(t *testing.T) {
		f, err := ini.Load([]byte(`
[log]
MODE = console
`))
		require.NoError(t, err)

		got, hasConsole, err := initLogConf(f, false)
		assert.EqualError(t, err, `missing configuration section [log.console] for "console" logger`)
		assert.False(t, hasConsole)
		assert.Nil(t, got)
	})

	t.Run("no console logger", func(t *testing.T) {
		f, err := ini.Load([]byte(`
[log]
MODE = file

[log.file]
`))
		require.NoError(t, err)

		got, hasConsole, err := initLogConf(f, false)
		require.NoError(t, err)

		assert.False(t, hasConsole)
		assert.NotNil(t, got)
	})

	t.Run("child level inherits the parent", func(t *testing.T) {
		f, err := ini.Load([]byte(`
[log]
MODE = console
LEVEL = error

[log.console]
`))
		require.NoError(t, err)

		got, hasConsole, err := initLogConf(f, false)
		require.NoError(t, err)

		assert.True(t, hasConsole)
		assert.Equal(t, "error", got.Configs[0].Level)
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
	require.NoError(t, err)

	got, hasConsole, err := initLogConf(f, false)
	require.NoError(t, err)

	want := &logConf{
		RootPath: filepath.Join(WorkDir(), "log"),
		Modes:    []string{ModeConsole, ModeFile, ModeSlack, ModeDiscord},
		Configs: []*loggerConf{
			{
				Name:   ModeConsole,
				Level:  "trace",
				Buffer: 10,
			},
			{
				Name:     ModeFile,
				Level:    "info",
				Buffer:   50,
				Filename: filepath.Join(WorkDir(), "log", "gogs.log"),
				Rotation: &FileRotation{
					Rotate:   true,
					Daily:    true,
					MaxSize:  1 << 20,
					MaxLines: 1000,
					MaxDays:  3,
				},
			},
			{
				Name:   ModeSlack,
				Level:  "warn",
				Buffer: 50,
				URL:    "https://slack.com",
			},
			{
				Name:     ModeDiscord,
				Level:    "error",
				Buffer:   50,
				URL:      "https://discordapp.com",
				Username: "yoyo",
			},
		},
	}
	assert.True(t, hasConsole)
	assert.Equal(t, want, got)
}

func Test_parseLevel(t *testing.T) {
	tests := []struct {
		in   string
		want slog.Level
	}{
		{"trace", slog.LevelDebug},
		{"", slog.LevelDebug},
		{"Info", slog.LevelInfo},
		{"warning", slog.LevelWarn},
		{"error", slog.LevelError},
		{"fatal", slog.LevelError + 4},
	}
	for _, test := range tests {
		assert.Equal(t, test.want, parseLevel(test.in), test.in)
	}
}

func Test_newLogger(t *testing.T) {
	t.Run("each handler applies its own level", func(t *testing.T) {
		var console, file bytes.Buffer
		logger := newLogger(
			newCharmHandler(&console, slog.LevelInfo, false),
			newCharmHandler(&file, slog.LevelDebug, false),
		)

		logger.Debug("hidden from console")
		logger.Info("visible everywhere", "userID", 1)

		assert.NotContains(t, console.String(), "hidden from console")
		assert.Contains(t, console.String(), "visible everywhere userID=1")
		assert.Contains(t, file.String(), "hidden from console")
		assert.Contains(t, file.String(), "visible everywhere userID=1")
	})

	t.Run("production writes JSON", func(t *testing.T) {
		var buf bytes.Buffer
		logger := newLogger(newCharmHandler(&buf, slog.LevelInfo, true))

		logger.Info("started")

		line := strings.TrimSpace(buf.String())
		assert.True(t, strings.HasPrefix(line, "{"), line)
		assert.Contains(t, line, `"msg":"started"`)
	})
}
