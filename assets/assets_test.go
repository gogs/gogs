// Copyright 2021 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package assets

import (
	"testing"
)

func TestAsset(t *testing.T) {
	// Make sure it does not blow up
	_, err := Asset("conf/app.ini")
	if err != nil {
		t.Fatal(err)
	}
}

func TestAssetDir(t *testing.T) {
	// Make sure it does not blow up
	_, err := AssetDir("conf")
	if err != nil {
		t.Fatal(err)
	}
}

func TestMustAsset(t *testing.T) {
	// Make sure it does not blow up
	MustAsset("conf/app.ini")
}

func TestMustAssetDir(t *testing.T) {
	// Make sure it does not blow up
	MustAssetDir("conf")
}
