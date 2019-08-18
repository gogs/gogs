// Copyright 2017 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func Test_parseRemoteUpdateOutput(t *testing.T) {
	Convey("Parse mirror remote update output", t, func() {
		testCases := []struct {
			output  string
			results []*mirrorSyncResult
		}{
			{
				`
From https://try.gogs.io/unknwon/upsteam
 * [new branch]      develop    -> develop
   b0bb24f..1d85a4f  master     -> master
 - [deleted]         (none)     -> bugfix
`,
				[]*mirrorSyncResult{
					{"develop", GIT_SHORT_EMPTY_SHA, ""},
					{"master", "b0bb24f", "1d85a4f"},
					{"bugfix", "", GIT_SHORT_EMPTY_SHA},
				},
			},
		}

		for _, tc := range testCases {
			results := parseRemoteUpdateOutput(tc.output)
			So(len(results), ShouldEqual, len(tc.results))

			for i := range tc.results {
				So(tc.results[i].refName, ShouldEqual, results[i].refName)
				So(tc.results[i].oldCommitID, ShouldEqual, results[i].oldCommitID)
				So(tc.results[i].newCommitID, ShouldEqual, results[i].newCommitID)
			}
		}
	})
}

func Test_findPasswordInMirrorAddress(t *testing.T) {
	Convey("Find password portion in mirror address", t, func() {
		testCases := []struct {
			addr       string
			start, end int
			found      bool
			password   string
		}{
			{"http://localhost:3000/user/repo.git", -1, -1, false, ""},
			{"http://user@localhost:3000/user/repo.git", -1, -1, false, ""},
			{"http://user:@localhost:3000/user/repo.git", -1, -1, false, ""},
			{"http://user:password@localhost:3000/user/repo.git", 12, 20, true, "password"},
			{"http://username:my%3Asecure%3Bpassword@localhost:3000/user/repo.git", 16, 38, true, "my%3Asecure%3Bpassword"},
			{"http://username:my%40secure%23password@localhost:3000/user/repo.git", 16, 38, true, "my%40secure%23password"},
			{"http://username:@@localhost:3000/user/repo.git", 16, 17, true, "@"},
		}

		for _, tc := range testCases {
			start, end, found := findPasswordInMirrorAddress(tc.addr)
			So(start, ShouldEqual, tc.start)
			So(end, ShouldEqual, tc.end)
			So(found, ShouldEqual, tc.found)
			if found {
				So(tc.addr[start:end], ShouldEqual, tc.password)
			}
		}
	})
}

func Test_unescapeMirrorCredentials(t *testing.T) {
	Convey("Escape credentials in mirror address", t, func() {
		testCases := []string{
			"http://localhost:3000/user/repo.git", "http://localhost:3000/user/repo.git",
			"http://user@localhost:3000/user/repo.git", "http://user@localhost:3000/user/repo.git",
			"http://user:@localhost:3000/user/repo.git", "http://user:@localhost:3000/user/repo.git",
			"http://user:password@localhost:3000/user/repo.git", "http://user:password@localhost:3000/user/repo.git",
			"http://user:my%3Asecure%3Bpassword@localhost:3000/user/repo.git", "http://user:my:secure;password@localhost:3000/user/repo.git",
			"http://user:my%40secure%23password@localhost:3000/user/repo.git", "http://user:my@secure#password@localhost:3000/user/repo.git",
		}

		for i := 0; i < len(testCases); i += 2 {
			So(unescapeMirrorCredentials(testCases[i]), ShouldEqual, testCases[i+1])
		}
	})
}

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
