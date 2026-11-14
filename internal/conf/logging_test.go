package conf

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

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

	t.Run("duplicate modes", func(t *testing.T) {
		f, err := ini.Load([]byte(`
[log]
MODE = file, console, File

[log.file]
[log.console]
`))
		require.NoError(t, err)

		got, _, err := initLogConf(f, false)
		require.NoError(t, err)
		assert.Equal(t, []string{ModeFile, ModeConsole}, got.Modes)
		assert.Len(t, got.Configs, 2)
	})

	t.Run("negative buffer length", func(t *testing.T) {
		f, err := ini.Load([]byte(`
[log]
MODE = slack
BUFFER_LEN = -1

[log.slack]
URL = https://slack.com
`))
		require.NoError(t, err)

		got, _, err := initLogConf(f, false)
		require.NoError(t, err)
		assert.Equal(t, int64(0), got.Configs[0].Buffer)
	})

	t.Run("missing webhook URL", func(t *testing.T) {
		f, err := ini.Load([]byte(`
[log]
MODE = console, discord

[log.console]
[log.discord]
`))
		require.NoError(t, err)

		got, _, err := initLogConf(f, false)
		assert.EqualError(t, err, `missing URL in configuration section [log.discord] for "discord" logger`)
		assert.Nil(t, got)
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
		{"fatal", slog.LevelError},
	}
	for _, test := range tests {
		assert.Equal(t, test.want, parseLevel(test.in), test.in)
	}
}

func Test_rotatingWriter(t *testing.T) {
	t.Run("resumes after a failed rotation", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("Windows cannot remove a file that is still open")
		}

		filename := filepath.Join(t.TempDir(), "gogs.log")
		w, err := newRotatingWriter(filename, FileRotation{Rotate: true, MaxSize: 10})
		require.NoError(t, err)

		// Renaming a missing file fails, so the rotation triggered by this write fails.
		require.NoError(t, os.Remove(filename))
		_, err = w.Write([]byte("triggers rotation\n"))
		require.NoError(t, err)
		_, err = os.Stat(filename)
		require.True(t, os.IsNotExist(err), "rotation should have failed")

		_, err = w.Write([]byte("second\n"))
		require.NoError(t, err)
		_, err = w.Write([]byte("third\n"))
		require.NoError(t, err)

		rotated, err := os.ReadFile(filename + "." + time.Now().Format(dateFormat))
		require.NoError(t, err)
		assert.Equal(t, "second\n", string(rotated), "rotation is retried")

		current, err := os.ReadFile(filename)
		require.NoError(t, err)
		assert.Equal(t, "third\n", string(current))
	})

	t.Run("drops lines it cannot write", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("Windows cannot remove a directory with an open file")
		}

		dir := filepath.Join(t.TempDir(), "log")
		filename := filepath.Join(dir, "gogs.log")
		w, err := newRotatingWriter(filename, FileRotation{Rotate: true, MaxSize: 10})
		require.NoError(t, err)
		logger := newLogger(newCharmHandler(w, slog.LevelDebug, false))

		// The rotation fails and the log directory cannot be recreated, so no
		// file can be opened for the next line.
		require.NoError(t, os.RemoveAll(dir))
		require.NoError(t, os.WriteFile(dir, nil, 0o600))
		logger.Info("triggers rotation")
		logger.Info("dropped")

		require.NoError(t, os.Remove(dir))
		logger.Info("written")

		// The retried rotation may have moved the line to a rotated file.
		files, err := filepath.Glob(filename + "*")
		require.NoError(t, err)
		var logged strings.Builder
		for _, f := range files {
			data, err := os.ReadFile(f)
			require.NoError(t, err)
			logged.Write(data)
		}
		assert.NotContains(t, logged.String(), "dropped")
		assert.Contains(t, logged.String(), "written")
	})
}

type blockingPoster struct {
	release  chan struct{}
	received chan string
}

func (p *blockingPoster) post(level, message string) error {
	<-p.release
	p.received <- level + " " + message
	return nil
}

func Test_webhookHandler(t *testing.T) {
	t.Run("does not wait for the post", func(t *testing.T) {
		poster := &blockingPoster{
			release:  make(chan struct{}),
			received: make(chan string, 1),
		}
		logger := newLogger(newWebhookHandler(slog.LevelError, poster, 10))

		logged := make(chan struct{})
		go func() {
			logger.Info("below level")
			logger.Error("Failed to deliver", "userID", 1)
			close(logged)
		}()
		select {
		case <-logged:
		case <-time.After(5 * time.Second):
			t.Fatal("logging waited for the webhook post")
		}

		close(poster.release)
		assert.Equal(t, "ERROR Failed to deliver userID=1", <-poster.received)
	})

	t.Run("posts the queue before a fatal exit", func(t *testing.T) {
		// FatalContext exits the process, so it runs in a child test process.
		if url := os.Getenv("GOGS_TEST_WEBHOOK_URL"); url != "" {
			logger := newLogger(newWebhookHandler(slog.LevelError, newSlackHandler(url), 10))
			logger.Error("Queued before fatal")
			logger.FatalContext(context.Background(), "Fatal")
			return
		}

		var mu sync.Mutex
		var posted []string
		server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			// A slow webhook keeps messages queued when FatalContext is called.
			time.Sleep(200 * time.Millisecond)
			body, _ := io.ReadAll(r.Body)
			mu.Lock()
			posted = append(posted, string(body))
			mu.Unlock()
		}))
		defer server.Close()

		cmd := exec.Command(os.Args[0], "-test.run=^Test_webhookHandler$/^posts_the_queue_before_a_fatal_exit$")
		cmd.Env = append(os.Environ(), "GOGS_TEST_WEBHOOK_URL="+server.URL)
		err := cmd.Run()
		var exitErr *exec.ExitError
		require.ErrorAs(t, err, &exitErr)
		assert.Equal(t, 1, exitErr.ExitCode())

		mu.Lock()
		defer mu.Unlock()
		require.Len(t, posted, 2)
		assert.Contains(t, posted[0], "Queued before fatal")
		assert.Contains(t, posted[1], "Fatal")
	})
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

		var record struct{ Time string }
		require.NoError(t, json.Unmarshal([]byte(line), &record))
		_, err := time.Parse(time.RFC3339, record.Time)
		assert.NoError(t, err, "time %q has no time zone", record.Time)
	})
}
