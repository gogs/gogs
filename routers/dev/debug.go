// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package dev

import (
	"net/http/pprof"

	"github.com/go-martini/martini"
)

func RegisterDebugRoutes(r martini.Router) {
	r.Any("/debug/pprof/cmdline", pprof.Cmdline)
	r.Any("/debug/pprof/profile", pprof.Profile)
	r.Any("/debug/pprof/symbol", pprof.Symbol)
	r.Any("/debug/pprof/**", pprof.Index)
}
