// Copyright 2018 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

import (
	"os"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func Test_isRepositoryGitPath(t *testing.T) {
	Convey("Check if path is or resides inside '.git'", t, func() {
		sep := string(os.PathSeparator)
		testCases := []struct {
			path   string
			expect bool
		}{
			{"." + sep + ".git", true},
			{"." + sep + ".git" + sep + "", true},
			{"." + sep + ".git" + sep + "hooks" + sep + "pre-commit", true},
			{".git" + sep + "hooks", true},
			{"dir" + sep + ".git", true},

			{".gitignore", false},
			{"dir" + sep + ".gitkeep", false},
		}
		for _, tc := range testCases {
			So(isRepositoryGitPath(tc.path), ShouldEqual, tc.expect)
		}
	})
}
