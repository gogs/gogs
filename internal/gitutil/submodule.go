// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package gitutil

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/gogs/git-module"

	"gogs.io/gogs/internal/lazyregexp"
)

var scpSyntax = lazyregexp.New(`^([a-zA-Z0-9_]+@)?([a-zA-Z0-9._-]+):(.*)$`)

// InferSubmoduleURL returns the inferred external URL of the submodule at best effort.
// The `baseURL` should be the URL of the current repository. If the submodule URL looks
// like a relative path, it assumes that the submodule is another repository on the same
// Gogs instance by appending it to the `baseURL` with the commit.
func InferSubmoduleURL(baseURL string, mod *git.Submodule) string {
	if !strings.HasSuffix(baseURL, "/") {
		baseURL += "/"
	}

	raw := strings.TrimSuffix(mod.URL, "/")
	raw = strings.TrimSuffix(raw, ".git")

	if strings.HasPrefix(raw, "../") {
		return fmt.Sprintf("%s%s/commit/%s", baseURL, raw, mod.Commit)
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		// Try parse as SCP syntax again
		match := scpSyntax.FindAllStringSubmatch(raw, -1)
		if len(match) == 0 {
			return mod.URL
		}
		parsed = &url.URL{
			Scheme: "http",
			Host:   match[0][2],
			Path:   match[0][3],
		}
	}

	switch parsed.Scheme {
	case "http", "https":
		raw = parsed.String()
	case "ssh":
		raw = fmt.Sprintf("http://%s%s", parsed.Hostname(), parsed.Path)
	default:
		return raw
	}

	return fmt.Sprintf("%s/commit/%s", raw, mod.Commit)
}
