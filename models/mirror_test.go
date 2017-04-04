// Copyright 2017 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func Test_escapeMirrorCredentials(t *testing.T) {
	Convey("Escape credentials in mirror address", t, func() {
		testCases := []string{
			"http://localhost:3000/user/repo.git", "http://localhost:3000/user/repo.git",
			"http://user@localhost:3000/user/repo.git", "http://user@localhost:3000/user/repo.git",
			"http://user:@localhost:3000/user/repo.git", "http://user:@localhost:3000/user/repo.git",
			"http://user:password@localhost:3000/user/repo.git", "http://user:password@localhost:3000/user/repo.git",
			"http://user:my:secure;password@localhost:3000/user/repo.git", "http://user:my%3Asecure%3Bpassword@localhost:3000/user/repo.git",
			"http://user:my@secure#password@localhost:3000/user/repo.git", "http://user:my%40secure%23password@localhost:3000/user/repo.git",
		}

		for i := 0; i < len(testCases); i += 2 {
			So(escapeMirrorCredentials(testCases[i]), ShouldEqual, testCases[i+1])
		}
	})
}
