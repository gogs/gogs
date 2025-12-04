// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package conf

import (
	"sync"
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

var mockServer sync.Mutex

func SetMockServer(t *testing.T, opts ServerOpts) {
	mockServer.Lock()
	before := Server
	Server = opts
	t.Cleanup(func() {
		Server = before
		mockServer.Unlock()
	})
}

var mockSSH sync.Mutex

func SetMockSSH(t *testing.T, opts SSHOpts) {
	mockSSH.Lock()
	before := SSH
	SSH = opts
	t.Cleanup(func() {
		SSH = before
		mockSSH.Unlock()
	})
}

var mockRepository sync.Mutex

func SetMockRepository(t *testing.T, opts RepositoryOpts) {
	mockRepository.Lock()
	before := Repository
	Repository = opts
	t.Cleanup(func() {
		Repository = before
		mockRepository.Unlock()
	})
}

func SetMockUI(t *testing.T, opts UIOpts) {
	before := UI
	UI = opts
	t.Cleanup(func() {
		UI = before
	})
}

var mockPicture sync.Mutex

func SetMockPicture(t *testing.T, opts PictureOpts) {
	mockPicture.Lock()
	before := Picture
	Picture = opts
	t.Cleanup(func() {
		Picture = before
		mockPicture.Unlock()
	})
}
