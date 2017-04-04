// Copyright 2016 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package avatar

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func Test_RandomImage(t *testing.T) {
	Convey("Generate a random avatar from email", t, func() {
		_, err := RandomImage([]byte("gogs@local"))
		So(err, ShouldBeNil)

		Convey("Try to generate an image with size zero", func() {
			_, err := RandomImageSize(0, []byte("gogs@local"))
			So(err, ShouldNotBeNil)
		})
	})
}
