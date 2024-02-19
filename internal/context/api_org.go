// Copyright 2016 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package context

import (
	"gogs.io/gogs/internal/database"
)

type APIOrganization struct {
	Organization *database.User
	Team         *database.Team
}
