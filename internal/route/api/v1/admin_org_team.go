package v1

import (
	"net/http"

	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/route/api/v1/types"
)

type adminCreateTeamRequest struct {
	Name        string `json:"name" binding:"Required;AlphaDashDot;MaxSize(30)"`
	Description string `json:"description" binding:"MaxSize(255)"`
	Permission  string `json:"permission"`
}

func adminCreateTeam(c *context.APIContext, form adminCreateTeamRequest) {
	team := &database.Team{
		OrgID:       c.Org.Organization.ID,
		Name:        form.Name,
		Description: form.Description,
		Authorize:   database.ParseAccessMode(form.Permission),
	}
	if err := database.NewTeam(team); err != nil {
		if database.IsErrTeamAlreadyExist(err) {
			c.ErrorStatus(http.StatusUnprocessableEntity, err)
		} else {
			c.Error(err, "new team")
		}
		return
	}

	c.JSON(http.StatusCreated, toOrganizationTeam(team))
}

func adminAddTeamMember(c *context.APIContext) {
	u := getUserByParams(c)
	if c.Written() {
		return
	}
	if err := c.Org.Team.AddMember(u.ID); err != nil {
		c.Error(err, "add member")
		return
	}

	c.NoContent()
}

func adminRemoveTeamMember(c *context.APIContext) {
	u := getUserByParams(c)
	if c.Written() {
		return
	}

	if err := c.Org.Team.RemoveMember(u.ID); err != nil {
		c.Error(err, "remove member")
		return
	}

	c.NoContent()
}

func adminListTeamMembers(c *context.APIContext) {
	team := c.Org.Team
	if err := team.GetMembers(); err != nil {
		c.Error(err, "get team members")
		return
	}

	apiMembers := make([]*types.User, len(team.Members))
	for i := range team.Members {
		apiMembers[i] = toUser(team.Members[i])
	}
	c.JSONSuccess(apiMembers)
}
