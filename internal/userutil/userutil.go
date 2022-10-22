// Copyright 2022 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package userutil

import (
	"encoding/hex"
	"fmt"
	"strings"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/tool"
)

// DashboardURLPath returns the URL path to the user or organization dashboard.
func DashboardURLPath(name string, isOrganization bool) string {
	if isOrganization {
		return conf.Server.Subpath + "/org/" + name + "/dashboard/"
	}
	return conf.Server.Subpath + "/"
}

// GenerateActivateCode generates an activate code based on user information and
// the given email.
func GenerateActivateCode(id int64, email, name, password, rands string) string {
	code := tool.CreateTimeLimitCode(
		fmt.Sprintf("%d%s%s%s%s", id, email, strings.ToLower(name), password, rands),
		conf.Auth.ActivateCodeLives,
		nil,
	)

	// Add tailing hex username
	code += hex.EncodeToString([]byte(strings.ToLower(name)))
	return code
}
