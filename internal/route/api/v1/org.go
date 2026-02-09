package v1

import (
	"net/http"

	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/route/api/v1/types"
)

type CreateOrgRequest struct {
	UserName    string `json:"username" binding:"Required"`
	FullName    string `json:"full_name"`
	Description string `json:"description"`
	Website     string `json:"website"`
	Location    string `json:"location"`
}

type EditOrgRequest struct {
	FullName    string `json:"full_name"`
	Description string `json:"description"`
	Website     string `json:"website"`
	Location    string `json:"location"`
}

func CreateOrgForUser(c *context.APIContext, apiForm CreateOrgRequest, user *database.User) {
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

	c.JSON(201, ToOrganization(org))
}

func listUserOrgs(c *context.APIContext, u *database.User, all bool) {
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
		apiOrgs[i] = ToOrganization(orgs[i])
	}
	c.JSONSuccess(&apiOrgs)
}

func ListMyOrgs(c *context.APIContext) {
	listUserOrgs(c, c.User, true)
}

func CreateMyOrg(c *context.APIContext, apiForm CreateOrgRequest) {
	CreateOrgForUser(c, apiForm, c.User)
}

func ListUserOrgs(c *context.APIContext) {
	u := GetUserByParams(c)
	if c.Written() {
		return
	}
	listUserOrgs(c, u, false)
}

func GetOrg(c *context.APIContext) {
	c.JSONSuccess(ToOrganization(c.Org.Organization))
}

func EditOrg(c *context.APIContext, form EditOrgRequest) {
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
	c.JSONSuccess(ToOrganization(org))
}
