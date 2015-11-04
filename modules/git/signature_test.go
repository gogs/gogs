// Copyright 2015 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package git

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func Test_newSignatureFromCommitline(t *testing.T) {
	Convey("Parse signature from commit line", t, func() {
		line := "Intern <intern@macbook-intern.(none)> 1445412825 +0200"
		sig, err := newSignatureFromCommitline([]byte(line))
		So(err, ShouldBeNil)
		So(sig, ShouldNotBeNil)
	})
}
