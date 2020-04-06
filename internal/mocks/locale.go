// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package mocks

import (
	"gopkg.in/macaron.v1"
)

var _ macaron.Locale = (*Locale)(nil)

type Locale struct {
	MockLang string
	MockTr   func(string, ...interface{}) string
}

func (l *Locale) Language() string {
	return l.MockLang
}

func (l *Locale) Tr(format string, args ...interface{}) string {
	return l.MockTr(format, args...)
}
