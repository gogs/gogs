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
	// isHTTPS := detect HTTPS or reverse proxy
	isHTTPS := false

	// useHTTPS := setting whether HTTPS shall be used
	useHTTPS := false

	if !isHTTPS && useHTTPS {
		// redirect here
	}

	// enable XSS protection in Internet Explorer, Chrome & Safari
	ctx.Resp.Header().Set("X-Xss-Protection", "1; mode=block")

	// Disallow mime sniffing in Internet Explorer & Chrome
	ctx.Resp.Header().Set("X-Content-Type-Options", "nosniff")


	csp := "default-src 'self';"                    // child-src, connect-src, font-src, manifest-src, media-src, script-src
	csp += "img-src '*';"                           // all image sources
	csp += "style-src 'self' 'unsafe-inline';"      // <style> in HTML header and body
	csp += "frame-src 'none';"                      // no <frame> and <iframe>
	csp += "object-src 'none';"                     // no <object>, <embed> and <applet>
	csp += "reflected-xss block;"                   // same as X-Xss-Protection

	// cspReferrer := get setting. default: origin-when-cross-origin
	// possible values: no-referrer, no-referrer-when-downgrade, origin-when-cross-origin, unsafe-url
	cspReferrer := "origin-when-cross-origin"
	if cspReferrer == "" {
		cspReferrer = "origin-when-cross-origin"
	}
	csp += "referrer " + cspReferrer + ";"

	if (useHTTPS) {
		// cspUpgradeReqs := get setting. default: true
		cspUpgradeReqs := true
		if cspUpgradeReqs == true {
			csp += "upgrade-insecure-requests;"
		}
	}
	// Only allow loading of resources of these domains
	ctx.Resp.Header().Set("Content-Security-Policy", csp)


	// frameOptions := get value from setting. default: "SAMEORIGIN"
	// possible values: DENY, SAMEORIGIN, ALLOW-FROM https://example.com
	frameOptions := "SAMEORIGIN"
	if frameOptions == "" {
		frameOptions = "SAMEORIGIN"
	}
	// Allow framing only for own domain
	ctx.Resp.Header().Set("X-Frame-Options", frameOptions)


	if (useHTTPS) {
		// maxAge := get value from setting. unit: seconds, default: "31536000" (1 year)
		maxAge := "31536000"
		maxSeconds, err := strconv.Atoi(maxAge)
		if err != nil {
			// check skip value
			// insert name of setting here
			log.Warn("Invalid value for HSTS setting: %v", err)
			maxSeconds = 31536000
		}
		ctx.Resp.Header().Set("Strict-Transport-Security", "max-age=" + strconv.Itoa(maxSeconds))
	}
}
