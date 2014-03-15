// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package base

import (
	"html/template"
)

func Str2html(raw string) template.HTML {
	return template.HTML(raw)
}

var TemplateFuncs template.FuncMap = map[string]interface{}{
	"AppName": func() string {
		return AppName
	},
	"AppVer": func() string {
		return AppVer
	},
	"str2html":   Str2html,
	"TimeSince":  TimeSince,
	"Subtract":   Subtract,
	"ActionIcon": ActionIcon,
	"ActionDesc": ActionDesc,
	"DateFormat": DateFormat,
}
