// Copyright 2018 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package tool

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func Test_IsSameSiteURLPath(t *testing.T) {
	Convey("Check if a path belongs to the same site", t, func() {
		testCases := []struct {
			url    string
			expect bool
		}{
			{"//github.com", false},
			{"http://github.com", false},
			{"https://github.com", false},
			{"/\\github.com", false},

			{"/admin", true},
			{"/user/repo", true},
		}

		for _, tc := range testCases {
			So(IsSameSiteURLPath(tc.url), ShouldEqual, tc.expect)
		}
	})
}

func Test_NormalizePath(t *testing.T) {
	Convey("Normalize malicious user-defined path", t, func() {
		testCases := []struct {
			path   string
			expect string
		}{
			{"../../../../../../../../../a9f0ab6c3ef63dd8", "a9f0ab6c3ef63dd8"},
			{"..././a9f0ab6c3ef63dd8", "a9f0ab6c3ef63dd8"},
		}
		for _, tc := range testCases {
			So(NormalizePath(tc.path), ShouldEqual, tc.expect)
		}
	})
}
