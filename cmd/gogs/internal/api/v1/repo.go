package v1

import (
	"net/http"

	"github.com/cockroachdb/errors"

	"gogs.io/gogs/internal/api/v1types"
	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/form"
	"gogs.io/gogs/internal/git"
	"gogs.io/gogs/internal/urlx"
)

func searchRepos(c *apiContext) {
	pageSize := toAllowedPageSize(c.QueryInt("limit"))
	opts := &database.SearchRepoOptions{
		Keyword:  c.Query("q"),
		OwnerID:  c.QueryInt64("uid"),
		OrderBy:  "id ASC", // Unique, so walking the pages lists every repository once.
		PageSize: pageSize,
		Page:     c.queryPage(pageSize),
	}

	// Check visibility.
	if c.IsLogged && opts.OwnerID > 0 {
		if c.User.ID == opts.OwnerID {
			opts.Private = true
		} else {
			u, err := database.Handle.Users().GetByID(c.Request().Context(), opts.OwnerID)
			if err != nil {
				c.JSON(http.StatusInternalServerError, map[string]any{
					"ok":    false,
					"error": err.Error(),
				})
				return
			}
			if u.IsOrganization() && database.Handle.Organizations().IsOwner(c.Request().Context(), u.ID, c.User.ID) {
				opts.Private = true
			}
			// FIXME: how about collaborators?
		}
	}

	repos, count, err := database.Handle.Repositories().Search(c.Request().Context(), *opts)
	if err != nil {
		c.JSON(http.StatusInternalServerError, map[string]any{
			"ok":    false,
			"error": err.Error(),
		})
		return
	}

	if err = database.Handle.Repositories().LoadAttributes(c.Request().Context(), repos...); err != nil {
		c.JSON(http.StatusInternalServerError, map[string]any{
			"ok":    false,
			"error": err.Error(),
		})
		return
	}

	basePerms, err := basePermissions(c, repos...)
	if err != nil {
		c.Logger.Error("Failed to get permissions of base repositories", "error", err)
		c.JSON(http.StatusInternalServerError, map[string]any{
			"ok":    false,
			"error": err.Error(),
		})
		return
	}

	results := make([]*v1types.Repository, len(repos))
	for i := range repos {
		results[i] = toRepository(repos[i], nil, basePerms[repos[i].ForkID])
	}

	c.SetLinkHeader(int(count), opts.PageSize)
	c.JSONSuccess(map[string]any{
		"ok":   true,
		"data": results,
	})
}

func listReposOfUser(c *apiContext, username string) {
	user, err := database.Handle.Users().GetByUsername(c.Request().Context(), username)
	if err != nil {
		c.NotFoundOrError(err, "get user by name")
		return
	}

	// Only list public repositories if user requests someone else's repository list,
	// or an organization the user isn't a member of.
	var ownRepos []*database.Repository
	if user.IsOrganization() {
		ownRepos, _, err = database.Handle.Organizations().ListAccessibleRepos(c.Request().Context(), user.ID, c.User.ID, 1, user.NumRepos)
	} else {
		ownRepos, err = database.Handle.Repositories().ListByOwner(c.Request().Context(), database.UserRepoOptions{
			UserID:   user.ID,
			Private:  c.User.ID == user.ID,
			Page:     1,
			PageSize: user.NumRepos,
		})
	}
	if err != nil {
		c.Error(err, "get user repositories")
		return
	}

	if err = database.Handle.Repositories().LoadAttributes(c.Request().Context(), ownRepos...); err != nil {
		c.Error(err, "load attributes")
		return
	}

	// Early return for querying other user's repositories
	if c.User.ID != user.ID {
		accessModes, err := database.Handle.Permissions().AccessModes(c.Request().Context(), c.User.ID, ownRepos)
		if err != nil {
			c.Error(err, "get access modes")
			return
		}
		basePerms, err := basePermissions(c, ownRepos...)
		if err != nil {
			c.Error(err, "get permissions of base repositories")
			return
		}
		repos := make([]*v1types.Repository, len(ownRepos))
		for i, repo := range ownRepos {
			accessMode := accessModes[repo.ID]
			// Site admins have full access to every repository through the API, the
			// same as when getting a single repository.
			if c.IsTokenAuth && c.User.IsAdmin {
				accessMode = database.AccessModeOwner
			}
			repos[i] = toRepository(repo, &v1types.RepositoryPermission{
				Admin: accessMode >= database.AccessModeAdmin,
				Push:  accessMode >= database.AccessModeWrite,
				Pull:  accessMode >= database.AccessModeRead,
			}, basePerms[repo.ForkID])
		}
		c.JSONSuccess(&repos)
		return
	}

	accessibleReposWithAccessMode, err := database.Handle.Repositories().GetByCollaboratorIDWithAccessMode(c.Request().Context(), user.ID)
	if err != nil {
		c.Error(err, "get repositories accesses by collaborator")
		return
	}

	accessibleRepos := make([]*database.Repository, 0, len(accessibleReposWithAccessMode))
	for repo := range accessibleReposWithAccessMode {
		accessibleRepos = append(accessibleRepos, repo)
	}
	if err = database.Handle.Repositories().LoadAttributes(c.Request().Context(), accessibleRepos...); err != nil {
		c.Error(err, "load attributes for accessible repositories")
		return
	}

	basePerms, err := basePermissions(c, append(ownRepos, accessibleRepos...)...)
	if err != nil {
		c.Error(err, "get permissions of base repositories")
		return
	}

	numOwnRepos := len(ownRepos)
	repos := make([]*v1types.Repository, 0, numOwnRepos+len(accessibleReposWithAccessMode))
	for _, r := range ownRepos {
		repos = append(repos, toRepository(r, &v1types.RepositoryPermission{Admin: true, Push: true, Pull: true}, basePerms[r.ForkID]))
	}

	for repo, access := range accessibleReposWithAccessMode {
		repos = append(repos,
			toRepository(repo, &v1types.RepositoryPermission{
				Admin: access >= database.AccessModeAdmin,
				Push:  access >= database.AccessModeWrite,
				Pull:  true,
			}, basePerms[repo.ForkID]),
		)
	}

	c.JSONSuccess(&repos)
}

func listMyRepos(c *apiContext) {
	listReposOfUser(c, c.User.Name)
}

func listUserRepositories(c *apiContext) {
	listReposOfUser(c, c.Param("username"))
}

func listOrgRepositories(c *apiContext) {
	listReposOfUser(c, c.Param("org"))
}

type createRepoRequest struct {
	Name         string `json:"name" binding:"Required;AlphaDashDot;MaxSize(100)"`
	Description  string `json:"description" binding:"MaxSize(255)"`
	Private      bool   `json:"private"`
	AutoInit     bool   `json:"auto_init"`
	Gitignores   string `json:"gitignores"`
	License      string `json:"license"`
	Readme       string `json:"readme"`
	ObjectFormat string `json:"object_format" binding:"OmitEmpty;In(sha1,sha256)"`
}

func createUserRepo(c *apiContext, owner *database.User, opt createRepoRequest) {
	// CreateRepository checks for an existing repository directory before it
	// checks the name, so a taken name would fail there with a 500 instead of
	// a 422.
	taken, err := database.Handle.Repositories().Exists(c.Request().Context(), owner, opt.Name)
	if err != nil {
		c.Error(err, "check repository existence")
		return
	} else if taken {
		c.ErrorStatus(http.StatusUnprocessableEntity, errors.Newf("repository %q already exists", opt.Name))
		return
	}

	repo, err := database.Handle.Repositories().Create(c.Request().Context(), c.Logger, c.User, owner, database.CreateRepoOptionsLegacy{
		Name:         opt.Name,
		Description:  opt.Description,
		Gitignores:   opt.Gitignores,
		License:      opt.License,
		Readme:       opt.Readme,
		IsPrivate:    opt.Private || conf.Repository.ForcePrivate,
		AutoInit:     opt.AutoInit,
		ObjectFormat: git.ObjectFormat(opt.ObjectFormat),
	})
	if err != nil {
		if database.IsErrRepoAlreadyExist(err) ||
			database.IsErrNameNotAllowed(err) {
			c.ErrorStatus(http.StatusUnprocessableEntity, err)
		} else {
			if repo != nil {
				if err = database.Handle.Repositories().Delete(c.Request().Context(), c.Logger, c.User.ID, repo.ID); err != nil {
					c.Logger.Error("Failed to delete repository", "error", err)
				}
			}
			c.Error(err, "create repository")
		}
		return
	}

	c.JSON(201, toRepository(repo, &v1types.RepositoryPermission{Admin: true, Push: true, Pull: true}, nil))
}

func createRepo(c *apiContext, opt createRepoRequest) {
	// Shouldn't reach this condition, but just in case.
	if c.User.IsOrganization() {
		c.ErrorStatus(http.StatusUnprocessableEntity, errors.New("Not allowed to create repository for organization."))
		return
	}
	createUserRepo(c, c.User, opt)
}

func createOrgRepo(c *apiContext, opt createRepoRequest) {
	org, err := database.Handle.Organizations().GetByName(c.Request().Context(), c.Param("org"))
	if err != nil {
		c.NotFoundOrError(err, "get organization by name")
		return
	}

	if !database.Handle.Organizations().IsOwner(c.Request().Context(), org.ID, c.User.ID) {
		c.ErrorStatus(http.StatusForbidden, errors.New("Given user is not owner of organization."))
		return
	}
	createUserRepo(c, org, opt)
}

type migrateRepoRequest struct {
	CloneAddr    string `json:"clone_addr" binding:"Required"`
	AuthUsername string `json:"auth_username"`
	AuthPassword string `json:"auth_password"`
	UID          int64  `json:"uid" binding:"Required"`
	RepoName     string `json:"repo_name" binding:"Required;AlphaDashDot;MaxSize(100)"`
	Mirror       bool   `json:"mirror"`
	Private      bool   `json:"private"`
	Unlisted     bool   `json:"unlisted"`
	Description  string `json:"description" binding:"MaxSize(512)"`
}

func migrate(c *apiContext, f migrateRepoRequest) {
	ctxUser := c.User
	// Not equal means context user is an organization,
	// or is another user/organization if current user is admin.
	if f.UID != ctxUser.ID {
		org, err := database.Handle.Users().GetByID(c.Request().Context(), f.UID)
		if err != nil {
			if database.IsErrUserNotExist(err) {
				c.ErrorStatus(http.StatusUnprocessableEntity, err)
			} else {
				c.Error(err, "get user by ID")
			}
			return
		} else if !org.IsOrganization() && !c.User.IsAdmin {
			c.ErrorStatus(http.StatusForbidden, errors.New("Given user is not an organization."))
			return
		}
		ctxUser = org
	}

	if ctxUser.IsOrganization() && !c.User.IsAdmin {
		// Check ownership of organization.
		if !database.Handle.Organizations().IsOwner(c.Request().Context(), ctxUser.ID, c.User.ID) {
			c.ErrorStatus(http.StatusForbidden, errors.New("Given user is not owner of organization."))
			return
		}
	}

	remoteAddr, err := form.ParseRemoteAddr(form.ParseRemoteAddrOptions{
		CloneAddr:    f.CloneAddr,
		User:         c.User,
		AuthUsername: f.AuthUsername,
		AuthPassword: f.AuthPassword,
	})
	if err != nil {
		if database.IsErrInvalidCloneAddr(err) {
			addrErr := err.(database.ErrInvalidCloneAddr)
			switch {
			case addrErr.IsURLError:
				c.ErrorStatus(http.StatusUnprocessableEntity, err)
			case addrErr.IsPermissionDenied:
				c.ErrorStatus(http.StatusUnprocessableEntity, errors.New("You are not allowed to import local repositories."))
			case addrErr.IsInvalidPath:
				c.ErrorStatus(http.StatusUnprocessableEntity, errors.New("Invalid local path, it does not exist or not a directory."))
			case addrErr.IsBlockedLocalAddress:
				c.ErrorStatus(http.StatusUnprocessableEntity, errors.New("Clone address resolved to a local network address that is implicitly blocked."))
			case addrErr.IsSSHAddress:
				c.ErrorStatus(http.StatusUnprocessableEntity, errors.New("SSH clone addresses are not supported, use the HTTP or HTTPS address with a username and password instead."))
			default:
				c.Error(err, "unexpected error")
			}
		} else {
			c.Error(err, "parse remote address")
		}
		return
	}

	repo, err := database.Handle.Repositories().Migrate(c.Request().Context(), c.Logger, c.User, ctxUser, database.MigrateRepoOptions{
		Name:        f.RepoName,
		Description: f.Description,
		IsPrivate:   f.Private || conf.Repository.ForcePrivate,
		IsMirror:    f.Mirror,
		RemoteAddr:  remoteAddr,
	})
	if err != nil {
		if repo != nil {
			if errDelete := database.Handle.Repositories().Delete(c.Request().Context(), c.Logger, ctxUser.ID, repo.ID); errDelete != nil {
				c.Logger.Error("Failed to delete repository", "error", errDelete)
			}
		}

		if database.IsErrReachLimitOfRepo(err) {
			c.ErrorStatus(http.StatusUnprocessableEntity, err)
		} else {
			c.Error(errors.New(database.HandleMirrorCredentials(err.Error(), true)), "migrate repository")
		}
		return
	}

	c.Logger.Debug("Repository migrated", "ownerName", ctxUser.Name, "repoName", f.RepoName)
	c.JSON(201, toRepository(repo, &v1types.RepositoryPermission{Admin: true, Push: true, Pull: true}, nil))
}

// FIXME: inject in the handler chain
func parseOwnerAndRepo(c *apiContext) (*database.User, *database.Repository) {
	owner, err := database.Handle.Users().GetByUsername(c.Request().Context(), c.Param("username"))
	if err != nil {
		if database.IsErrUserNotExist(err) {
			c.ErrorStatus(http.StatusUnprocessableEntity, err)
		} else {
			c.Error(err, "get user by name")
		}
		return nil, nil
	}

	repos := database.Handle.Repositories()
	repo, err := repos.GetByName(c.Request().Context(), owner.ID, c.Param("reponame"))
	if err != nil {
		c.NotFoundOrError(err, "get repository by name")
		return nil, nil
	}
	repo.Owner = owner
	if err = repos.LoadAttributes(c.Request().Context(), repo); err != nil {
		c.Error(err, "load repository attributes")
		return nil, nil
	}

	return owner, repo
}

func getRepo(c *apiContext) {
	_, repo := parseOwnerAndRepo(c)
	if c.Written() {
		return
	}

	basePerms, err := basePermissions(c, repo)
	if err != nil {
		c.Error(err, "get permissions of base repository")
		return
	}
	c.JSONSuccess(toRepository(repo, &v1types.RepositoryPermission{
		Admin: c.Repo.IsAdmin(),
		Push:  c.Repo.IsWriter(),
		Pull:  true,
	}, basePerms[repo.ForkID]))
}

func deleteRepo(c *apiContext) {
	owner, repo := parseOwnerAndRepo(c)
	if c.Written() {
		return
	}

	if owner.IsOrganization() && !database.Handle.Organizations().IsOwner(c.Request().Context(), owner.ID, c.User.ID) {
		c.ErrorStatus(http.StatusForbidden, errors.New("Given user is not owner of organization."))
		return
	}

	if err := database.Handle.Repositories().Delete(c.Request().Context(), c.Logger, owner.ID, repo.ID); err != nil {
		c.Error(err, "delete repository")
		return
	}

	c.Logger.Debug("Repository deleted", "ownerName", owner.Name, "repoName", repo.Name)
	c.NoContent()
}

func listForks(c *apiContext) {
	forks, err := database.Handle.Repositories().ListForks(c.Request().Context(), c.Repo.Repository)
	if err != nil {
		c.Error(err, "get forks")
		return
	}

	accessModes, err := database.Handle.Permissions().AccessModes(c.Request().Context(), c.UserID(), forks)
	if err != nil {
		c.Error(err, "get access modes")
		return
	}
	// Every fork has the repository of the request as its base.
	basePerm := accessPermission(c, c.Repo.AccessMode)

	apiForks := make([]*v1types.Repository, 0, len(forks))
	for _, fork := range forks {
		perm := accessPermission(c, accessModes[fork.ID])
		if !perm.Pull {
			continue
		}
		if err := fork.GetOwner(); err != nil {
			c.Error(err, "get owner")
			return
		}
		apiForks = append(apiForks, toRepository(fork, perm, basePerm))
	}

	c.JSONSuccess(&apiForks)
}

type editIssueTrackerRequest struct {
	EnableIssues          *bool   `json:"enable_issues"`
	EnableExternalTracker *bool   `json:"enable_external_tracker"`
	ExternalTrackerURL    *string `json:"external_tracker_url"`
	TrackerURLFormat      *string `json:"tracker_url_format"`
	TrackerIssueStyle     *string `json:"tracker_issue_style"`
}

func issueTracker(c *apiContext, form editIssueTrackerRequest) {
	_, repo := parseOwnerAndRepo(c)
	if c.Written() {
		return
	}

	if form.EnableIssues != nil {
		repo.EnableIssues = *form.EnableIssues
	}
	if form.EnableExternalTracker != nil {
		repo.EnableExternalTracker = *form.EnableExternalTracker
	}
	if form.ExternalTrackerURL != nil {
		repo.ExternalTrackerURL = *form.ExternalTrackerURL
	}
	if form.TrackerURLFormat != nil {
		repo.ExternalTrackerFormat = *form.TrackerURLFormat
	}
	if form.TrackerIssueStyle != nil {
		repo.ExternalTrackerStyle = *form.TrackerIssueStyle
	}
	// Browsers are sent to the URL, which must not run script, e.g., a
	// "javascript:" URL.
	if repo.EnableIssues && repo.EnableExternalTracker && !urlx.IsHTTP(repo.ExternalTrackerURL) {
		c.ErrorStatus(http.StatusUnprocessableEntity, errors.New("external_tracker_url must be an HTTP or HTTPS URL"))
		return
	}

	if err := database.Handle.Repositories().Update(c.Request().Context(), c.Logger, repo, false); err != nil {
		c.Error(err, "update repository")
		return
	}

	c.NoContent()
}

type editWikiRequest struct {
	EnableWiki         *bool   `json:"enable_wiki"`
	AllowPublicWiki    *bool   `json:"allow_public_wiki"`
	EnableExternalWiki *bool   `json:"enable_external_wiki"`
	ExternalWikiURL    *string `json:"external_wiki_url"`
}

func wiki(c *apiContext, form editWikiRequest) {
	_, repo := parseOwnerAndRepo(c)
	if c.Written() {
		return
	}

	if form.AllowPublicWiki != nil {
		repo.AllowPublicWiki = *form.AllowPublicWiki
	}
	if form.EnableExternalWiki != nil {
		repo.EnableExternalWiki = *form.EnableExternalWiki
	}
	if form.EnableWiki != nil {
		repo.EnableWiki = *form.EnableWiki
	}
	if form.ExternalWikiURL != nil {
		repo.ExternalWikiURL = *form.ExternalWikiURL
	}
	// Browsers are sent to the URL, which must not run script, e.g., a
	// "javascript:" URL.
	if repo.EnableWiki && repo.EnableExternalWiki && !urlx.IsHTTP(repo.ExternalWikiURL) {
		c.ErrorStatus(http.StatusUnprocessableEntity, errors.New("external_wiki_url must be an HTTP or HTTPS URL"))
		return
	}
	if err := database.Handle.Repositories().Update(c.Request().Context(), c.Logger, repo, false); err != nil {
		c.Error(err, "update repository")
		return
	}

	c.NoContent()
}

func mirrorSync(c *apiContext) {
	_, repo := parseOwnerAndRepo(c)
	if c.Written() {
		return
	} else if !repo.IsMirror {
		c.NotFound()
		return
	}

	go database.MirrorQueue.Add(repo.ID)
	c.Status(http.StatusAccepted)
}

func releases(c *apiContext) {
	_, repo := parseOwnerAndRepo(c)
	releases, err := database.Handle.Releases().ListByRepoID(c.Request().Context(), repo.ID)
	if err != nil {
		c.Error(err, "get releases by repository ID")
		return
	}
	apiReleases := make([]*v1types.RepositoryRelease, 0, len(releases))
	for _, r := range releases {
		publisher, err := database.Handle.Users().GetByID(c.Request().Context(), r.PublisherID)
		if err != nil {
			c.Error(err, "get release publisher")
			return
		}
		r.Publisher = publisher
	}
	for _, r := range releases {
		apiReleases = append(apiReleases, toRelease(r))
	}

	c.JSONSuccess(&apiReleases)
}

// accessPermission returns the permission of the access mode, with site admins
// given full access through the API, the same as elsewhere in the API.
func accessPermission(c *apiContext, mode database.AccessMode) *v1types.RepositoryPermission {
	if c.IsTokenAuth && c.User.IsAdmin {
		mode = database.AccessModeOwner
	}
	return &v1types.RepositoryPermission{
		Admin: mode >= database.AccessModeAdmin,
		Push:  mode >= database.AccessModeWrite,
		Pull:  mode >= database.AccessModeRead,
	}
}

// basePermissions returns the permission the viewer has to the base repository
// of each of the forks, keyed by the ID of the base repository. A base
// repository the viewer cannot read is left out, e.g., after the owner of the
// fork is removed as a collaborator of it, so that it is not shown as a parent.
func basePermissions(c *apiContext, repos ...*database.Repository) (map[int64]*v1types.RepositoryPermission, error) {
	var bases []*database.Repository
	for _, repo := range repos {
		if repo.IsFork && repo.BaseRepo != nil {
			bases = append(bases, repo.BaseRepo)
		}
	}
	if len(bases) == 0 {
		return nil, nil
	}

	modes, err := database.Handle.Permissions().AccessModes(c.Request().Context(), c.UserID(), bases)
	if err != nil {
		return nil, errors.Wrap(err, "get access modes")
	}
	perms := make(map[int64]*v1types.RepositoryPermission, len(bases))
	for _, base := range bases {
		if perm := accessPermission(c, modes[base.ID]); perm.Pull {
			perms[base.ID] = perm
		}
	}
	return perms, nil
}
