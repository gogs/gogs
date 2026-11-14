package web

import (
	"context"
	"net/http"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/flamego/flamego"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/i18n"
	"gogs.io/gogs/internal/markup"
)

// orgMembership is the organization named by the `org` path parameter and
// the role of the signed-in user in it.
type orgMembership struct {
	Org *database.User
	// IsOwner is true for owners and site admins, who have the access of owners.
	IsOwner bool
}

// withOrgMembership maps the organization named by the `org` path parameter
// as *orgMembership. Like the server-rendered pages, only members see the
// pages of an organization.
func withOrgMembership(c flamego.Context, user *database.User) {
	w := c.ResponseWriter()
	name := c.Param("org")
	org, err := database.Handle.Users().GetByUsername(c.Request().Context(), name)
	if err != nil && !database.IsErrUserNotExist(err) {
		log.Error("withOrgMembership: get user by username %q: %v", name, err)
		writeErrorResponse(w, http.StatusInternalServerError, errors.Wrap(err, "get organization"))
		return
	}
	if err != nil || !org.IsOrganization() {
		writeErrorResponse(w, http.StatusNotFound, errors.New("organization does not exist"))
		return
	}

	isOwner := user.IsAdmin || org.IsOwnedBy(user.ID)
	if !isOwner && !org.IsOrgMember(user.ID) {
		writeErrorResponse(w, http.StatusNotFound, errors.New("organization does not exist"))
		return
	}
	c.Map(&orgMembership{Org: org, IsOwner: isOwner})
}

// visibleTeams returns the teams of the organization the signed-in user can
// see: every team for owners, their own teams for other members.
func (m *orgMembership) visibleTeams(userID int64) ([]*database.Team, error) {
	if m.IsOwner {
		err := m.Org.GetTeams()
		return m.Org.Teams, err
	}
	return m.Org.GetUserTeams(userID)
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

func getOrgMembers(m *orgMembership) (statusCode int, resp *orgMembers, err error) {
	org := m.Org
	if err = org.GetMembers(0); err != nil {
		log.Error("getOrgMembers: get members of organization %d: %v", org.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "get members")
	}
	resp = &orgMembers{Items: make([]orgMember, 0, len(org.Members))}
	for _, u := range org.Members {
		member := toExploreUser(u)
		resp.Items = append(resp.Items, orgMember{
			Name:      member.Name,
			FullName:  member.FullName,
			AvatarURL: member.AvatarURL,
			IsPublic:  u.IsPublicMember(org.ID),
			IsOwner:   u.IsUserOrgOwner(org.ID),
		})
	}
	return http.StatusOK, resp, nil
}

// getUserByParam returns the user named by the `username` path parameter, or
// a 404 response with the localized message of the server-rendered forms.
func getUserByParam(ctx context.Context, l i18n.Locale, name string) (*database.User, int, error) {
	u, err := database.Handle.Users().GetByUsername(ctx, name)
	if err != nil {
		if database.IsErrUserNotExist(err) {
			return nil, http.StatusNotFound, errors.New(l.Tr("form.user_not_exist"))
		}
		return nil, http.StatusInternalServerError, errors.Wrap(err, "get user")
	}
	return u, 0, nil
}

// setOrgMemberPublic makes the membership of a user public or private. Members
// can change their own, owners anyone's.
func setOrgMemberPublic(c flamego.Context, l i18n.Locale, user *database.User, m *orgMembership, public bool) (statusCode int, resp any, err error) {
	member, code, err := getUserByParam(c.Request().Context(), l, c.Param("username"))
	if err != nil {
		if code == http.StatusInternalServerError {
			log.Error("setOrgMemberPublic: %v", err)
		}
		return code, nil, err
	}
	if member.ID != user.ID && !m.IsOwner {
		return http.StatusForbidden, nil, errors.New("organization owner required")
	}
	if err = database.ChangeOrgUserStatus(m.Org.ID, member.ID, public); err != nil {
		log.Error("setOrgMemberPublic: set public=%t for user %d in organization %d: %v", public, member.ID, m.Org.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "change membership visibility")
	}
	return http.StatusNoContent, nil, nil
}

func putOrgMemberPublic(c flamego.Context, l i18n.Locale, user *database.User, m *orgMembership) (statusCode int, resp any, err error) {
	return setOrgMemberPublic(c, l, user, m, true)
}

func deleteOrgMemberPublic(c flamego.Context, l i18n.Locale, user *database.User, m *orgMembership) (statusCode int, resp any, err error) {
	return setOrgMemberPublic(c, l, user, m, false)
}

// deleteOrgMember removes a user from the organization. Members can leave,
// owners can remove anyone, except the last owner.
func deleteOrgMember(c flamego.Context, l i18n.Locale, user *database.User, m *orgMembership) (statusCode int, resp any, err error) {
	member, code, err := getUserByParam(c.Request().Context(), l, c.Param("username"))
	if err != nil {
		if code == http.StatusInternalServerError {
			log.Error("deleteOrgMember: %v", err)
		}
		return code, nil, err
	}
	if member.ID != user.ID && !m.IsOwner {
		return http.StatusForbidden, nil, errors.New("organization owner required")
	}
	if err = m.Org.RemoveMember(member.ID); err != nil {
		if database.IsErrLastOrgOwner(err) {
			return http.StatusConflict, nil, errors.New(l.Tr("form.last_org_owner"))
		}
		log.Error("deleteOrgMember: remove user %d from organization %d: %v", member.ID, m.Org.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "remove member")
	}
	return http.StatusNoContent, nil, nil
}

// putOrgMember adds a user to the organization. Only owners can invite.
func putOrgMember(c flamego.Context, l i18n.Locale, m *orgMembership) (statusCode int, resp any, err error) {
	if !m.IsOwner {
		return http.StatusForbidden, nil, errors.New("organization owner required")
	}
	member, code, err := getUserByParam(c.Request().Context(), l, c.Param("username"))
	if err != nil {
		if code == http.StatusInternalServerError {
			log.Error("putOrgMember: %v", err)
		}
		return code, nil, err
	}
	if member.IsOrganization() {
		return http.StatusNotFound, nil, errors.New(l.Tr("form.user_not_exist"))
	}
	if err = m.Org.AddMember(member.ID); err != nil {
		log.Error("putOrgMember: add user %d to organization %d: %v", member.ID, m.Org.ID, err)
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
	// ViewerIsMember is true when the signed-in user is on the team.
	ViewerIsMember bool `json:"viewerIsMember"`
}

type orgTeams struct {
	Items []orgTeamSummary `json:"items"`
}

func toOrgTeamMembers(users []*database.User) []orgTeamMember {
	items := make([]orgTeamMember, 0, len(users))
	for _, u := range users {
		member := toExploreUser(u)
		items = append(items, orgTeamMember{Name: member.Name, FullName: member.FullName, AvatarURL: member.AvatarURL})
	}
	return items
}

func getOrgTeams(user *database.User, m *orgMembership) (statusCode int, resp *orgTeams, err error) {
	teams, err := m.visibleTeams(user.ID)
	if err != nil {
		log.Error("getOrgTeams: get teams of organization %d: %v", m.Org.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "get teams")
	}
	resp = &orgTeams{Items: make([]orgTeamSummary, 0, len(teams))}
	for _, t := range teams {
		if err = t.GetMembers(); err != nil {
			log.Error("getOrgTeams: get members of team %d: %v", t.ID, err)
			return http.StatusInternalServerError, nil, errors.Wrap(err, "get team members")
		}
		resp.Items = append(resp.Items, orgTeamSummary{
			Name:           t.Name,
			LowerName:      t.LowerName,
			MemberCount:    t.NumMembers,
			RepoCount:      t.NumRepos,
			Members:        toOrgTeamMembers(t.Members),
			ViewerIsMember: t.IsMember(user.ID),
		})
	}
	return http.StatusOK, resp, nil
}

// orgTeam is the team named by the `team` path parameter.
type orgTeam struct {
	*database.Team
}

// withOrgTeam maps the team named by the `team` path parameter as *orgTeam.
// Like the server-rendered pages, members other than owners only see their
// own teams.
func withOrgTeam(c flamego.Context, user *database.User, m *orgMembership) {
	w := c.ResponseWriter()
	teams, err := m.visibleTeams(user.ID)
	if err != nil {
		log.Error("withOrgTeam: get teams of organization %d: %v", m.Org.ID, err)
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

func getOrgTeam(l i18n.Locale, user *database.User, team *orgTeam) (statusCode int, resp *orgTeamDetail, err error) {
	var permission string
	switch {
	case team.IsOwnerTeam():
		permission = l.Tr("org.teams.owners_permission_desc")
	case team.Authorize == database.AccessModeRead:
		permission = l.Tr("org.teams.read_permission_desc")
	case team.Authorize == database.AccessModeWrite:
		permission = l.Tr("org.teams.write_permission_desc")
	case team.Authorize == database.AccessModeAdmin:
		permission = l.Tr("org.teams.admin_permission_desc")
	}
	return http.StatusOK, &orgTeamDetail{
		Name:        team.Name,
		LowerName:   team.LowerName,
		Description: team.Description,
		// Translations are community-maintained, so sanitize them like the
		// server-rendered page did.
		PermissionHTML: markup.Sanitize(permission),
		MemberCount:    team.NumMembers,
		RepoCount:      team.NumRepos,
		Permission:     team.Authorize.String(),
		IsOwnerTeam:    team.IsOwnerTeam(),
		ViewerIsMember: team.IsMember(user.ID),
	}, nil
}

type orgTeamMembers struct {
	Items []orgTeamMember `json:"items"`
}

func getOrgTeamMembers(team *orgTeam) (statusCode int, resp *orgTeamMembers, err error) {
	if err = team.GetMembers(); err != nil {
		log.Error("getOrgTeamMembers: get members of team %d: %v", team.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "get team members")
	}
	return http.StatusOK, &orgTeamMembers{Items: toOrgTeamMembers(team.Members)}, nil
}

// putOrgTeamMember adds a user to the team, which also makes them a member of
// the organization. Only owners can add anyone, themselves included.
func putOrgTeamMember(c flamego.Context, l i18n.Locale, m *orgMembership, team *orgTeam) (statusCode int, resp any, err error) {
	if !m.IsOwner {
		return http.StatusForbidden, nil, errors.New("organization owner required")
	}
	member, code, err := getUserByParam(c.Request().Context(), l, c.Param("username"))
	if err != nil {
		if code == http.StatusInternalServerError {
			log.Error("putOrgTeamMember: %v", err)
		}
		return code, nil, err
	}
	if member.IsOrganization() {
		return http.StatusNotFound, nil, errors.New(l.Tr("form.user_not_exist"))
	}
	if err = team.AddMember(member.ID); err != nil {
		log.Error("putOrgTeamMember: add user %d to team %d: %v", member.ID, team.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "add team member")
	}
	return http.StatusNoContent, nil, nil
}

// deleteOrgTeamMember removes a user from the team. Members can leave, owners
// can remove anyone, except the last member of the owners team.
func deleteOrgTeamMember(c flamego.Context, l i18n.Locale, user *database.User, m *orgMembership, team *orgTeam) (statusCode int, resp any, err error) {
	member, code, err := getUserByParam(c.Request().Context(), l, c.Param("username"))
	if err != nil {
		if code == http.StatusInternalServerError {
			log.Error("deleteOrgTeamMember: %v", err)
		}
		return code, nil, err
	}
	if member.ID != user.ID && !m.IsOwner {
		return http.StatusForbidden, nil, errors.New("organization owner required")
	}
	if err = team.RemoveMember(member.ID); err != nil {
		if database.IsErrLastOrgOwner(err) {
			return http.StatusConflict, nil, errors.New(l.Tr("form.last_org_owner"))
		}
		log.Error("deleteOrgTeamMember: remove user %d from team %d: %v", member.ID, team.ID, err)
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

func getOrgTeamRepos(team *orgTeam) (statusCode int, resp *orgTeamRepos, err error) {
	if err = team.GetRepositories(); err != nil {
		log.Error("getOrgTeamRepos: get repositories of team %d: %v", team.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "get team repositories")
	}
	resp = &orgTeamRepos{Items: make([]orgTeamRepo, 0, len(team.Repos))}
	for _, repo := range team.Repos {
		resp.Items = append(resp.Items, orgTeamRepo{
			Name:      repo.Name,
			IsPrivate: repo.IsPrivate,
			IsFork:    repo.IsFork,
			IsMirror:  repo.IsMirror,
		})
	}
	return http.StatusOK, resp, nil
}

// teamRepoChangeAllowed returns an error response unless the signed-in user
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

func putOrgTeamRepo(c flamego.Context, l i18n.Locale, m *orgMembership, team *orgTeam) (statusCode int, resp any, err error) {
	if code, err := teamRepoChangeAllowed(m, team); err != nil {
		return code, nil, err
	}
	repo, err := database.GetRepositoryByName(m.Org.ID, c.Param("repo"))
	if err != nil {
		if database.IsErrRepoNotExist(err) {
			return http.StatusNotFound, nil, errors.New(l.Tr("org.teams.add_nonexistent_repo"))
		}
		log.Error("putOrgTeamRepo: get repository %q of organization %d: %v", c.Param("repo"), m.Org.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "get repository")
	}
	if err = team.AddRepository(repo); err != nil {
		log.Error("putOrgTeamRepo: add repository %d to team %d: %v", repo.ID, team.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "add team repository")
	}
	return http.StatusNoContent, nil, nil
}

func deleteOrgTeamRepo(c flamego.Context, l i18n.Locale, m *orgMembership, team *orgTeam) (statusCode int, resp any, err error) {
	if code, err := teamRepoChangeAllowed(m, team); err != nil {
		return code, nil, err
	}
	repo, err := database.GetRepositoryByName(m.Org.ID, c.Param("repo"))
	if err != nil {
		if database.IsErrRepoNotExist(err) {
			return http.StatusNotFound, nil, errors.New("repository does not exist")
		}
		log.Error("deleteOrgTeamRepo: get repository %q of organization %d: %v", c.Param("repo"), m.Org.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "get repository")
	}
	if err = team.RemoveRepository(repo.ID); err != nil {
		log.Error("deleteOrgTeamRepo: remove repository %d from team %d: %v", repo.ID, team.ID, err)
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
		code, msg = http.StatusUnprocessableEntity, l.Tr("form.team_name_been_taken")
	case database.IsErrNameNotAllowed(err):
		code, msg = http.StatusBadRequest, l.Tr("org.form.team_name_not_allowed", err.(database.ErrNameNotAllowed).Value())
	default:
		return 0, nil
	}
	return code, &bindingErrorResponse{Fields: fieldErrors{"teamName": &msg}}
}

func postOrgTeam(l i18n.Locale, m *orgMembership, req orgTeamRequest) (statusCode int, resp any, err error) {
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
	if err = database.NewTeam(t); err != nil {
		if code, resp := teamNameError(l, err); resp != nil {
			return code, resp, nil
		}
		log.Error("postOrgTeam: create team %q in organization %d: %v", req.TeamName, m.Org.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "create team")
	}
	return http.StatusCreated, &orgTeamResponse{LowerName: t.LowerName}, nil
}

// patchOrgTeam changes the name, the description and the permission of a
// team. The owners team keeps its name and permission.
func patchOrgTeam(l i18n.Locale, m *orgMembership, team *orgTeam, req orgTeamRequest) (statusCode int, resp any, err error) {
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
	if err = database.UpdateTeam(t, authChanged); err != nil {
		if code, resp := teamNameError(l, err); resp != nil {
			return code, resp, nil
		}
		log.Error("patchOrgTeam: update team %d: %v", t.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "update team")
	}
	return http.StatusOK, &orgTeamResponse{LowerName: t.LowerName}, nil
}

func deleteOrgTeam(m *orgMembership, team *orgTeam) (statusCode int, resp any, err error) {
	if !m.IsOwner {
		return http.StatusForbidden, nil, errors.New("organization owner required")
	}
	// An organization always needs its owners team.
	if team.IsOwnerTeam() {
		return http.StatusForbidden, nil, errors.New("the owners team cannot be deleted")
	}
	if err = database.DeleteTeam(team.Team); err != nil {
		log.Error("deleteOrgTeam: delete team %d: %v", team.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "delete team")
	}
	return http.StatusNoContent, nil, nil
}
