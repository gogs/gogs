// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package base

import (
	"container/list"
	"fmt"
	"html/template"
	"strings"
	"time"
)

func Str2html(raw string) template.HTML {
	return template.HTML(raw)
}

func Range(l int) []int {
	return make([]int, l)
}

func List(l *list.List) chan interface{} {
	e := l.Front()
	c := make(chan interface{})
	go func() {
		for e != nil {
			c <- e.Value
			e = e.Next()
		}
		close(c)
	}()
	return c
}

func ShortSha(sha1 string) string {
	if len(sha1) == 40 {
		return sha1[:10]
	}
	return sha1
}

var mailDomains = map[string]string{
	"gmail.com": "gmail.com",
}

var TemplateFuncs template.FuncMap = map[string]interface{}{
	"AppName": func() string {
		return AppName
	},
	"AppVer": func() string {
		return AppVer
	},
	"AppDomain": func() string {
		return Domain
	},
	"LoadTimes": func(startTime time.Time) string {
		return fmt.Sprint(time.Since(startTime).Nanoseconds()/1e6) + "ms"
	},
	"AvatarLink": AvatarLink,
	"str2html":   Str2html,
	"TimeSince":  TimeSince,
	"FileSize":   FileSize,
	"Subtract":   Subtract,
	"ActionIcon": ActionIcon,
	"ActionDesc": ActionDesc,
	"DateFormat": DateFormat,
	"List":       List,
	"Mail2Domain": func(mail string) string {
		if !strings.Contains(mail, "@") {
			return "try.gogits.org"
		}

		suffix := strings.SplitN(mail, "@", 2)[1]
		domain, ok := mailDomains[suffix]
		if !ok {
			return "mail." + suffix
		}
		return domain
	},
	"SubStr": func(str string, start, length int) string {
		return str[start : start+length]
	},
	"DiffTypeToStr":     DiffTypeToStr,
	"DiffLineTypeToStr": DiffLineTypeToStr,
	"ShortSha":          ShortSha,
}
