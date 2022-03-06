// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package testutil

import (
	log "unknwon.dev/clog/v2"
)

var _ log.Logger = (*noopLogger)(nil)

// noopLogger is a placeholder logger that logs nothing.
type noopLogger struct{}

func (*noopLogger) Name() string {
	return "noop"
}

func (*noopLogger) Level() log.Level {
	return log.LevelTrace
}

func (*noopLogger) Write(log.Messager) error {
	return nil
}

// InitNoopLogger is a init function to initialize a noop logger.
var InitNoopLogger = func(name string, vs ...interface{}) (log.Logger, error) {
	return &noopLogger{}, nil
}
