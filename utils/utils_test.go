// Copyright (c) 2013 GPMGo Members. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package utils

import (
	"testing"
)

var RemotePaths = []string{
	"github.com/coocood/qbs",
	"code.google.com/p/draw2d",
	"launchpad.net/goamz",
	"bitbucket.org/gotamer/conv",
}

func TestIsValidRemotePath(t *testing.T) {
	for _, p := range RemotePaths {
		if !IsValidRemotePath(p) {
			t.Errorf("Invalid remote path: %s", p)
		}
	}
}
