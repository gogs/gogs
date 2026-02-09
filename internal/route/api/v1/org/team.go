package org

import (
	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/route/api/v1/convert"
	"gogs.io/gogs/internal/route/api/v1/types"
)

func ListTeams(c *context.APIContext) {
	org := c.Org.Organization
	if err := org.GetTeams(); err != nil {
		c.Error(err, "get teams")
		return
	}

	apiTeams := make([]*types.Team, len(org.Teams))
	for i := range org.Teams {
		apiTeams[i] = convert.ToTeam(org.Teams[i])
	}
	c.JSONSuccess(apiTeams)
}
