// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package dev

import (
	"net/http/pprof"

	"github.com/Unknwon/macaron"
)

func RegisterDebugRoutes(r *macaron.Macaron) {
	r.Any("/debug/pprof/cmdline", pprof.Cmdline)
	r.Any("/debug/pprof/profile", pprof.Profile)
	r.Any("/debug/pprof/symbol", pprof.Symbol)
	r.Any("/debug/pprof/**", pprof.Index)
}
