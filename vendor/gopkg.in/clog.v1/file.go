// Copyright 2017 Unknwon
//
// Licensed under the Apache License, Version 2.0 (the "License"): you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
// WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
// License for the specific language governing permissions and limitations
// under the License.

package clog

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	SIMPLE_DATE_FORMAT = "2006-01-02"
	LOG_PREFIX_LENGTH  = len("2017/02/06 21:20:08 ")
)

// FileRotationConfig represents rotation related configurations for file mode logger.
// All the settings can take effect at the same time, remain zero values to disable them.
type FileRotationConfig struct {
	// Do rotation for output files.
	Rotate bool
	// Rotate on daily basis.
	Daily bool
	// Maximum size in bytes of file for a rotation.
	MaxSize int64
	// Maximum number of lines for a rotation.
	MaxLines int64
	// Maximum lifetime of a output file in days.
	MaxDays int64
}

type FileConfig struct {
	// Minimum level of messages to be processed.
	Level LEVEL
	// Buffer size defines how many messages can be queued before hangs.
	BufferSize int64
	// File name to outout messages.
	Filename string
	// Rotation related configurations.
	FileRotationConfig
}

type file struct {
	// Indicates whether object is been used in standalone mode.
	standalone bool

	*log.Logger
	Adapter

	file         *os.File
	filename     string
	openDay      int
	currentSize  int64
	currentLines int64
	rotate       FileRotationConfig
}

func newFile() Logger {
	return &file{
		Adapter: Adapter{
			quitChan: make(chan struct{}),
		},
	}
}

// NewFileWriter returns an io.Writer for synchronized file logger in standalone mode.
func NewFileWriter(filename string, cfg FileRotationConfig) (io.Writer, error) {
	f := &file{
		standalone: true,
	}
	if err := f.Init(FileConfig{
		Filename:           filename,
		FileRotationConfig: cfg,
	}); err != nil {
		return nil, err
	}

	return f, nil
}

func (f *file) Level() LEVEL { return f.level }

var newLineBytes = []byte("\n")

func (f *file) initFile() (err error) {
	f.file, err = os.OpenFile(f.filename, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0660)
	if err != nil {
		return fmt.Errorf("OpenFile '%s': %v", f.filename, err)
	}

	f.Logger = log.New(f.file, "", log.Ldate|log.Ltime)
	return nil
}

// isExist checks whether a file or directory exists.
// It returns false when the file or directory does not exist.
func isExist(path string) bool {
	_, err := os.Stat(path)
	return err == nil || os.IsExist(err)
}

// rotateFilename returns next available rotate filename with given date.
func (f *file) rotateFilename(date string) string {
	filename := fmt.Sprintf("%s.%s", f.filename, date)
	if !isExist(filename) {
		return filename
	}

	format := filename + ".%03d"
	for i := 1; i < 1000; i++ {
		filename := fmt.Sprintf(format, i)
		if !isExist(filename) {
			return filename
		}
	}

	panic("too many log files for yesterday")
}

func (f *file) deleteOutdatedFiles() {
	filepath.Walk(filepath.Dir(f.filename), func(path string, info os.FileInfo, err error) error {
		if !info.IsDir() &&
			info.ModTime().Before(time.Now().Add(-24*time.Hour*time.Duration(f.rotate.MaxDays))) &&
			strings.HasPrefix(filepath.Base(path), filepath.Base(f.filename)) {
			os.Remove(path)
		}
		return nil
	})
}

func (f *file) initRotate() error {
	// Gather basic file info for rotation.
	fi, err := f.file.Stat()
	if err != nil {
		return fmt.Errorf("Stat: %v", err)
	}

	f.currentSize = fi.Size()

	// If there is any content in the file, count the number of lines.
	if f.rotate.MaxLines > 0 && f.currentSize > 0 {
		data, err := ioutil.ReadFile(f.filename)
		if err != nil {
			return fmt.Errorf("ReadFile '%s': %v", f.filename, err)
		}

		f.currentLines = int64(bytes.Count(data, newLineBytes)) + 1
	}

	if f.rotate.Daily {
		now := time.Now()
		f.openDay = now.Day()

		lastWriteTime := fi.ModTime()
		if lastWriteTime.Year() != now.Year() ||
			lastWriteTime.Month() != now.Month() ||
			lastWriteTime.Day() != now.Day() {

			if err = f.file.Close(); err != nil {
				return fmt.Errorf("Close: %v", err)
			}
			if err = os.Rename(f.filename, f.rotateFilename(lastWriteTime.Format(SIMPLE_DATE_FORMAT))); err != nil {
				return fmt.Errorf("Rename: %v", err)
			}

			if err = f.initFile(); err != nil {
				return fmt.Errorf("initFile: %v", err)
			}
		}
	}

	if f.rotate.MaxDays > 0 {
		f.deleteOutdatedFiles()
	}
	return nil
}

func (f *file) Init(v interface{}) (err error) {
	cfg, ok := v.(FileConfig)
	if !ok {
		return ErrConfigObject{"FileConfig", v}
	}

	if !isValidLevel(cfg.Level) {
		return ErrInvalidLevel{}
	}
	f.level = cfg.Level

	f.filename = cfg.Filename
	os.MkdirAll(filepath.Dir(f.filename), os.ModePerm)
	if err = f.initFile(); err != nil {
		return fmt.Errorf("initFile: %v", err)
	}

	f.rotate = cfg.FileRotationConfig
	if f.rotate.Rotate {
		f.initRotate()
	}

	if !f.standalone {
		f.msgChan = make(chan *Message, cfg.BufferSize)
	}
	return nil
}

func (f *file) ExchangeChans(errorChan chan<- error) chan *Message {
	f.errorChan = errorChan
	return f.msgChan
}

func (f *file) write(msg *Message) int {
	f.Logger.Print(msg.Body)

	bytesWrote := len(msg.Body)
	if !f.standalone {
		bytesWrote += LOG_PREFIX_LENGTH
	}
	if f.rotate.Rotate {
		f.currentSize += int64(bytesWrote)
		f.currentLines++ // TODO: should I care if log message itself contains new lines?

		var (
			needsRotate = false
			rotateDate  time.Time
		)

		now := time.Now()
		if f.rotate.Daily && now.Day() != f.openDay {
			needsRotate = true
			rotateDate = now.Add(-24 * time.Hour)

		} else if (f.rotate.MaxSize > 0 && f.currentSize >= f.rotate.MaxSize) ||
			(f.rotate.MaxLines > 0 && f.currentLines >= f.rotate.MaxLines) {
			needsRotate = true
			rotateDate = now
		}

		if needsRotate {
			f.file.Close()
			if err := os.Rename(f.filename, f.rotateFilename(rotateDate.Format(SIMPLE_DATE_FORMAT))); err != nil {
				f.errorChan <- fmt.Errorf("fail to rename rotate file '%s': %v", f.filename, err)
			}

			if err := f.initFile(); err != nil {
				f.errorChan <- fmt.Errorf("fail to init log file '%s': %v", f.filename, err)
			}

			f.openDay = now.Day()
			f.currentSize = 0
			f.currentLines = 0
		}
	}
	return bytesWrote
}

var _ io.Writer = new(file)

// Write implements method of io.Writer interface.
func (f *file) Write(p []byte) (int, error) {
	return f.write(&Message{
		Body: string(p),
	}), nil
}

func (f *file) Start() {
LOOP:
	for {
		select {
		case msg := <-f.msgChan:
			f.write(msg)
		case <-f.quitChan:
			break LOOP
		}
	}

	for {
		if len(f.msgChan) == 0 {
			break
		}

		f.write(<-f.msgChan)
	}
	f.quitChan <- struct{}{} // Notify the cleanup is done.
}

func (f *file) Destroy() {
	f.quitChan <- struct{}{}
	<-f.quitChan

	close(f.msgChan)
	close(f.quitChan)

	f.file.Close()
}

func init() {
	Register(FILE, newFile)
}
