package logx

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseLevel(t *testing.T) {
	tests := []struct {
		in   string
		want slog.Level
	}{
		{"trace", LevelTrace},
		{"", LevelTrace},
		{"DEBUG", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"Warn", slog.LevelWarn},
		{"warning", slog.LevelWarn},
		{"error", slog.LevelError},
		{"nope", LevelTrace},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, ParseLevel(tt.in), tt.in)
	}
}

func TestLogger_PrintfMethods(t *testing.T) {
	var buf bytes.Buffer
	logger := NewWriter(&buf, LevelTrace, false, false)

	logger.Tracef("trace %d", 1)
	logger.Infof("hello %s", "world")
	logger.Errorf("boom: %v", assert.AnError)

	out := buf.String()
	assert.Contains(t, out, "TRAC trace 1")
	assert.Contains(t, out, "hello world")
	assert.Contains(t, out, "boom:")
}

func TestLogger_ScopedKeepsPrintf(t *testing.T) {
	var buf bytes.Buffer
	logger := NewWriter(&buf, LevelTrace, false, false).Scoped("hooks")

	logger.Errorf("failed: %s", "disk")

	assert.Contains(t, buf.String(), "[hooks] failed: disk")
}

func TestNewFromOptions_FansOut(t *testing.T) {
	var console, file bytes.Buffer
	logger, err := NewFromOptions(Options{
		Modes: []Mode{
			{Name: "console", Level: slog.LevelInfo, Writer: &console},
			{Name: "file", Level: LevelTrace, Writer: &file},
		},
	})
	require.NoError(t, err)

	logger.Tracef("hidden from console")
	logger.Infof("visible %s", "everywhere")

	assert.NotContains(t, console.String(), "hidden from console")
	assert.Contains(t, console.String(), "visible everywhere")
	assert.Contains(t, file.String(), "hidden from console")
	assert.Contains(t, file.String(), "visible everywhere")
}

func TestNewFromOptions_JSONInProd(t *testing.T) {
	var buf bytes.Buffer
	logger, err := NewFromOptions(Options{
		Prod: true,
		Modes: []Mode{
			{Name: "console", Level: slog.LevelInfo, Writer: &buf},
		},
	})
	require.NoError(t, err)

	logger.Infof("started")

	line := strings.TrimSpace(buf.String())
	assert.True(t, strings.HasPrefix(line, "{"), line)
	assert.Contains(t, line, `"msg":"started"`)
}
