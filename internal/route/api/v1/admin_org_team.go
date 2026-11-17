package v1

import (
	"net/http"

	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/route/api/v1/types"
)

type adminCreateTeamRequest struct {
	Name        string `json:"name" binding:"Required;AlphaDashDot;MaxSize(30)"`
	Description string `json:"description" binding:"MaxSize(255)"`
	Permission  string `json:"permission"`
}

func adminCreateTeam(c *apiContext, form adminCreateTeamRequest) {
	team := &database.Team{
		OrgID:       c.Org.Organization.ID,
		Name:        form.Name,
		Description: form.Description,
		Authorize:   database.ParseAccessMode(form.Permission),
	}
	if err := database.Handle.Teams().Create(c.Request().Context(), team); err != nil {
		if database.IsErrTeamAlreadyExist(err) {
			c.ErrorStatus(http.StatusUnprocessableEntity, err)
		} else {
			c.Error(err, "new team")
		}
		return
	}

	c.JSON(http.StatusCreated, toOrganizationTeam(team))
}

func adminAddTeamMember(c *apiContext) {
	u := getUserByParams(c)
	if c.Written() {
		return
	}
	if err := database.Handle.Teams().AddMember(c.Request().Context(), c.Org.Team.OrgID, c.Org.Team.ID, u.ID); err != nil {
		c.Error(err, "add member")
		return
	}

	c.NoContent()
}

func adminRemoveTeamMember(c *apiContext) {
	u := getUserByParams(c)
	if c.Written() {
		return
	}

	if err := database.Handle.Teams().RemoveMember(c.Request().Context(), c.Org.Team.OrgID, c.Org.Team.ID, u.ID); err != nil {
		c.Error(err, "remove member")
		return
	}

	c.NoContent()
}

func adminListTeamMembers(c *apiContext) {
	members, err := database.Handle.Teams().ListMembers(c.Request().Context(), c.Org.Team.ID)
	if err != nil {
		c.Error(err, "get team members")
		return
	}

	apiMembers := make([]*types.User, len(members))
	for i := range members {
		apiMembers[i] = toUser(members[i])
	}
	c.JSONSuccess(apiMembers)
}
