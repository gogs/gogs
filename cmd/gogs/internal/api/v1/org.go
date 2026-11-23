package v1

import (
	"net/http"

	"gogs.io/gogs/internal/api/v1types"
	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/database"
)

type createOrgRequest struct {
	UserName    string `json:"username" binding:"Required;AlphaDashDot;MaxSize(35)"`
	FullName    string `json:"full_name"`
	Description string `json:"description"`
	Website     string `json:"website"`
	Location    string `json:"location"`
}

func createOrgForUser(c *apiContext, links database.LinkOptions, repositoryRoot string, apiForm createOrgRequest, user *database.User) {
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
	if err := database.Handle.Organizations().Create(c.Request().Context(), repositoryRoot, org, user); err != nil {
		if database.IsErrUserAlreadyExist(err) ||
			database.IsErrNameNotAllowed(err) ||
			database.IsErrOrgOwnerIsOrganization(err) {
			c.ErrorStatus(http.StatusUnprocessableEntity, err)
		} else {
			c.Error(err, "create organization")
		}
		return
	}

	c.JSON(201, toOrganization(links, org))
}

func listOrgsOfUser(c *apiContext, links database.LinkOptions, u *database.User, all bool) {
	orgs, err := database.Handle.Organizations().List(
		c.Request().Context(),
		database.ListOrgsOptions{
			MemberID:              u.ID,
			IncludePrivateMembers: all,
		},
	)
	if err != nil {
		c.Error(err, "list organizations")
		return
	}

	apiOrgs := make([]*v1types.Organization, len(orgs))
	for i := range orgs {
		apiOrgs[i] = toOrganization(links, orgs[i])
	}
	c.JSONSuccess(&apiOrgs)
}

func listMyOrgs(c *apiContext, links database.LinkOptions) {
	listOrgsOfUser(c, links, c.User, true)
}

func createMyOrg(c *apiContext, links database.LinkOptions, repository conf.RepositoryOpts, apiForm createOrgRequest) {
	createOrgForUser(c, links, repository.Root, apiForm, c.User)
}

func listUserOrgs(c *apiContext, links database.LinkOptions) {
	u := getUserByParams(c)
	if c.Written() {
		return
	}
	listOrgsOfUser(c, links, u, false)
}

func getOrg(c *apiContext, links database.LinkOptions) {
	c.JSONSuccess(toOrganization(links, c.Org.Organization))
}

type editOrgRequest struct {
	FullName    string `json:"full_name"`
	Description string `json:"description"`
	Website     string `json:"website"`
	Location    string `json:"location"`
}

func editOrg(c *apiContext, links database.LinkOptions, form editOrgRequest) {
	org := c.Org.Organization
	if !database.Handle.Organizations().IsOwner(c.Request().Context(), org.ID, c.User.ID) {
		c.Status(http.StatusForbidden)
		return
	}

	err := database.Handle.Users().Update(
		c.Request().Context(),
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

	org, err = database.Handle.Organizations().GetByName(c.Request().Context(), org.Name)
	if err != nil {
		c.Error(err, "get organization")
		return
	}
	c.JSONSuccess(toOrganization(links, org))
}
