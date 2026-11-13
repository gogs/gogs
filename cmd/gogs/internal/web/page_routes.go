package web

import (
	"bytes"
	stdctx "context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"slices"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/flamego/flamego"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/unknwon/com"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/app"
	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/errx"
	"gogs.io/gogs/internal/git"
	"gogs.io/gogs/internal/osx"
	"gogs.io/gogs/internal/repox"
	routerepo "gogs.io/gogs/internal/route/repo"
	"gogs.io/gogs/internal/userx"
)

// mountPageRoutes mounts the routes of the pages, which the web app renders,
// and of the files and redirects served next to them.
func mountPageRoutes(f *flamego.Flame, webApp http.Handler) {
	page := newPageContexter(webApp)
	serveWeb := func(c *pageContext) { c.ServeWeb() }
	reqSignIn := pageAccess(true)
	ignSignIn := pageAccess(conf.Auth.RequireSigninView)

	f.Get("/", page, ignSignIn, serveWeb)
	f.Group("/explore", func() {
		f.Get("", func(c flamego.Context) {
			c.Redirect(conf.Server.Subpath + "/explore/repos")
		})
		f.Get("/{type: /(repos|users|organizations)/}", serveWeb)
	}, page, ignSignIn)
	f.Get("/{type: /(issues|pulls)/}", page, reqSignIn, serveWeb)

	f.Group("/user", func() {
		f.Group("/settings", func() {
			f.Get("", serveWeb)
			f.Get("/{page: /(avatar|password|email|ssh|security|repositories|organizations|applications|delete)/}", serveWeb)
			// Like the server-rendered pages, these pages are only found in the
			// two-factor authentication state they apply to.
			f.Get("/security/two_factor_enable", func(c *pageContext) {
				if database.Handle.TwoFactors().IsEnabled(c.Request().Context(), c.User.ID) {
					c.NotFound()
					return
				}
				c.ServeWeb()
			})
			f.Get("/security/two_factor_recovery_codes", func(c *pageContext) {
				if !database.Handle.TwoFactors().IsEnabled(c.Request().Context(), c.User.ID) {
					c.NotFound()
					return
				}
				c.ServeWeb()
			})
		}, reqSignIn)

		f.Any("/activate_email", activateEmail)
		f.Get("/email2user", email2User)
		// The pages for signing in stay reachable when the site requires signing in to
		// view content.
		f.Get("/{page: /(sign-in|sign-up|mfa|reset-password|activate)/}", serveWeb)
	}, page)

	f.Group("/admin", func() {
		f.Get("", serveWeb)
		f.Get("/{page: /(operations|users|orgs|repos|auths|notices|config|monitor)/}", serveWeb)
		f.Get("/users/new", serveWeb)
		f.Get("/users/{id}", func(c *pageContext) {
			user, err := database.Handle.Users().GetByID(c.Request().Context(), c.ParamInt64("id"))
			if err != nil {
				if errx.IsNotFound(err) {
					c.NotFound()
					return
				}
				log.Error("admin user page: get user by ID: %v", err)
				c.ServerError(err)
				return
			}
			// Organizations have their own settings.
			if user.IsOrganization() {
				c.NotFound()
				return
			}
			c.ServeWeb()
		})
		f.Get("/auths/new", serveWeb)
		f.Get("/auths/{id}", func(c *pageContext) {
			if _, err := database.Handle.LoginSources().GetByID(c.Request().Context(), c.ParamInt64("id")); err != nil {
				if errx.IsNotFound(err) {
					c.NotFound()
					return
				}
				log.Error("admin authentication source page: get login source by ID: %v", err)
				c.ServerError(err)
				return
			}
			c.ServeWeb()
		})
	}, page, reqSignIn, requirePageAdmin)

	f.Group("/{owner}", func() {
		f.Get("", serveUserProfile)
		f.Get("/{type: /(followers|following)/}", withPageProfileUser, serveWeb)
	}, page, ignSignIn)
	f.Get("/attachments/{uuid}", page, ignSignIn, getAttachment)

	// webhookPages mounts the webhook settings pages of a repository or an
	// organization. Only the kinds of webhooks the site allows can be added, and
	// webhooks of others are not found.
	webhookPages := func(lookup func(c *pageContext, id int64) (*database.Webhook, error)) {
		f.Get("", serveWeb)
		f.Get("/{type}/new", func(c *pageContext) {
			if !slices.Contains(conf.Webhook.Types, c.Param("type")) {
				c.NotFound()
				return
			}
			c.ServeWeb()
		})
		f.Get("/{id}", func(c *pageContext) {
			id := c.ParamInt64("id")
			if id <= 0 {
				c.NotFound()
				return
			}
			if _, err := lookup(c, id); err != nil {
				if errx.IsNotFound(err) {
					c.NotFound()
					return
				}
				log.Error("webhook page: get webhook %d: %v", id, err)
				c.ServerError(err)
				return
			}
			c.ServeWeb()
		})
	}

	f.Group("/org", func() {
		f.Get("/create", func(c *pageContext) {
			if !c.User.CanCreateOrganization() {
				c.NotFound()
				return
			}
			c.ServeWeb()
		})

		f.Group("/{org}", func() {
			f.Get("/dashboard", serveWeb)
			f.Get("/{type: /(issues|pulls)/}", serveWeb)
			f.Get("/members", serveWeb)
			f.Get("/teams", serveWeb)
			f.Get("/teams/{team}", serveWeb)
			f.Get("/teams/{team}/repositories", serveWeb)
		}, withPageOrg(false))

		f.Group("/{org}", func() {
			f.Get("/teams/new", serveWeb)
			f.Get("/teams/{team}/edit", serveWeb)
			f.Group("/settings", func() {
				f.Get("", serveWeb)
				f.Group("/hooks", func() {
					webhookPages(func(c *pageContext, id int64) (*database.Webhook, error) {
						return database.GetWebhookByOrgID(c.Org.Organization.ID, id)
					})
				})
				f.Get("/delete", serveWeb)
			})
			f.Get("/invitations/new", serveWeb)
		}, withPageOrg(true))
	}, page, reqSignIn)

	f.Group("/repo", func() {
		f.Get("/{page: /(create|migrate)/}", func(c *pageContext) {
			_, err := repoOwner(c.Request().Context(), c.User, c.QueryInt64("org"))
			if errors.Is(err, errRepoOwnerNotAllowed) {
				c.NotFound()
				return
			} else if err != nil {
				log.Error("new repository page: get repository owner: %v", err)
				c.ServerError(err)
				return
			}
			c.ServeWeb()
		})
		f.Get("/fork/{id}", func(c *pageContext) {
			_, err := forkBaseRepo(c.User, c.ParamInt64("id"))
			if errors.Is(err, errForkBaseNotFound) {
				c.NotFound()
				return
			} else if err != nil {
				log.Error("fork page: get repository to fork: %v", err)
				c.ServerError(err)
				return
			}
			c.ServeWeb()
		})
	}, page, reqSignIn)

	f.Group("/{owner}/{repo}", func() {
		f.Group("/settings", func() {
			f.Get("", serveWeb)
			f.Get("/collaboration", serveWeb)
			f.Group("/branches", func() {
				f.Get("", serveWeb)
				f.Get("/{**}", serveWeb)
			}, func(c *pageContext) {
				if c.Repo.Repository.IsMirror {
					c.NotFound()
				}
			})
			f.Group("/hooks", func() {
				webhookPages(func(c *pageContext, id int64) (*database.Webhook, error) {
					return database.GetWebhookOfRepoByID(c.Repo.Repository.ID, id)
				})
				f.Group("/git", func() {
					f.Get("", serveWeb)
					f.Get("/{name}", serveWeb)
				}, requireGitHooks)
			})
			f.Get("/keys", serveWeb)
		})
	}, page, reqSignIn, withPageRepo(false, false), requirePageRepoAdmin)

	f.Group("/{owner}/{repo}", func() {
		f.Get("/issues", requireIssues, serveIssueList)
		f.Get("/issues/{index}", serveIssue)
		f.Get("/{page: /(labels|milestones)/}", serveWeb)
	}, page, ignSignIn, withPageRepo(true, false))
	f.Get("/{owner}/{repo}/issues/new", page, reqSignIn, withPageRepo(true, false), requireIssues, serveWeb)
	f.Group("/{owner}/{repo}/wiki", func() {
		f.Get("/?{page}", serveWeb)
		f.Get("/_pages", serveWeb)
	}, page, ignSignIn, withPageRepo(false, true), requireWiki)

	f.Group("/{owner}/{repo}", func() {
		f.Group("/milestones", func() {
			f.Get("/new", serveWeb)
			f.Get("/{id}/edit", func(c *pageContext) {
				// An ID of 0 would match any milestone of the repository.
				id := c.ParamInt64("id")
				if id <= 0 {
					c.NotFound()
					return
				}
				if _, err := database.GetMilestoneByRepoID(c.Repo.Repository.ID, id); err != nil {
					if errx.IsNotFound(err) {
						c.NotFound()
						return
					}
					log.Error("milestone page: get milestone %d: %v", id, err)
					c.ServerError(err)
					return
				}
				c.ServeWeb()
			})
		}, requirePageRepoWriter)

		f.Group("/releases", func() {
			f.Get("/new", serveWeb)
			f.Get("/edit/{**}", serveWeb)
		}, requireNonEmptyRepo, requirePageRepoWriter)

		f.Get("/compare/{**}", requirePulls, serveWeb)

		f.Group("", func() {
			f.Get("/_edit/{**}", serveWeb)
			f.Get("/_new/{**}", serveWeb)
			f.Get("/_delete/{**}", serveWeb)
			f.Get("/_upload/{**}", requireUploads, serveWeb)
		}, requireNonEmptyRepo, requirePageRepoWriter, requirePageEditor)

		f.Group("/wiki", func() {
			f.Get("/_new", serveWeb)
			f.Get("/{page}/_edit", serveWeb)
		}, requirePageRepoWriter, requireWiki)
	}, page, reqSignIn, withPageRepo(false, false))

	f.Group("/{owner}/{repo}", func() {
		f.Get("/releases", requireNonEmptyRepo, serveWeb)
		f.Get("/pulls", requirePulls, serveIssueList)
		f.Get("/pulls/{index}", servePull)
		f.Get("/pulls/{index}/{tab: /(commits|files)/}", servePull)
		f.Group("/branches", func() {
			f.Get("", serveWeb)
			f.Get("/all", serveWeb)
		}, requireNonEmptyRepo)
		f.Get("/archive/{**}", requireNonEmptyRepo, getArchive)
		f.Get("/forks", requireNonEmptyRepo, serveWeb)
		f.Get("/src/{**}", requireNonEmptyRepo, serveWeb)
		f.Get("/commits/{**}", requireNonEmptyRepo, serveWeb)
		f.Get("/search", requireNonEmptyRepo, serveWeb)
		f.Get("/search/{**}", requireNonEmptyRepo, serveWeb)
		f.Get("/commit/{sha: /[a-f0-9]{7,64}/}", requireNonEmptyRepo, serveWeb)
		f.Get("/compare/"+compareRangeRoute, requireNonEmptyRepo, serveWeb)
	}, page, ignSignIn, withPageRepo(false, false))
	f.Group("/{owner}/{repo}", func() {
		f.Get("", serveWeb)
		f.Get("/{type: /(stars|watchers)/}", serveWeb)
	}, serveGoGet, page, ignSignIn, withPageRepo(false, false))

	f.Get("/-/metrics", app.MetricsFilter, promhttp.Handler().ServeHTTP)
}

// serveUserProfile serves the profile page of a user or an organization, and the
// public SSH keys of a user at "/{username}.keys".
func serveUserProfile(c *pageContext) {
	// Usernames cannot end with ".keys", so "/alice.keys" is always the public
	// SSH keys of "alice".
	name, isKeys := strings.CutSuffix(c.Param("owner"), ".keys")
	user, err := database.Handle.Users().GetByUsername(c.Request().Context(), name)
	if err != nil {
		if errx.IsNotFound(err) {
			c.NotFound()
			return
		}
		log.Error("serveUserProfile: get user by name %q: %v", name, err)
		c.ServerError(err)
		return
	}

	if !isKeys {
		c.ServeWeb()
		return
	}

	keys, err := database.ListPublicKeys(user.ID)
	if err != nil {
		log.Error("serveUserProfile: list public keys of user %d: %v", user.ID, err)
		c.ServerError(err)
		return
	}
	var buf bytes.Buffer
	for _, key := range keys {
		buf.WriteString(key.OmitEmail())
		buf.WriteString("\n")
	}
	w := c.ResponseWriter()
	w.Header().Set("Content-Type", "text/plain; charset=UTF-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(buf.Bytes())
}

// withPageProfileUser turns away requests for users that do not exist.
func withPageProfileUser(c *pageContext) {
	name := c.Param("owner")
	if _, err := database.Handle.Users().GetByUsername(c.Request().Context(), name); err != nil {
		if errx.IsNotFound(err) {
			c.NotFound()
			return
		}
		log.Error("withPageProfileUser: get user by name %q: %v", name, err)
		c.ServerError(err)
	}
}

// getAttachment serves an attachment of an issue, a comment or a release to
// viewers who can read the repository.
func getAttachment(c *pageContext) {
	attach, err := database.GetAttachmentByUUID(c.Param("uuid"))
	if err != nil {
		if errx.IsNotFound(err) {
			c.NotFound()
			return
		}
		log.Error("getAttachment: get attachment by UUID: %v", err)
		c.ServerError(err)
		return
	}

	// Resolve the repository that owns this attachment so we can enforce
	// repository-level read permission. Without this check, anyone with the UUID
	// could download files belonging to private repositories.
	var repo *database.Repository
	switch {
	case attach.IssueID > 0:
		issue, err := database.GetIssueByID(attach.IssueID)
		if err != nil {
			if errx.IsNotFound(err) {
				c.NotFound()
				return
			}
			log.Error("getAttachment: get issue %d: %v", attach.IssueID, err)
			c.ServerError(err)
			return
		}
		repo = issue.Repo
	case attach.ReleaseID > 0:
		release, err := database.GetReleaseByID(attach.ReleaseID)
		if err != nil {
			if errx.IsNotFound(err) {
				c.NotFound()
				return
			}
			log.Error("getAttachment: get release %d: %v", attach.ReleaseID, err)
			c.ServerError(err)
			return
		}
		repo = release.Repo
	}
	if repo == nil || !repo.HasAccess(c.UserID()) || !osx.IsFile(attach.LocalPath()) {
		c.NotFound()
		return
	}

	fr, err := os.Open(attach.LocalPath())
	if err != nil {
		log.Error("getAttachment: open attachment %d: %v", attach.ID, err)
		c.ServerError(err)
		return
	}
	defer func() { _ = fr.Close() }()

	w := c.ResponseWriter()
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; sandbox")
	w.Header().Set("Cache-Control", "private,max-age=86400")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename="%s"`, attach.Name))
	if _, err = io.Copy(w, fr); err != nil {
		log.Error("getAttachment: copy attachment %d to response: %v", attach.ID, err)
	}
}

// getArchive serves the archive of a branch, a tag or a commit, e.g.,
// "master.zip".
func getArchive(c *pageContext) {
	repo := c.Repo.Repository
	gitRepo, err := git.Open(repo.RepoPath())
	if err != nil {
		log.Error("getArchive: open repository %d: %v", repo.ID, err)
		c.ServerError(err)
		return
	}

	archivePath, name, err := routerepo.PrepareArchive(c.Request().Context(), gitRepo, repo.Name, c.Param("**"))
	if err != nil {
		if errx.IsNotFound(err) {
			c.NotFound()
			return
		}
		log.Error("getArchive: prepare archive of repository %d: %v", repo.ID, err)
		c.ServerError(err)
		return
	}
	routerepo.ServeArchive(c.ResponseWriter(), c.Request().Request, archivePath, name)
}

// serveIssueList serves the issue or pull request list of a repository. Like
// the server-rendered page did, it sends signed-out visitors to sign in for the
// lists about the viewer.
func serveIssueList(c *pageContext) {
	if !c.IsLogged() && slices.Contains([]string{"assigned", "created_by", "mentioned"}, c.Query("type")) {
		c.redirectToSignIn()
		return
	}
	c.ServeWeb()
}

// pageIssue returns the issue or pull request of the "index" path parameter,
// or false after responding when there is none.
func pageIssue(c *pageContext) (*database.Issue, bool) {
	index := c.ParamInt64("index")
	if index <= 0 {
		c.NotFound()
		return nil, false
	}
	issue, err := database.GetIssueByIndex(c.Repo.Repository.ID, index)
	if err != nil {
		if errx.IsNotFound(err) {
			c.NotFound()
			return nil, false
		}
		log.Error("pageIssue: get issue %d of repository %d: %v", index, c.Repo.Repository.ID, err)
		c.ServerError(err)
		return nil, false
	}
	return issue, true
}

// serveIssue serves the page of an issue, and sends pull requests to their own
// page.
func serveIssue(c *pageContext) {
	issue, ok := pageIssue(c)
	if !ok {
		return
	}
	if issue.IsPull {
		c.Context.Redirect(fmt.Sprintf("%s/pulls/%d", c.Repo.Repository.Link(), issue.Index))
		return
	}
	requireIssues(c)
	if c.ResponseWriter().Written() {
		return
	}
	c.ServeWeb()
}

// servePull serves the page of a pull request, and sends issues to their own
// page.
func servePull(c *pageContext) {
	issue, ok := pageIssue(c)
	if !ok {
		return
	}
	if !issue.IsPull {
		c.Context.Redirect(fmt.Sprintf("%s/issues/%d", c.Repo.Repository.Link(), issue.Index))
		return
	}
	requirePulls(c)
	if c.ResponseWriter().Written() {
		return
	}
	c.ServeWeb()
}

// activateEmail activates the email address of an activation link, and sends
// the user to their email settings, which confirm it.
func activateEmail(c *pageContext) {
	ctx := c.Request().Context()
	code := c.Query("code")
	emailAddr := c.Query("email")
	if email := verifyActivateEmailCode(ctx, code, emailAddr); email != nil {
		if err := database.Handle.Users().MarkEmailActivated(ctx, email.UserID, email.Email); err != nil {
			log.Error("activateEmail: mark email %q of user %d activated: %v", email.Email, email.UserID, err)
			c.ServerError(err)
			return
		}
		log.Trace("Email activated: %s", email.Email)
		c.Redirect(conf.Server.Subpath + "/user/settings/email?activated=1")
		return
	}
	c.Redirect(conf.Server.Subpath + "/user/settings/email")
}

// verifyActivateEmailCode returns the email address the code activates, or nil
// when the code is not valid for it.
func verifyActivateEmailCode(ctx stdctx.Context, code, email string) *database.EmailAddress {
	user := parseUserFromCode(ctx, code)
	if user == nil || !userx.VerifyActivateCode(code, user.ID, email, user.LowerName, user.Password, user.Rands) {
		return nil
	}
	emailAddress, err := database.Handle.Users().GetEmail(ctx, user.ID, email, false)
	if err != nil {
		return nil
	}
	return emailAddress
}

// email2User sends the viewer to the profile of the user with the email
// address in the query.
func email2User(c *pageContext) {
	u, err := database.Handle.Users().GetByEmail(c.Request().Context(), c.Query("email"))
	if err != nil {
		if errx.IsNotFound(err) {
			c.NotFound()
			return
		}
		log.Error("email2User: get user by email: %v", err)
		c.ServerError(err)
		return
	}
	c.Redirect(conf.Server.Subpath + "/user/" + u.Name)
}

// serveGoGet answers "go get" requests for a repository with the go-import
// meta, whether or not the viewer may see the repository, or the repository
// exists at all, because "go get" does not send credentials from ".netrc".
func serveGoGet(c flamego.Context) {
	if c.Query("go-get") != "1" {
		return
	}

	ownerName := c.Param("owner")
	repoName := c.Param("repo")
	branchName := "master"
	owner, err := database.Handle.Users().GetByUsername(c.Request().Context(), ownerName)
	if err == nil {
		repo, err := database.Handle.Repositories().GetByName(c.Request().Context(), owner.ID, repoName)
		if err == nil && repo.DefaultBranch != "" {
			branchName = repo.DefaultBranch
		}
	}

	prefix := conf.Server.ExternalURL + path.Join(ownerName, repoName, "src", branchName)
	insecureFlag := ""
	if !strings.HasPrefix(conf.Server.ExternalURL, "https://") {
		insecureFlag = "--insecure "
	}
	w := c.ResponseWriter()
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(com.Expand(`<!doctype html>
<html>
	<head>
		<meta name="go-import" content="{GoGetImport} git {CloneLink}">
		<meta name="go-source" content="{GoGetImport} _ {GoDocDirectory} {GoDocFile}">
	</head>
	<body>
		go get {InsecureFlag}{GoGetImport}
	</body>
</html>
`,
		map[string]string{
			"GoGetImport":    path.Join(conf.Server.URL.Host, conf.Server.Subpath, ownerName, repoName),
			"CloneLink":      repox.HTTPSCloneURL(ownerName, repoName),
			"GoDocDirectory": prefix + "{/dir}",
			"GoDocFile":      prefix + "{/dir}/{file}#L{line}",
			"InsecureFlag":   insecureFlag,
		},
	)))
}
