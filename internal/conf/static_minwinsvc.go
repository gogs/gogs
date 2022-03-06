//go:build minwinsvc
// +build minwinsvc

// Copyright 2015 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package conf

import (
	_ "github.com/gogs/minwinsvc"
)

func init() {
	HasMinWinSvc = true
}
