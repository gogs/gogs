// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package conf

import (
	"testing"
)

func SetMockApp(t *testing.T, opts AppOpts) {
	before := App
	App = opts
	t.Cleanup(func() {
		App = before
	})
}

func SetMockAuth(t *testing.T, otps AuthOpts) {
	before := Auth
	Auth = otps
	t.Cleanup(func() {
		Auth = before
	})
}

func SetMockServer(t *testing.T, opts ServerOpts) {
	before := Server
	Server = opts
	t.Cleanup(func() {
		Server = before
	})
}

func SetMockSSH(t *testing.T, opts SSHOpts) {
	before := SSH
	SSH = opts
	t.Cleanup(func() {
		SSH = before
	})
}

func SetMockRepository(t *testing.T, opts RepositoryOpts) {
	before := Repository
	Repository = opts
	t.Cleanup(func() {
		Repository = before
	})
}

func SetMockUI(t *testing.T, opts UIOpts) {
	before := UI
	UI = opts
	t.Cleanup(func() {
		UI = before
	})
}
