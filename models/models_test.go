// Copyright 2016 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func Test_parsePostgreSQLHostPort(t *testing.T) {
	testSuites := []struct {
		input      string
		host, port string
	}{
		{"127.0.0.1:1234", "127.0.0.1", "1234"},
		{"127.0.0.1", "127.0.0.1", "5432"},
		{"[::1]:1234", "[::1]", "1234"},
		{"[::1]", "[::1]", "5432"},
		{"/tmp/pg.sock:1234", "/tmp/pg.sock", "1234"},
		{"/tmp/pg.sock", "/tmp/pg.sock", "5432"},
	}

	Convey("Parse PostgreSQL host and port", t, func() {
		for _, suite := range testSuites {
			host, port := parsePostgreSQLHostPort(suite.input)
			So(host, ShouldEqual, suite.host)
			So(port, ShouldEqual, suite.port)
		}
	})
}
