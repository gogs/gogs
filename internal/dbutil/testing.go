// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package dbutil

import (
	"strings"
	"time"
)

// QuoteIdentifier returns quoted identifier with double quotes.
func QuoteIdentifier(s string) string {
	return `"` + strings.Replace(s, `"`, `""`, -1) + `"`
}

// RecreateTime recreates the time.Time with minimal timezone information to be
// comparable in tests. We need to do this because the time got back from SQLite
// driver includes weird timezone information.
func RecreateTime(t time.Time) time.Time {
	return time.Unix(t.Unix(), 0)
}
