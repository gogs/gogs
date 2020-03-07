// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package gitutil

import (
	"fmt"
	"strings"

	"github.com/gogs/git-module"
)

// InferSubmoduleURL returns the inferred external URL of the submodule at best effort.
func InferSubmoduleURL(mod *git.Submodule) string {
	urlStr := mod.URL()
	urlStr = strings.TrimSuffix(urlStr, ".git")
	return fmt.Sprintf("%s/commit/%s", urlStr, mod.Commit())
}
