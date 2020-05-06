// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package semverutil

import (
	"strings"

	"github.com/Masterminds/semver/v3"
)

// Compare returns true if the comparison is true for given versions. It returns false if
// comparison is false, or failed to parse one or both versions as Semantic Versions.
//
// See https://github.com/Masterminds/semver#basic-comparisons for supported comparisons.
func Compare(version1, comparison, version2 string) bool {
	clean := func(v string) string {
		if strings.Count(v, ".") > 2 {
			fields := strings.SplitN(v, ".", 4)
			v = strings.Join(fields[:3], ".")
		}
		return v
	}

	v, err := semver.NewVersion(clean(version1))
	if err != nil {
		return false
	}

	c, err := semver.NewConstraint(comparison + " " + clean(version2))
	if err != nil {
		return false
	}

	return c.Check(v)
}
