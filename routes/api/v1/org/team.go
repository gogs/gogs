// Copyright 2016 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package org

import (
	api "github.com/gogits/go-gogs-client"

	"github.com/gogits/gogs/pkg/context"
	"github.com/gogits/gogs/routes/api/v1/convert"
)

func ListTeams(c *context.APIContext) {
	org := c.Org.Organization
	if err := org.GetTeams(); err != nil {
		c.Error(500, "GetTeams", err)
		return
	}

	apiTeams := make([]*api.Team, len(org.Teams))
	for i := range org.Teams {
		apiTeams[i] = convert.ToTeam(org.Teams[i])
	}
	c.JSON(200, apiTeams)
}
