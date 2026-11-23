package web

import (
	"context"
	"net/http"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/flamego/flamego"
	"unknwon.dev/x/logx"

	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/i18n"
	"gogs.io/gogs/internal/markup"
)

// orgMembership is the organization named by the `org` path parameter and
// the role of the authenticated user in it.
type orgMembership struct {
	Org *database.User
	// IsOwner is true for owners and site admins, who have the access of owners.
	IsOwner bool
}

// withOrgMembership maps the organization named by the `org` path parameter
// as *orgMembership. Only members see the pages of an organization.
func withOrgMembership(logger *logx.Logger, c flamego.Context, viewer *database.User) {
	ctx := c.Request().Context()
	w := c.ResponseWriter()
	name := c.Param("org")
	org, err := database.Handle.Users().GetByUsername(c.Request().Context(), name)
	if err != nil && !database.IsErrUserNotExist(err) {
		logger.Error("Failed to get organization by name", "orgName", name, "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, errors.Wrap(err, "get organization"))
		return
	}
	if err != nil || !org.IsOrganization() {
		writeErrorResponse(w, http.StatusNotFound, errors.New("organization does not exist"))
		return
	}

	isOwner := viewer.IsAdmin || database.Handle.Organizations().IsOwner(ctx, org.ID, viewer.ID)
	if !isOwner && !database.Handle.Organizations().IsMember(ctx, org.ID, viewer.ID) {
		writeErrorResponse(w, http.StatusNotFound, errors.New("organization does not exist"))
		return
	}
	c.Map(&orgMembership{Org: org, IsOwner: isOwner})
}

// visibleTeams returns the teams of the organization the authenticated user can
// see: every team for owners, their own teams for other members.
func (m *orgMembership) visibleTeams(ctx context.Context, userID int64) ([]*database.Team, error) {
	if m.IsOwner {
		return database.Handle.Teams().ListByOrg(ctx, m.Org.ID)
	}
	return database.Handle.Teams().ListByMember(ctx, m.Org.ID, userID)
}

type orgMember struct {
	Name      string `json:"name"`
	FullName  string `json:"fullName,omitempty"`
	AvatarURL string `json:"avatarURL"`
	IsPublic  bool   `json:"isPublic"`
	IsOwner   bool   `json:"isOwner"`
}

type orgMembers struct {
	Items []orgMember `json:"items"`
}

func getOrgMembers(c flamego.Context, links database.LinkOptions, logger *logx.Logger, m *orgMembership) (statusCode int, resp *orgMembers, err error) {
	ctx := c.Request().Context()
	org := m.Org
	members, err := database.Handle.Organizations().ListMembers(ctx, org.ID, 0)
	if err != nil {
		logger.Error("Failed to get members of organization", "orgID", org.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "get members")
	}
	resp = &orgMembers{Items: make([]orgMember, 0, len(members))}
	for _, u := range members {
		member := toExploreUser(links, u)
		resp.Items = append(resp.Items, orgMember{
			Name:      member.Name,
			FullName:  member.FullName,
			AvatarURL: member.AvatarURL,
			IsPublic:  database.Handle.Organizations().IsPublicMember(ctx, org.ID, u.ID),
			IsOwner:   database.Handle.Organizations().IsOwner(ctx, org.ID, u.ID),
		})
	}
	return http.StatusOK, resp, nil
}

// getUserByParam returns the user with the name, or the status code and the
// error to answer with, which is localized when the user does not exist.
func getUserByParam(ctx context.Context, l i18n.Locale, name string) (*database.User, int, error) {
	u, err := database.Handle.Users().GetByUsername(ctx, name)
	if err != nil {
		if database.IsErrUserNotExist(err) {
			return nil, http.StatusNotFound, errors.New(l.Tr("form::user_not_exist"))
		}
		return nil, http.StatusInternalServerError, errors.Wrap(err, "get user")
	}
	return u, 0, nil
}

// setOrgMemberPublic makes the membership of a user public or private. Members
// can change their own, owners anyone's.
func setOrgMemberPublic(logger *logx.Logger, c flamego.Context, l i18n.Locale, viewer *database.User, m *orgMembership, public bool) (statusCode int, resp any, err error) {
	ctx := c.Request().Context()
	member, code, err := getUserByParam(c.Request().Context(), l, c.Param("username"))
	if err != nil {
		if code == http.StatusInternalServerError {
			logger.Error("Failed to get user by username", "username", c.Param("username"), "error", err)
		}
		return code, nil, err
	}
	if member.ID != viewer.ID && !m.IsOwner {
		return http.StatusForbidden, nil, errors.New("organization owner required")
	}
	if err = database.Handle.Organizations().SetMemberVisibility(ctx, m.Org.ID, member.ID, public); err != nil {
		logger.Error("Failed to change membership visibility", "public", public, "userID", member.ID, "orgID", m.Org.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "change membership visibility")
	}
	return http.StatusNoContent, nil, nil
}

func putOrgMemberPublic(logger *logx.Logger, c flamego.Context, l i18n.Locale, viewer *database.User, m *orgMembership) (statusCode int, resp any, err error) {
	return setOrgMemberPublic(logger, c, l, viewer, m, true)
}

func deleteOrgMemberPublic(logger *logx.Logger, c flamego.Context, l i18n.Locale, viewer *database.User, m *orgMembership) (statusCode int, resp any, err error) {
	return setOrgMemberPublic(logger, c, l, viewer, m, false)
}

// deleteOrgMember removes a user from the organization. Members can leave,
// owners can remove anyone, except the last owner.
func deleteOrgMember(logger *logx.Logger, c flamego.Context, l i18n.Locale, viewer *database.User, m *orgMembership) (statusCode int, resp any, err error) {
	ctx := c.Request().Context()
	member, code, err := getUserByParam(c.Request().Context(), l, c.Param("username"))
	if err != nil {
		if code == http.StatusInternalServerError {
			logger.Error("Failed to get user by username", "username", c.Param("username"), "error", err)
		}
		return code, nil, err
	}
	if member.ID != viewer.ID && !m.IsOwner {
		return http.StatusForbidden, nil, errors.New("organization owner required")
	}
	if err = database.Handle.Organizations().RemoveMember(ctx, m.Org.ID, member.ID); err != nil {
		if database.IsErrLastOrgOwner(err) {
			return http.StatusConflict, nil, errors.New(l.Tr("form::last_org_owner"))
		}
		logger.Error("Failed to remove member from organization", "userID", member.ID, "orgID", m.Org.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "remove member")
	}
	return http.StatusNoContent, nil, nil
}

// putOrgMember adds a user to the organization. Only owners can invite.
func putOrgMember(logger *logx.Logger, c flamego.Context, l i18n.Locale, m *orgMembership) (statusCode int, resp any, err error) {
	ctx := c.Request().Context()
	if !m.IsOwner {
		return http.StatusForbidden, nil, errors.New("organization owner required")
	}
	member, code, err := getUserByParam(c.Request().Context(), l, c.Param("username"))
	if err != nil {
		if code == http.StatusInternalServerError {
			logger.Error("Failed to get user by username", "username", c.Param("username"), "error", err)
		}
		return code, nil, err
	}
	if member.IsOrganization() {
		return http.StatusNotFound, nil, errors.New(l.Tr("form::user_not_exist"))
	}
	if err = database.Handle.Organizations().AddMember(ctx, m.Org.ID, member.ID); err != nil {
		logger.Error("Failed to add member to organization", "userID", member.ID, "orgID", m.Org.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "add member")
	}
	return http.StatusNoContent, nil, nil
}

type orgTeamMember struct {
	Name      string `json:"name"`
	FullName  string `json:"fullName,omitempty"`
	AvatarURL string `json:"avatarURL"`
}

type orgTeamSummary struct {
	Name        string          `json:"name"`
	LowerName   string          `json:"lowerName"`
	MemberCount int             `json:"memberCount"`
	RepoCount   int             `json:"repoCount"`
	Members     []orgTeamMember `json:"members"`
	// ViewerIsMember is true when the authenticated user is on the team.
	ViewerIsMember bool `json:"viewerIsMember"`
}

type orgTeams struct {
	Items []orgTeamSummary `json:"items"`
}

func toOrgTeamMembers(links database.LinkOptions, users []*database.User) []orgTeamMember {
	items := make([]orgTeamMember, 0, len(users))
	for _, u := range users {
		member := toExploreUser(links, u)
		items = append(items, orgTeamMember{Name: member.Name, FullName: member.FullName, AvatarURL: member.AvatarURL})
	}
	return items
}

func getOrgTeams(c flamego.Context, links database.LinkOptions, logger *logx.Logger, viewer *database.User, m *orgMembership) (statusCode int, resp *orgTeams, err error) {
	ctx := c.Request().Context()
	teams, err := m.visibleTeams(ctx, viewer.ID)
	if err != nil {
		logger.Error("Failed to get teams of organization", "orgID", m.Org.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "get teams")
	}
	resp = &orgTeams{Items: make([]orgTeamSummary, 0, len(teams))}
	for _, t := range teams {
		members, err := database.Handle.Teams().ListMembers(ctx, t.ID)
		if err != nil {
			logger.Error("Failed to get members of team", "teamID", t.ID, "error", err)
			return http.StatusInternalServerError, nil, errors.Wrap(err, "get team members")
		}
		resp.Items = append(resp.Items, orgTeamSummary{
			Name:           t.Name,
			LowerName:      t.LowerName,
			MemberCount:    t.NumMembers,
			RepoCount:      t.NumRepos,
			Members:        toOrgTeamMembers(links, members),
			ViewerIsMember: database.Handle.Teams().IsMember(ctx, t.OrgID, t.ID, viewer.ID),
		})
	}
	return http.StatusOK, resp, nil
}

// orgTeam is the team named by the `team` path parameter.
type orgTeam struct {
	*database.Team
}

// withOrgTeam maps the team named by the `team` path parameter as *orgTeam.
// Members other than owners only see their own teams.
func withOrgTeam(logger *logx.Logger, c flamego.Context, viewer *database.User, m *orgMembership) {
	ctx := c.Request().Context()
	w := c.ResponseWriter()
	teams, err := m.visibleTeams(ctx, viewer.ID)
	if err != nil {
		logger.Error("Failed to get teams of organization", "orgID", m.Org.ID, "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, errors.Wrap(err, "get teams"))
		return
	}
	name := strings.ToLower(c.Param("team"))
	for _, t := range teams {
		if t.LowerName == name {
			c.Map(&orgTeam{t})
			return
		}
	}
	writeErrorResponse(w, http.StatusNotFound, errors.New("team does not exist"))
}

type orgTeamDetail struct {
	Name        string `json:"name"`
	LowerName   string `json:"lowerName"`
	Description string `json:"description,omitempty"`
	// PermissionHTML describes the access the team grants, sanitized for
	// rendering as HTML.
	PermissionHTML string `json:"permissionHTML"`
	MemberCount    int    `json:"memberCount"`
	RepoCount      int    `json:"repoCount"`
	// Permission is "read", "write" or "admin", or "owner" for the owners
	// team.
	Permission string `json:"permission"`
	// IsOwnerTeam is true for the owners team, which has every repository.
	IsOwnerTeam    bool `json:"isOwnerTeam"`
	ViewerIsMember bool `json:"viewerIsMember"`
}

func getOrgTeam(c flamego.Context, l i18n.Locale, viewer *database.User, team *orgTeam) (statusCode int, resp *orgTeamDetail, err error) {
	ctx := c.Request().Context()
	var permission string
	switch {
	case team.IsOwnerTeam():
		permission = l.Tr("org::teams.owners_permission_desc")
	case team.Authorize == database.AccessModeRead:
		permission = l.Tr("org::teams.read_permission_desc")
	case team.Authorize == database.AccessModeWrite:
		permission = l.Tr("org::teams.write_permission_desc")
	case team.Authorize == database.AccessModeAdmin:
		permission = l.Tr("org::teams.admin_permission_desc")
	}
	return http.StatusOK, &orgTeamDetail{
		Name:        team.Name,
		LowerName:   team.LowerName,
		Description: team.Description,
		// Translations are community-maintained, so they are sanitized.
		PermissionHTML: markup.Sanitize(permission),
		MemberCount:    team.NumMembers,
		RepoCount:      team.NumRepos,
		Permission:     team.Authorize.String(),
		IsOwnerTeam:    team.IsOwnerTeam(),
		ViewerIsMember: database.Handle.Teams().IsMember(ctx, team.OrgID, team.ID, viewer.ID),
	}, nil
}

type orgTeamMembers struct {
	Items []orgTeamMember `json:"items"`
}

func getOrgTeamMembers(c flamego.Context, links database.LinkOptions, logger *logx.Logger, team *orgTeam) (statusCode int, resp *orgTeamMembers, err error) {
	ctx := c.Request().Context()
	members, err := database.Handle.Teams().ListMembers(ctx, team.ID)
	if err != nil {
		logger.Error("Failed to get members of team", "teamID", team.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "get team members")
	}
	return http.StatusOK, &orgTeamMembers{Items: toOrgTeamMembers(links, members)}, nil
}

// putOrgTeamMember adds a user to the team, which also makes them a member of
// the organization. Only owners can add anyone, themselves included.
func putOrgTeamMember(logger *logx.Logger, c flamego.Context, l i18n.Locale, m *orgMembership, team *orgTeam) (statusCode int, resp any, err error) {
	ctx := c.Request().Context()
	if !m.IsOwner {
		return http.StatusForbidden, nil, errors.New("organization owner required")
	}
	member, code, err := getUserByParam(c.Request().Context(), l, c.Param("username"))
	if err != nil {
		if code == http.StatusInternalServerError {
			logger.Error("Failed to get user by username", "username", c.Param("username"), "error", err)
		}
		return code, nil, err
	}
	if member.IsOrganization() {
		return http.StatusNotFound, nil, errors.New(l.Tr("form::user_not_exist"))
	}
	if err = database.Handle.Teams().AddMember(ctx, team.OrgID, team.ID, member.ID); err != nil {
		logger.Error("Failed to add member to team", "userID", member.ID, "teamID", team.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "add team member")
	}
	return http.StatusNoContent, nil, nil
}

// deleteOrgTeamMember removes a user from the team. Members can leave, owners
// can remove anyone, except the last member of the owners team.
func deleteOrgTeamMember(logger *logx.Logger, c flamego.Context, l i18n.Locale, viewer *database.User, m *orgMembership, team *orgTeam) (statusCode int, resp any, err error) {
	ctx := c.Request().Context()
	member, code, err := getUserByParam(c.Request().Context(), l, c.Param("username"))
	if err != nil {
		if code == http.StatusInternalServerError {
			logger.Error("Failed to get user by username", "username", c.Param("username"), "error", err)
		}
		return code, nil, err
	}
	if member.ID != viewer.ID && !m.IsOwner {
		return http.StatusForbidden, nil, errors.New("organization owner required")
	}
	if err = database.Handle.Teams().RemoveMember(ctx, team.OrgID, team.ID, member.ID); err != nil {
		if database.IsErrLastOrgOwner(err) {
			return http.StatusConflict, nil, errors.New(l.Tr("form::last_org_owner"))
		}
		logger.Error("Failed to remove member from team", "userID", member.ID, "teamID", team.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "remove team member")
	}
	return http.StatusNoContent, nil, nil
}

type orgTeamRepo struct {
	Name      string `json:"name"`
	IsPrivate bool   `json:"isPrivate"`
	IsFork    bool   `json:"isFork"`
	IsMirror  bool   `json:"isMirror"`
}

type orgTeamRepos struct {
	Items []orgTeamRepo `json:"items"`
}

func getOrgTeamRepos(c flamego.Context, logger *logx.Logger, team *orgTeam) (statusCode int, resp *orgTeamRepos, err error) {
	ctx := c.Request().Context()
	repos, err := database.Handle.Teams().ListRepos(ctx, team.ID)
	if err != nil {
		logger.Error("Failed to get repositories of team", "teamID", team.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "get team repositories")
	}
	resp = &orgTeamRepos{Items: make([]orgTeamRepo, 0, len(repos))}
	for _, repo := range repos {
		resp.Items = append(resp.Items, orgTeamRepo{
			Name:      repo.Name,
			IsPrivate: repo.IsPrivate,
			IsFork:    repo.IsFork,
			IsMirror:  repo.IsMirror,
		})
	}
	return http.StatusOK, resp, nil
}

// teamRepoChangeAllowed returns an error response unless the authenticated user
// may change the repositories of the team: owners, on any team other than the
// owners team, which has every repository.
func teamRepoChangeAllowed(m *orgMembership, team *orgTeam) (int, error) {
	if !m.IsOwner {
		return http.StatusForbidden, errors.New("organization owner required")
	}
	if team.IsOwnerTeam() {
		return http.StatusForbidden, errors.New("the owners team has every repository")
	}
	return 0, nil
}

func putOrgTeamRepo(logger *logx.Logger, c flamego.Context, l i18n.Locale, m *orgMembership, team *orgTeam) (statusCode int, resp any, err error) {
	ctx := c.Request().Context()
	if code, err := teamRepoChangeAllowed(m, team); err != nil {
		return code, nil, err
	}
	repo, err := database.Handle.Repositories().GetByName(c.Request().Context(), m.Org.ID, c.Param("repo"))
	if err != nil {
		if database.IsErrRepoNotExist(err) {
			return http.StatusNotFound, nil, errors.New(l.Tr("org::teams.add_nonexistent_repo"))
		}
		logger.Error("Failed to get repository of organization", "repoName", c.Param("repo"), "orgID", m.Org.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "get repository")
	}
	if err = database.Handle.Teams().AddRepo(ctx, team.Team, repo); err != nil {
		logger.Error("Failed to add repository to team", "repoID", repo.ID, "teamID", team.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "add team repository")
	}
	return http.StatusNoContent, nil, nil
}

func deleteOrgTeamRepo(logger *logx.Logger, c flamego.Context, l i18n.Locale, m *orgMembership, team *orgTeam) (statusCode int, resp any, err error) {
	ctx := c.Request().Context()
	if code, err := teamRepoChangeAllowed(m, team); err != nil {
		return code, nil, err
	}
	repo, err := database.Handle.Repositories().GetByName(c.Request().Context(), m.Org.ID, c.Param("repo"))
	if err != nil {
		if database.IsErrRepoNotExist(err) {
			return http.StatusNotFound, nil, errors.New("repository does not exist")
		}
		logger.Error("Failed to get repository of organization", "repoName", c.Param("repo"), "orgID", m.Org.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "get repository")
	}
	if err = database.Handle.Teams().RemoveRepo(ctx, team.Team, repo.ID); err != nil {
		logger.Error("Failed to remove repository from team", "repoID", repo.ID, "teamID", team.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "remove team repository")
	}
	return http.StatusNoContent, nil, nil
}

type orgTeamRequest struct {
	TeamName    string `json:"teamName" validate:"required,alphadashdot,max=30"`
	Description string `json:"description" validate:"max=255"`
	// Permission is "read", "write" or "admin". The owners team ignores it.
	Permission string `json:"permission"`
}

type orgTeamResponse struct {
	LowerName string `json:"lowerName"`
}

// teamPermissions are the access levels a team other than the owners team
// can grant.
var teamPermissions = map[string]database.AccessMode{
	"read":  database.AccessModeRead,
	"write": database.AccessModeWrite,
	"admin": database.AccessModeAdmin,
}

// teamNameError returns the response for a team name the organization cannot
// use, or nil when err is about something else.
func teamNameError(l i18n.Locale, err error) (int, *bindingErrorResponse) {
	var msg string
	var code int
	switch {
	case database.IsErrTeamAlreadyExist(err):
		code, msg = http.StatusUnprocessableEntity, l.Tr("form::team_name_been_taken")
	case database.IsErrNameNotAllowed(err):
		code, msg = http.StatusBadRequest, l.Tr("org::form.team_name_not_allowed", "name", err.(database.ErrNameNotAllowed).Value())
	default:
		return 0, nil
	}
	return code, &bindingErrorResponse{Fields: fieldErrors{"teamName": &msg}}
}

func postOrgTeam(c flamego.Context, logger *logx.Logger, l i18n.Locale, m *orgMembership, req orgTeamRequest) (statusCode int, resp any, err error) {
	ctx := c.Request().Context()
	if !m.IsOwner {
		return http.StatusForbidden, nil, errors.New("organization owner required")
	}
	permission, ok := teamPermissions[req.Permission]
	if !ok {
		return http.StatusBadRequest, nil, errors.New("invalid permission")
	}
	t := &database.Team{
		OrgID:       m.Org.ID,
		Name:        req.TeamName,
		Description: req.Description,
		Authorize:   permission,
	}
	if err = database.Handle.Teams().Create(ctx, t); err != nil {
		if code, resp := teamNameError(l, err); resp != nil {
			return code, resp, nil
		}
		logger.Error("Failed to create team", "teamName", req.TeamName, "orgID", m.Org.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "create team")
	}
	return http.StatusCreated, &orgTeamResponse{LowerName: t.LowerName}, nil
}

// patchOrgTeam changes the name, the description and the permission of a
// team. The owners team keeps its name and permission.
func patchOrgTeam(c flamego.Context, logger *logx.Logger, l i18n.Locale, m *orgMembership, team *orgTeam, req orgTeamRequest) (statusCode int, resp any, err error) {
	ctx := c.Request().Context()
	if !m.IsOwner {
		return http.StatusForbidden, nil, errors.New("organization owner required")
	}
	t := team.Team
	authChanged := false
	if !t.IsOwnerTeam() {
		permission, ok := teamPermissions[req.Permission]
		if !ok {
			return http.StatusBadRequest, nil, errors.New("invalid permission")
		}
		// Renaming a team to a reserved name would make its page unreachable.
		if err = database.IsUsableTeamName(req.TeamName); err != nil {
			code, resp := teamNameError(l, err)
			return code, resp, nil
		}
		t.Name = req.TeamName
		authChanged = t.Authorize != permission
		t.Authorize = permission
	}
	t.Description = req.Description
	if err = database.Handle.Teams().Update(ctx, t, authChanged); err != nil {
		if code, resp := teamNameError(l, err); resp != nil {
			return code, resp, nil
		}
		logger.Error("Failed to update team", "teamID", t.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "update team")
	}
	return http.StatusOK, &orgTeamResponse{LowerName: t.LowerName}, nil
}

func deleteOrgTeam(c flamego.Context, logger *logx.Logger, m *orgMembership, team *orgTeam) (statusCode int, resp any, err error) {
	ctx := c.Request().Context()
	if !m.IsOwner {
		return http.StatusForbidden, nil, errors.New("organization owner required")
	}
	// An organization always needs its owners team.
	if team.IsOwnerTeam() {
		return http.StatusForbidden, nil, errors.New("the owners team cannot be deleted")
	}
	if err = database.Handle.Teams().Delete(ctx, team.Team); err != nil {
		logger.Error("Failed to delete team", "teamID", team.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "delete team")
	}
	return http.StatusNoContent, nil, nil
}
