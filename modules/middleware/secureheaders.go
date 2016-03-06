// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package middleware

import (
	// "github.com/gogits/gogs/modules/setting"
)

func SecureHeaders(ctx *Context) {
	// enable XSS protection in Internet Explorer, Chrome & Safari
	ctx.Resp.Header().Set("X-Xss-Protection", "1; mode=block")

	// Allow framing only for own domain
	ctx.Resp.Header().Set("X-Frame-Options", "SAMEORIGIN")

	// Disallow mime sniffing in Internet Explorer & Chrome
	ctx.Resp.Header().Set("X-Content-Type-Options", "nosniff")

	// Only allow loading of resources of these domains
	ctx.Resp.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline' 'unsafe-eval'; img-src 'self' '*.gravatar.com'; style-src 'self' 'unsafe-inline'; font-src 'self'; frame-src 'none'; object-src 'none'")
}
