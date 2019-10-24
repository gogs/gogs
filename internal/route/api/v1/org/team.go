// Copyright 2016 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package org

import (
	api "github.com/gogs/go-gogs-client"
	convert2 "gogs.io/gogs/internal/route/api/v1/convert"

	"gogs.io/gogs/internal/context"
)

func ListTeams(c *context.APIContext) {
	org := c.Org.Organization
	if err := org.GetTeams(); err != nil {
		c.Error(500, "GetTeams", err)
		return
	}

	apiTeams := make([]*api.Team, len(org.Teams))
	for i := range org.Teams {
		apiTeams[i] = convert2.ToTeam(org.Teams[i])
	}
	c.JSON(200, apiTeams)
}
