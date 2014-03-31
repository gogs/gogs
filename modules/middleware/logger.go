// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package middleware

import (
	"fmt"
	"log"
	"net/http"
	"runtime"
	"time"

	"github.com/go-martini/martini"
)

var isWindows bool

func init() {
	isWindows = runtime.GOOS == "windows"
}

func Logger() martini.Handler {
	return func(res http.ResponseWriter, req *http.Request, ctx martini.Context, log *log.Logger) {
		start := time.Now()
		log.Printf("Started %s %s", req.Method, req.URL.Path)

		rw := res.(martini.ResponseWriter)
		ctx.Next()

		content := fmt.Sprintf("Completed %v %s in %v", rw.Status(), http.StatusText(rw.Status()), time.Since(start))
		if !isWindows {
			switch rw.Status() {
			case 200:
				content = fmt.Sprintf("\033[1;32m%s\033[0m", content)
			case 304:
				content = fmt.Sprintf("\033[1;33m%s\033[0m", content)
			case 404:
				content = fmt.Sprintf("\033[1;31m%s\033[0m", content)
			case 500:
				content = fmt.Sprintf("\033[1;36m%s\033[0m", content)
			}
		}
		log.Println(content)
	}
}
