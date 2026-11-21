package v1

import (
	"net/http"

	"gogs.io/gogs/internal/route/api/v1/types"
)

func listTeams(c *apiContext) {
	org := c.Org.Organization
	if !c.User.IsAdmin && !org.IsOrgMember(c.User.ID) {
		c.Status(http.StatusForbidden)
		return
	}

	if err := org.GetTeams(); err != nil {
		c.Error(err, "get teams")
		return
	}

	apiTeams := make([]*types.OrganizationTeam, len(org.Teams))
	for i := range org.Teams {
		apiTeams[i] = toOrganizationTeam(org.Teams[i])
	}
	c.JSONSuccess(apiTeams)
}
