package logx

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/cockroachdb/errors"
)

const dateFormat = "2006-01-02"

// RotationConfig controls when a log file is renamed and when old files are
// deleted. A zero value for a limit disables that limit. Rotate must be true
// for any limit to take effect.
type RotationConfig struct {
	Rotate   bool
	Daily    bool
	MaxSize  int64
	MaxLines int64
	MaxDays  int64
}

// rotatingWriter appends to a file and rotates it by day, size, and line count.
type rotatingWriter struct {
	filename string
	config   RotationConfig

	mu           sync.Mutex
	file         *os.File
	openDay      int
	currentSize  int64
	currentLines int64
}

// NewRotatingWriter opens filename for append and applies cfg.
func NewRotatingWriter(filename string, cfg RotationConfig) (io.Writer, error) {
	w := &rotatingWriter{
		filename: filename,
		config:   cfg,
	}
	if err := w.init(); err != nil {
		return nil, err
	}
	return w, nil
}

func (w *rotatingWriter) init() error {
	if err := os.MkdirAll(filepath.Dir(w.filename), os.ModePerm); err != nil {
		return errors.Wrap(err, "create log directory")
	}
	if err := w.open(); err != nil {
		return err
	}
	if !w.config.Rotate {
		return nil
	}
	return w.initRotation()
}

func (w *rotatingWriter) open() error {
	file, err := os.OpenFile(w.filename, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o660)
	if err != nil {
		return errors.Wrapf(err, "open %q", w.filename)
	}
	w.file = file
	return nil
}

func (w *rotatingWriter) initRotation() error {
	fi, err := w.file.Stat()
	if err != nil {
		return errors.Wrap(err, "stat")
	}
	w.currentSize = fi.Size()

	if w.config.MaxLines > 0 && w.currentSize > 0 {
		data, err := os.ReadFile(w.filename)
		if err != nil {
			return errors.Wrapf(err, "read %q", w.filename)
		}
		w.currentLines = int64(bytes.Count(data, []byte("\n"))) + 1
	}

	if w.config.Daily {
		now := time.Now()
		w.openDay = now.Day()
		last := fi.ModTime()
		if last.Year() != now.Year() || last.Month() != now.Month() || last.Day() != now.Day() {
			if err := w.rotate(last); err != nil {
				return err
			}
		}
	}
	if w.config.MaxDays > 0 {
		if err := w.deleteOutdated(); err != nil {
			return errors.Wrap(err, "delete outdated files")
		}
	}
	return nil
}

func (w *rotatingWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	n, err := w.file.Write(p)
	if err != nil || !w.config.Rotate {
		return n, err
	}

	w.currentSize += int64(n)
	w.currentLines += int64(bytes.Count(p, []byte("\n")))
	if !bytes.HasSuffix(p, []byte("\n")) {
		w.currentLines++
	}

	now := time.Now()
	var rotateDate time.Time
	switch {
	case w.config.Daily && now.Day() != w.openDay:
		rotateDate = now.Add(-24 * time.Hour)
	case (w.config.MaxSize > 0 && w.currentSize >= w.config.MaxSize) ||
		(w.config.MaxLines > 0 && w.currentLines >= w.config.MaxLines):
		rotateDate = now
	default:
		return n, err
	}

	if rerr := w.rotate(rotateDate); rerr != nil {
		return n, rerr
	}
	w.openDay = now.Day()
	w.currentSize = 0
	w.currentLines = 0
	if w.config.MaxDays > 0 {
		if rerr := w.deleteOutdated(); rerr != nil {
			return n, errors.Wrap(rerr, "delete outdated files")
		}
	}
	return n, nil
}

func (w *rotatingWriter) rotate(date time.Time) error {
	if err := w.file.Close(); err != nil {
		return errors.Wrap(err, "close current file")
	}
	dest := rotateFilename(w.filename, date.Format(dateFormat))
	if err := os.Rename(w.filename, dest); err != nil {
		return errors.Wrapf(err, "rename %q", w.filename)
	}
	if err := w.open(); err != nil {
		return err
	}
	return nil
}

func (w *rotatingWriter) deleteOutdated() error {
	cutoff := time.Now().Add(-24 * time.Hour * time.Duration(w.config.MaxDays))
	base := filepath.Base(w.filename)
	return filepath.Walk(filepath.Dir(w.filename), func(path string, fi os.FileInfo, err error) error {
		if err != nil || fi == nil || fi.IsDir() {
			return err
		}
		if fi.ModTime().Before(cutoff) && strings.HasPrefix(filepath.Base(path), base) {
			return os.Remove(path)
		}
		return nil
	})
}

func rotateFilename(filename, date string) string {
	name := fmt.Sprintf("%s.%s", filename, date)
	if _, err := os.Stat(name); os.IsNotExist(err) {
		return name
	}
	for i := 1; i < 1000; i++ {
		candidate := fmt.Sprintf("%s.%03d", name, i)
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
	}
	return fmt.Sprintf("%s.%d", name, time.Now().UnixNano())
}
