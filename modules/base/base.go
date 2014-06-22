// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package base

type (
	// Type TmplData represents data in the templates.
	TmplData map[string]interface{}
	TplName  string

	ApiJsonErr struct {
		Message string `json:"message"`
		DocUrl  string `json:"documentation_url"`
	}
)

var GoGetMetas = make(map[string]bool)
