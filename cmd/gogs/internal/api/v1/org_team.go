package v1

import (
	"gogs.io/gogs/internal/api/v1types"
	"gogs.io/gogs/internal/database"
)

func listTeams(c *apiContext) {
	org := c.Org.Organization
	teams, err := database.Handle.Teams().ListByOrg(c.Request().Context(), org.ID)
	if err != nil {
		c.Error(err, "get teams")
		return
	}

	apiTeams := make([]*v1types.OrganizationTeam, len(teams))
	for i := range teams {
		apiTeams[i] = toOrganizationTeam(teams[i])
	}
	c.JSONSuccess(apiTeams)
}
