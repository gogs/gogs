// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package log

import (
	"github.com/codegangsta/martini"
	"github.com/martini-contrib/render"
)

// ErrHandler is an interface for custom error handler.
type ErrHandler interface {
	Handle(string, render.Render, error)
}

// ErrHandle is a Middleware that maps a ErrHandler service into the Martini handler chain.
func ErrHandle() martini.Handler {
	return func(context martini.Context) {
		context.MapTo(&errHandler{}, (*ErrHandler)(nil))
	}
}

type errHandler struct {
}

func (eh *errHandler) Handle(title string, r render.Render, err error) {
	Error("%s: %v", title, err)
	r.HTML(200, "base/error", map[string]interface{}{
		"ErrorMsg": err,
	})
}
