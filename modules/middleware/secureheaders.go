// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package middleware

import (
	"strconv"

	"github.com/gogits/gogs/modules/log"
	// "github.com/gogits/gogs/modules/setting"
)

func SecureHeaders(ctx *Context) {
	// enable XSS protection in Internet Explorer, Chrome & Safari
	ctx.Resp.Header().Set("X-Xss-Protection", "1; mode=block")

	// Disallow mime sniffing in Internet Explorer & Chrome
	ctx.Resp.Header().Set("X-Content-Type-Options", "nosniff")

	// Only allow loading of resources of these domains
	ctx.Resp.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline' 'unsafe-eval'; img-src 'self' '*.gravatar.com'; style-src 'self' 'unsafe-inline'; font-src 'self'; frame-src 'none'; object-src 'none'")


	// frameOptions := get value from setting. default: "SAMEORIGIN"
	// possible values: DENY, SAMEORIGIN, ALLOW-FROM https://example.com
	frameOptions := "SAMEORIGIN"
	if frameOptions == "" {
		frameOptions = "SAMEORIGIN"
	}
	// Allow framing only for own domain
	ctx.Resp.Header().Set("X-Frame-Options", frameOptions)


	// if (we are delivering https) {
		// maxAge := get value from setting. unit: seconds, default: "31536000" (1 year)
		maxAge := "31536000"
		maxSeconds, err := strconv.Atoi(maxAge)
		if err != nil {
			// check skip value
			// insert name of setting here
			log.Error(4, "Invalid value for HSTS setting: %v", err)
			maxSeconds = 31536000
		}
		ctx.Resp.Header().Set("Strict-Transport-Security", "max-age=" + strconv.Itoa(maxSeconds))
	// }
}
