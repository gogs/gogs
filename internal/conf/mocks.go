// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package conf

import (
	"testing"
)

func SetMockServer(t *testing.T, opts ServerOpts) {
	before := Server
	Server = opts
	t.Cleanup(func() {
		Server = before
	})
}
