// +build miniwinsvc

// Copyright 2015 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package setting

import (
	_ "github.com/gogs/minwinsvc"
)

func init() {
	SupportMiniWinService = true
}
