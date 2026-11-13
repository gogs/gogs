package web

import (
	"strings"

	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/errx"
	"gogs.io/gogs/internal/git"
)

// pageRepo is the repository of a page, and the access the viewer has to it.
type pageRepo struct {
	Owner      *database.User
	Repository *database.Repository
	AccessMode database.AccessMode
}

// IsWriter returns true if the viewer has write or higher access.
func (r *pageRepo) IsWriter() bool {
	return r.AccessMode >= database.AccessModeWrite
}

// IsAdmin returns true if the viewer has admin or higher access.
func (r *pageRepo) IsAdmin() bool {
	return r.AccessMode >= database.AccessModeAdmin
}

// withPageRepo sets the repository named by the "owner" and "repo" path
// parameters as pageContext.Repo, if the viewer may see the page. Visitors
// without access to a repository whose issues or wiki are public are sent to
// those, unless issuesPage or wikiPage say the page is already one of them.
func withPageRepo(issuesPage, wikiPage bool) func(c *pageContext) {
	return func(c *pageContext) {
		ctx := c.Request().Context()
		ownerName := c.Param("owner")
		repoName := strings.TrimSuffix(c.Param("repo"), ".git")

		owner := c.User
		if !c.IsLogged() || c.User.LowerName != strings.ToLower(ownerName) {
			var err error
			owner, err = database.Handle.Users().GetByUsername(ctx, ownerName)
			if err != nil {
				if errx.IsNotFound(err) {
					c.NotFound()
					return
				}
				log.Error("withPageRepo: get user by name %q: %v", ownerName, err)
				c.ServerError(err)
				return
			}
		}

		repo, err := database.GetRepositoryByName(owner.ID, repoName)
		if err != nil {
			if errx.IsNotFound(err) {
				c.NotFound()
				return
			}
			log.Error("withPageRepo: get repository %q of %q: %v", repoName, owner.Name, err)
			c.ServerError(err)
			return
		}

		var mode database.AccessMode
		if c.IsLogged() && c.User.IsAdmin {
			mode = database.AccessModeOwner
		} else {
			mode = database.Handle.Permissions().AccessMode(ctx, c.UserID(), repo.ID,
				database.AccessModeOptions{
					OwnerID: repo.OwnerID,
					Private: repo.IsPrivate,
				},
			)
		}

		// Viewers without direct access to a fork have the access they have to its
		// base repository, up to write.
		if mode == database.AccessModeNone && repo.BaseRepo != nil {
			mode = min(
				database.Handle.Permissions().AccessMode(ctx, c.UserID(), repo.BaseRepo.ID,
					database.AccessModeOptions{
						OwnerID: repo.BaseRepo.OwnerID,
						Private: repo.BaseRepo.IsPrivate,
					},
				),
				database.AccessModeWrite,
			)
		}

		if mode == database.AccessModeNone {
			// Send the viewer to the part of the repository that is public, if not yet
			// on it.
			if repo.IsPartialPublic() &&
				(!(issuesPage || wikiPage) ||
					(issuesPage && !repo.CanGuestViewIssues()) ||
					(wikiPage && !repo.CanGuestViewWiki())) {
				switch {
				case repo.CanGuestViewIssues():
					c.Redirect(repo.Link() + "/issues")
				case repo.CanGuestViewWiki():
					c.Redirect(repo.Link() + "/wiki")
				default:
					c.NotFound()
				}
				return
			}

			if !repo.IsPartialPublic() ||
				(issuesPage && !repo.CanGuestViewIssues()) ||
				(wikiPage && !repo.CanGuestViewWiki()) {
				c.NotFound()
				return
			}
			repo.EnableIssues = repo.CanGuestViewIssues()
			repo.EnableWiki = repo.CanGuestViewWiki()
		}

		c.Repo = &pageRepo{
			Owner:      owner,
			Repository: repo,
			AccessMode: mode,
		}
	}
}

// requirePageRepoWriter turns away viewers without write access. Mount it after
// withPageRepo.
func requirePageRepoWriter(c *pageContext) {
	if !c.IsLogged() || (!c.Repo.IsWriter() && !c.User.IsAdmin) {
		c.NotFound()
	}
}

// requirePageRepoAdmin turns away viewers without admin access. Mount it after
// withPageRepo.
func requirePageRepoAdmin(c *pageContext) {
	if !c.IsLogged() || (!c.Repo.IsAdmin() && !c.User.IsAdmin) {
		c.NotFound()
	}
}

// requireNonEmptyRepo turns away requests for an empty repository. Mount it
// after withPageRepo.
func requireNonEmptyRepo(c *pageContext) {
	if c.Repo.Repository.IsEmpty {
		c.NotFound()
	}
}

// requireIssues turns away requests when the repository has no issues, and
// sends them to the external issue tracker when it has one. Mount it after
// withPageRepo.
func requireIssues(c *pageContext) {
	repo := c.Repo.Repository
	if !repo.EnableIssues {
		c.NotFound()
		return
	}
	if repo.EnableExternalTracker {
		c.Redirect(repo.ExternalTrackerURL)
	}
}

// requirePulls turns away requests when the repository does not take pull
// requests. Mount it after withPageRepo.
func requirePulls(c *pageContext) {
	if !c.Repo.Repository.AllowsPulls() {
		c.NotFound()
	}
}

// requireWiki turns away requests when the repository has no wiki, and sends
// them to the external wiki when it has one. Mount it after withPageRepo.
func requireWiki(c *pageContext) {
	repo := c.Repo.Repository
	if !repo.EnableWiki {
		c.NotFound()
		return
	}
	if repo.EnableExternalWiki {
		c.Redirect(repo.ExternalWikiURL)
	}
}

// requirePageEditor turns away requests to edit files unless the "**" path
// parameter starts with a branch the viewer may commit to without a pull
// request. Mount it after withPageRepo and requireNonEmptyRepo.
func requirePageEditor(c *pageContext) {
	repo := c.Repo.Repository
	if !repo.CanEnableEditor() || !c.Repo.IsWriter() {
		c.NotFound()
		return
	}

	ctx := c.Request().Context()
	gitRepo, err := git.Open(repo.RepoPath())
	if err != nil {
		log.Error("requirePageEditor: open repository %d: %v", repo.ID, err)
		c.ServerError(err)
		return
	}

	// Like the server-rendered pages, the shortest prefix of the path that names
	// a branch or a tag is the reference. Only branches can be edited.
	var branch string
	if path := c.Param("**"); path == "" {
		branch = repo.DefaultBranch
		if !gitRepo.HasBranch(ctx, branch) {
			branches, err := gitRepo.Branches(ctx)
			if err != nil {
				log.Error("requirePageEditor: list branches of repository %d: %v", repo.ID, err)
				c.ServerError(err)
				return
			}
			branch = branches[0]
		}
	} else {
		var refName string
		for part := range strings.SplitSeq(path, "/") {
			refName = strings.TrimPrefix(refName+"/"+part, "/")
			if gitRepo.HasBranch(ctx, refName) {
				branch = refName
				break
			}
			if gitRepo.HasTag(ctx, refName) {
				break
			}
		}
	}

	if branch == "" || repo.IsBranchRequirePullRequest(branch) {
		c.NotFound()
	}
}

// pageOrg is the organization of a page, and the membership of the viewer.
type pageOrg struct {
	Organization *database.User
	// IsOwner is true for owners and site admins.
	IsOwner bool
	// IsMember is true for members, owners and site admins.
	IsMember bool
}

// withPageOrg sets the organization named by the "org" path parameter as
// pageContext.Org, and turns away viewers who are not a member or, when
// requireOwner is true, an owner. When the page is about the team named by the
// "team" path parameter, the team must be one the viewer sees, which are the
// teams they are in, or every team for owners.
func withPageOrg(requireOwner bool) func(c *pageContext) {
	return func(c *pageContext) {
		name := c.Param("org")
		org, err := database.Handle.Users().GetByUsername(c.Request().Context(), name)
		if err != nil {
			if errx.IsNotFound(err) {
				c.NotFound()
				return
			}
			log.Error("withPageOrg: get organization by name %q: %v", name, err)
			c.ServerError(err)
			return
		}

		// Like the server-rendered pages, a user is sent to their profile.
		if !org.IsOrganization() {
			c.Redirect("/" + org.Name)
			return
		}

		o := &pageOrg{Organization: org}
		if c.User.IsAdmin || org.IsOwnedBy(c.User.ID) {
			o.IsOwner = true
			o.IsMember = true
		} else {
			o.IsMember = org.IsOrgMember(c.User.ID)
		}
		if !o.IsMember || (requireOwner && !o.IsOwner) {
			c.NotFound()
			return
		}
		c.Org = o

		teamName := c.Param("team")
		if teamName == "" {
			return
		}
		var teams []*database.Team
		if o.IsOwner {
			err = org.GetTeams()
			teams = org.Teams
		} else {
			teams, err = org.GetUserTeams(c.User.ID)
		}
		if err != nil {
			log.Error("withPageOrg: get teams of organization %d: %v", org.ID, err)
			c.ServerError(err)
			return
		}
		for _, team := range teams {
			if team.LowerName == strings.ToLower(teamName) {
				return
			}
		}
		c.NotFound()
	}
}

// requireGitHooks turns away users who may not edit Git hooks.
func requireGitHooks(c *pageContext) {
	if !c.User.CanEditGitHook() {
		c.NotFound()
	}
}

// requireUploads turns away requests when uploading files is disabled.
func requireUploads(c *pageContext) {
	if !conf.Repository.Upload.Enabled {
		c.NotFound()
	}
}
