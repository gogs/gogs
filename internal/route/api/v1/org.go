package v1

import (
	"net/http"

	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/route/api/v1/types"
)

type createOrgRequest struct {
	UserName    string `json:"username" binding:"Required"`
	FullName    string `json:"full_name"`
	Description string `json:"description"`
	Website     string `json:"website"`
	Location    string `json:"location"`
}

func createOrgForUser(c *context.APIContext, apiForm createOrgRequest, user *database.User) {
	if c.Written() {
		return
	}

	org := &database.User{
		Name:        apiForm.UserName,
		FullName:    apiForm.FullName,
		Description: apiForm.Description,
		Website:     apiForm.Website,
		Location:    apiForm.Location,
		IsActive:    true,
		Type:        database.UserTypeOrganization,
	}
	if err := database.CreateOrganization(org, user); err != nil {
		if database.IsErrUserAlreadyExist(err) ||
			database.IsErrNameNotAllowed(err) {
			c.ErrorStatus(http.StatusUnprocessableEntity, err)
		} else {
			c.Error(err, "create organization")
		}
		return
	}

	c.JSON(201, toOrganization(org))
}

func listOrgsOfUser(c *context.APIContext, u *database.User, all bool) {
	orgs, err := database.Handle.Organizations().List(
		c.Req.Context(),
		database.ListOrgsOptions{
			MemberID:              u.ID,
			IncludePrivateMembers: all,
		},
	)
	if err != nil {
		c.Error(err, "list organizations")
		return
	}

	apiOrgs := make([]*types.Organization, len(orgs))
	for i := range orgs {
		apiOrgs[i] = toOrganization(orgs[i])
	}
	c.JSONSuccess(&apiOrgs)
}

func listMyOrgs(c *context.APIContext) {
	listOrgsOfUser(c, c.User, true)
}

func createMyOrg(c *context.APIContext, apiForm createOrgRequest) {
	createOrgForUser(c, apiForm, c.User)
}

func listUserOrgs(c *context.APIContext) {
	u := getUserByParams(c)
	if c.Written() {
		return
	}
	listOrgsOfUser(c, u, false)
}

func getOrg(c *context.APIContext) {
	c.JSONSuccess(toOrganization(c.Org.Organization))
}

type editOrgRequest struct {
	FullName    string `json:"full_name"`
	Description string `json:"description"`
	Website     string `json:"website"`
	Location    string `json:"location"`
}

func editOrg(c *context.APIContext, form editOrgRequest) {
	org := c.Org.Organization
	if !org.IsOwnedBy(c.User.ID) {
		c.Status(http.StatusForbidden)
		return
	}

	err := database.Handle.Users().Update(
		c.Req.Context(),
		c.Org.Organization.ID,
		database.UpdateUserOptions{
			FullName:    &form.FullName,
			Website:     &form.Website,
			Location:    &form.Location,
			Description: &form.Description,
		},
	)
	if err != nil {
		c.Error(err, "update organization")
		return
	}

	org, err = database.GetOrgByName(org.Name)
	if err != nil {
		c.Error(err, "get organization")
		return
	}
	c.JSONSuccess(toOrganization(org))
}
