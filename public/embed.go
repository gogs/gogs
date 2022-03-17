// Copyright 2022 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package public

import (
	"embed"
)

//go:embed assets/* css/* img/* js/* plugins/*
var Files embed.FS
