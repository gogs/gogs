// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package mock

import (
	"gopkg.in/macaron.v1"
)

var _ macaron.Locale = (*Locale)(nil)

// Locale is a mock that implements macaron.Locale.
type Locale struct {
	lang string
	tr   func(string, ...interface{}) string
}

// NewLocale creates a new mock for macaron.Locale.
func NewLocale(lang string, tr func(string, ...interface{}) string) *Locale {
	return &Locale{
		lang: lang,
		tr:   tr,
	}
}

func (l *Locale) Language() string {
	return l.lang
}

func (l *Locale) Tr(format string, args ...interface{}) string {
	return l.tr(format, args...)
}
