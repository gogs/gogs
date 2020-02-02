// Copyright 2018 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package lazyregexp is a thin wrapper over regexp, allowing the use of global
// regexp variables without forcing them to be compiled at init.
package lazyregexp

import (
	"os"
	"regexp"
	"strings"
	"sync"
)

// Regexp is a wrapper around regexp.Regexp, where the underlying regexp will be
// compiled the first time it is needed.
type Regexp struct {
	str  string
	once sync.Once
	rx   *regexp.Regexp
}

func (r *Regexp) Regexp() *regexp.Regexp {
	r.once.Do(r.build)
	return r.rx
}

func (r *Regexp) build() {
	r.rx = regexp.MustCompile(r.str)
	r.str = ""
}

func (r *Regexp) Find(b []byte) []byte {
	return r.Regexp().Find(b)
}

func (r *Regexp) FindSubmatch(s []byte) [][]byte {
	return r.Regexp().FindSubmatch(s)
}

func (r *Regexp) FindStringSubmatch(s string) []string {
	return r.Regexp().FindStringSubmatch(s)
}

func (r *Regexp) FindStringSubmatchIndex(s string) []int {
	return r.Regexp().FindStringSubmatchIndex(s)
}

func (r *Regexp) ReplaceAllString(src, repl string) string {
	return r.Regexp().ReplaceAllString(src, repl)
}

func (r *Regexp) FindString(s string) string {
	return r.Regexp().FindString(s)
}

func (r *Regexp) FindAll(b []byte, n int) [][]byte {
	return r.Regexp().FindAll(b, n)
}

func (r *Regexp) FindAllString(s string, n int) []string {
	return r.Regexp().FindAllString(s, n)
}

func (r *Regexp) MatchString(s string) bool {
	return r.Regexp().MatchString(s)
}

func (r *Regexp) SubexpNames() []string {
	return r.Regexp().SubexpNames()
}

func (r *Regexp) FindAllStringSubmatch(s string, n int) [][]string {
	return r.Regexp().FindAllStringSubmatch(s, n)
}

func (r *Regexp) Split(s string, n int) []string {
	return r.Regexp().Split(s, n)
}

func (r *Regexp) ReplaceAllLiteralString(src, repl string) string {
	return r.Regexp().ReplaceAllLiteralString(src, repl)
}

func (r *Regexp) FindAllIndex(b []byte, n int) [][]int {
	return r.Regexp().FindAllIndex(b, n)
}

func (r *Regexp) Match(b []byte) bool {
	return r.Regexp().Match(b)
}

func (r *Regexp) ReplaceAllStringFunc(src string, repl func(string) string) string {
	return r.Regexp().ReplaceAllStringFunc(src, repl)
}

func (r *Regexp) ReplaceAll(src, repl []byte) []byte {
	return r.Regexp().ReplaceAll(src, repl)
}

var inTest = len(os.Args) > 0 && strings.HasSuffix(strings.TrimSuffix(os.Args[0], ".exe"), ".test")

// New creates a new lazy regexp, delaying the compiling work until it is first
// needed. If the code is being run as part of tests, the regexp compiling will
// happen immediately.
func New(str string) *Regexp {
	lr := &Regexp{str: str}
	if inTest {
		// In tests, always compile the regexps early.
		lr.Regexp()
	}
	return lr
}
