package v1

import (
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/route/api/v1/types"
)

// listTeams lists every team of the organization to its owners and site
// admins, and only their own teams to other members.
func listTeams(c *apiContext) {
	org := c.Org.Organization
	ctx := c.Request().Context()
	isOwner := c.User.IsAdmin || database.Handle.Organizations().IsOwner(ctx, org.ID, c.User.ID)
	if !isOwner && !database.Handle.Organizations().IsMember(ctx, org.ID, c.User.ID) {
		c.NotFound()
		return
	}

	var teams []*database.Team
	var err error
	if isOwner {
		teams, err = database.Handle.Teams().ListByOrg(ctx, org.ID)
	} else {
		teams, err = database.Handle.Teams().ListByMember(ctx, org.ID, c.User.ID)
	}
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
