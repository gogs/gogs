package v1

import (
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/route/api/v1/types"
)

func listTeams(c *apiContext) {
	org := c.Org.Organization
	teams, err := database.Handle.Teams().ListByOrg(c.Request().Context(), org.ID)
	if err != nil {
		c.Error(err, "get teams")
		return
	}

	apiTeams := make([]*types.OrganizationTeam, len(teams))
	for i := range teams {
		apiTeams[i] = toOrganizationTeam(teams[i])
	}
	c.JSONSuccess(apiTeams)
}
