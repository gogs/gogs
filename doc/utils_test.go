// Copyright 2013-2014 gopm authors.
//
// Licensed under the Apache License, Version 2.0 (the "License"): you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
// WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
// License for the specific language governing permissions and limitations
// under the License.

package doc

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

var VcsTestPairs = map[string]bool{
	"/.hg":     true,
	"/.git":    true,
	"/.svn":    true,
	"/.vendor": false,
}

func Test_isVcsPath(t *testing.T) {
	Convey("Test if the path is belonging to VCS", t, func() {
		for name, expect := range VcsTestPairs {
			So(isVcsPath(name), ShouldEqual, expect)
		}
	})
}

func TestGetDirsInfo(t *testing.T) {
	Convey("Get directory's information that exist", t, func() {
		dis, err := GetDirsInfo(".")
		So(err, ShouldBeNil)
		So(len(dis), ShouldEqual, 13)
	})

	Convey("Get directory's information does not exist", t, func() {
		dis, err := GetDirsInfo("./404")
		So(err, ShouldBeNil)
		So(len(dis), ShouldEqual, 0)
	})
}

var GoStdTestPairs = map[string]bool{
	"net/http": true,
	"fmt":      true,
	"github.com/gpmgo/gopm":  false,
	"github.com/Unknwon/com": false,
}

func TestIsGoRepoPath(t *testing.T) {
	Convey("Test if the path is belonging to Go STD", t, func() {
		for name, expect := range GoStdTestPairs {
			So(IsGoRepoPath(name), ShouldEqual, expect)
		}
	})
}

func TestGetImports(t *testing.T) {
	Convey("Get package that are imported", t, func() {
		So(len(GetImports(".", "github.com/gpmgo/gopm/docs", false)), ShouldEqual, 4)
	})
}
