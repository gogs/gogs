package web

import (
	stdctx "context"
	"encoding/json"
	"net/http"
	"net/url"
	"reflect"
	"regexp"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/flamego/binding"
	"github.com/flamego/flamego"
	"github.com/flamego/session"
	"github.com/flamego/validator"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/i18n"
	apiv1 "gogs.io/gogs/internal/route/api/v1"
)

// authMethod is how the signed-in user of a request authenticated.
type authMethod struct {
	basicAuth bool
	tokenAuth bool
}

// authenticator returns the middleware that maps the signed-in user
// (*database.User, nil for anonymous visitors) and the authMethod of each
// request, from the session, the access token, the reverse proxy
// authentication header, or HTTP Basic Authentication.
func authenticator(store context.Store) flamego.Handler {
	return func(c flamego.Context, sess session.Session) {
		user, isBasicAuth, isTokenAuth := context.AuthenticatedUser(store, c.Request().Request, sess)
		c.Map(user, authMethod{basicAuth: isBasicAuth, tokenAuth: isTokenAuth})
	}
}

func corsHeaders(w http.ResponseWriter) {
	if conf.HTTP.AccessControlAllowOrigin == "" {
		return
	}
	w.Header().Set("Access-Control-Allow-Origin", conf.HTTP.AccessControlAllowOrigin)
	w.Header().Set("Access-Control-Allow-Credentials", "true")
	w.Header().Set("Access-Control-Max-Age", "3600")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Access-Control-Allow-Headers, Authorization, X-Requested-With")
}

// apiV1Access turns away the API v1 requests of users prohibited from signing
// in, users who have not confirmed their email when that is required, and,
// when the site requires signing in to view content, anonymous visitors.
func apiV1Access(c *pageContext) {
	writeJSON := func(message string) {
		w := c.ResponseWriter()
		w.Header().Set("Content-Type", "application/json; charset=UTF-8")
		w.WriteHeader(http.StatusForbidden)
		body, _ := json.Marshal(map[string]string{"message": message})
		_, _ = w.Write(body)
	}
	switch {
	case c.IsLogged() && c.User.ProhibitLogin:
		c.serveWeb(context.WebContext{Page: context.WebPageProhibitLogin})
	case c.IsLogged() && !c.User.IsActive && conf.Auth.RequireEmailConfirmation:
		writeJSON("User account is not activated.")
	case !c.IsLogged() && conf.Auth.RequireSigninView:
		writeJSON("Only authenticated user is allowed to call APIs.")
	}
}

type apiV1RequestKey struct{}

// apiV1Request is what apiV1Bridger passes to the API v1 handler.
type apiV1Request struct {
	user   *database.User
	info   apiv1.RequestInfo
	locale i18n.Locale
}

// apiV1Bridger hands API v1 requests to its handler. Like Macaron did, the
// trailing slash is trimmed and each segment of the escaped path is unescaped
// before matching, so an encoded slash (%2F) stays within its segment.
func apiV1Bridger(apiHandler http.Handler) flamego.Handler {
	segmentEscaper := strings.NewReplacer("%", "%25", "/", "%2F")
	return func(c flamego.Context, user *database.User, method authMethod, l i18n.Locale) {
		r := c.Request().Request
		u := *r.URL
		segments := strings.Split(strings.TrimSuffix(u.EscapedPath(), "/"), "/")
		for i, segment := range segments {
			unescaped, err := url.PathUnescape(segment)
			if err == nil {
				segments[i] = segmentEscaper.Replace(unescaped)
			}
		}
		u.Path = strings.Join(segments, "/")
		u.RawPath = ""

		req := r.WithContext(stdctx.WithValue(r.Context(), apiV1RequestKey{}, apiV1Request{
			user: user,
			info: apiv1.RequestInfo{
				IsBasicAuth: method.basicAuth,
				IsTokenAuth: method.tokenAuth,
				Path:        r.URL.Path,
			},
			locale: l,
		}))
		req.URL = &u
		apiHandler.ServeHTTP(c.ResponseWriter(), req)
		// The API v1 route, e.g., "GET /api/v1/repos/{owner}/{repo}", is more
		// specific than the route that bridges to it. Copy it to the outer
		// request, the one the tracing handler reads.
		if req.Pattern != "" {
			r.Pattern = req.Pattern
		}
	}
}

func apiV1Injector(c flamego.Context) {
	req, _ := c.Request().Context().Value(apiV1RequestKey{}).(apiV1Request)
	c.Map(req.user, req.info, req.locale)
}

// requireViewAccess turns away the same viewers as the sign-in toggle of the
// server-rendered pages: disabled users and, when the site requires signing in
// to view any content (`[auth] REQUIRE_SIGNIN_VIEW`), anonymous visitors. Mount
// it on every route that serves site content, but not on the routes visitors
// need to sign in.
func requireViewAccess(c flamego.Context, viewer *database.User) {
	w := c.ResponseWriter()
	switch {
	case viewer != nil && viewer.ProhibitLogin:
		writeErrorResponse(w, http.StatusForbidden, errors.New("login prohibited"))
	case viewer != nil && viewer.IsDisabled():
		writeErrorResponse(w, http.StatusForbidden, errors.New("account not activated"))
	case viewer == nil && conf.Auth.RequireSigninView:
		writeErrorResponse(w, http.StatusUnauthorized, errors.New("authentication required"))
	}
}

func enforceWebAPIMaxBodySize(c flamego.Context) {
	r := c.Request().Request
	r.Body = http.MaxBytesReader(c.ResponseWriter(), r.Body, 4*1024) // 4 KiB
}

// webAPIValidator is the shared validator instance used by every webapi
// binding. Registering the json-tag name function makes validation errors
// carry the wire field name (e.g. "recoveryCode") via ve.Field(), so the
// 400 payload keys match what the React client sends and reads.
var webAPIValidator = func() *validator.Validate {
	v := validator.New()
	v.RegisterTagNameFunc(func(fld reflect.StructField) string {
		name, _, _ := strings.Cut(fld.Tag.Get("json"), ",")
		if name == "-" {
			return ""
		}
		return name
	})
	_ = v.RegisterValidation("alphadashdot", func(fl validator.FieldLevel) bool {
		return !alphaDashDotInvalid.MatchString(fl.Field().String())
	})
	return v
}()

var alphaDashDotInvalid = regexp.MustCompile(`[^\d\w\-_\.]`)

// bindJSON binds the request body to T. On binding or validation failure it
// short-circuits with a 400 carrying the standard renderBindingErrors payload,
// so downstream handlers can drop the `if len(bindErrs) > 0` boilerplate and
// the binding.Errors parameter entirely.
func bindJSON(model any) flamego.Handler {
	return binding.JSON(model, binding.Options{
		Validator: webAPIValidator,
		ErrorHandler: func(c flamego.Context, l i18n.Locale, errs binding.Errors) {
			w := c.ResponseWriter()
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(renderBindingErrors(l, errs))
		},
	})
}

func mountWebAPIRoutes(f *flamego.Flame) {
	f.Group("/api/web", func() {
		f.Group("/user", func() {
			f.Get("/info", getUserInfo)
			f.Combo("/sign-up").
				Get(getUserSignUp).
				Post(bindJSON(userSignUpRequest{}), postUserSignUp)
			f.Group("/reset-password", func() {
				f.Combo("").
					Get(getUserResetPassword).
					Post(bindJSON(userResetPasswordEmailRequest{}), postUserResetPassword)
				f.Post("/complete", bindJSON(userResetPasswordCompleteRequest{}), postUserResetPasswordComplete)
			})
			f.Combo("/sign-in").
				Get(getUserSignIn).
				Post(bindJSON(userSignInRequest{}), postUserSignIn)
			f.Group("/mfa", func() {
				f.Combo("").
					Get(getUserMFA).
					Post(bindJSON(userMFARequest{}), postUserMFA)
				f.Post("/recovery", bindJSON(userMFARecoveryRequest{}), postUserMFARecovery)
			})
			f.Group("/activate", func() {
				f.Combo("").
					Get(getUserActivate).
					Post(postUserActivate)
				f.Post("/complete", bindJSON(userActivateCompleteRequest{}), postUserActivateComplete)
			})
			f.Post("/sign-out", postUserSignOut)
			f.Delete("", requireSignIn, bindJSON(userDeleteRequest{}), deleteUser)
			f.Group("/settings", func() {
				f.Combo("").Get(getUserSettings).Patch(bindJSON(userSettingsRequest{}), patchUserSettings)
				f.Put("/gravatar", bindJSON(userGravatarRequest{}), putUserGravatar)
				f.Put("/password", bindJSON(userPasswordRequest{}), putUserPassword)
				f.Get("/repositories", getUserSettingsRepos)
				f.Delete("/repositories/{id}", deleteUserSettingsRepo)
				f.Get("/organizations", getUserSettingsOrgs)
				f.Combo("/emails").Get(getUserEmails).Post(bindJSON(userEmailRequest{}), postUserEmail)
				f.Delete("/emails/{email}", deleteUserEmail)
				f.Put("/emails/{email}/primary", putUserEmailPrimary)
				f.Combo("/ssh-keys").Get(getUserSSHKeys).Post(bindJSON(userSSHKeyRequest{}), postUserSSHKey)
				f.Delete("/ssh-keys/{id}", deleteUserSSHKey)
				f.Group("/two-factor", func() {
					f.Combo("").Get(getUserTwoFactor).Post(bindJSON(userTwoFactorRequest{}), postUserTwoFactor).Delete(deleteUserTwoFactor)
					f.Get("/enrollment", getUserTwoFactorEnrollment)
					f.Combo("/recovery-codes").Get(getUserRecoveryCodes).Post(postUserRecoveryCodes)
				})
				f.Combo("/tokens").Get(getUserAccessTokens).Post(bindJSON(userAccessTokenRequest{}), postUserAccessToken)
				f.Delete("/tokens/{id}", deleteUserAccessToken)
			}, requireSignIn)
		})
		f.Group("/{owner}/{repo}", func() {
			f.Get("/header", getRepoHeader)
			f.Get("/commit/{sha: /[0-9a-f]{7,64}/}", getRepoCommit)
			f.Combo("/watch").Post(postRepoWatch).Delete(deleteRepoWatch)
			f.Combo("/star").Post(postRepoStar).Delete(deleteRepoStar)
			f.Get("/stargazers", getRepoStargazers)
			f.Get("/watchers", getRepoWatchers)
			f.Delete("/watchers/{username}", deleteRepoWatcher)
			f.Get("/issues", getRepoIssues)
			f.Get("/issues/{index}", withRepoIssues, withRepoIssue, getRepoIssue)
			f.Group("/pulls/{index}", func() {
				f.Get("", getRepoPull)
				f.Get("/commits", getRepoPullCommits)
			}, withRepoPulls, withRepoPull)
			f.Get("/pulls", getRepoPulls)
			f.Get("/forks", getRepoForks)
			f.Get("/releases", withRepoCode, getRepoReleases)
			f.Get("/branches", withRepoCode, getRepoBranches)
			f.Get("/src", getRepoSrc)
			f.Get("/src/{path: **}", getRepoSrc)
			f.Get("/commits/{path: **}", withRepoCode, getRepoCommits)
			f.Get("/blame/{path: **}", withRepoCode, getRepoBlame)
			f.Get("/compare/"+compareRangeRoute, withRepoCode, getRepoCompare)
			f.Group("/wiki", func() {
				f.Get("", getRepoWiki)
				f.Get("/pages", getRepoWikiPages)
			}, withRepoWiki)
			f.Group("", func() {
				f.Get("/labels", getRepoLabels)
				f.Get("/milestones", getRepoMilestones)
			}, withIssueTracker)
		}, requireViewAccess, withRepoContext)
		f.Group("/{owner}/{repo}", func() {
			f.Post("/labels", bindJSON(repoLabelRequest{}), postRepoLabel)
			f.Post("/labels/initialize", bindJSON(repoLabelsInitializeRequest{}), postRepoLabelsInitialize)
			f.Combo("/labels/{id}", withRepoLabel).Patch(bindJSON(repoLabelRequest{}), patchRepoLabel).Delete(deleteRepoLabel)
			f.Post("/milestones", bindJSON(repoMilestoneRequest{}), postRepoMilestone)
			f.Group("/milestones/{id}", func() {
				f.Combo("").
					Get(getRepoMilestone).
					Patch(bindJSON(repoMilestoneRequest{}), patchRepoMilestone).
					Delete(deleteRepoMilestone)
				f.Combo("/closed").Put(putRepoMilestoneClosed).Delete(deleteRepoMilestoneClosed)
			}, withRepoMilestone)
			f.Group("/releases", func() {
				f.Get("/new", getRepoNewRelease)
				f.Combo("/edit/{tag: **}", withRepoRelease).Get(getRepoRelease).Delete(deleteRepoRelease)
			}, withRepoCode)
		}, requireSignIn, withRepoContext, withRepoWriter)
		f.Group("/{owner}/{repo}/wiki", func() {
			f.Get("/source", getRepoWikiPageSource)
			f.Delete("", deleteRepoWikiPage)
		}, requireSignIn, withRepoContext, withRepoWiki, withWikiWriter)
		f.Group("/{owner}/{repo}", func() {
			f.Delete("", bindJSON(repoConfirmRequest{}), deleteRepo)
			f.Group("/settings", func() {
				f.Combo("").Get(getRepoSettings).Patch(bindJSON(repoSettingsRequest{}), patchRepoSettings)
				f.Put("/mirror", bindJSON(repoMirrorRequest{}), putRepoMirror)
				f.Post("/mirror/sync", postRepoMirrorSync)
				f.Put("/advanced", bindJSON(repoAdvancedSettings{}), putRepoAdvanced)
				f.Post("/convert", bindJSON(repoConfirmRequest{}), postRepoConvert)
				f.Post("/transfer", bindJSON(repoTransferRequest{}), postRepoTransfer)
				f.Delete("/wiki", bindJSON(repoConfirmRequest{}), deleteRepoWiki)
				f.Combo("/collaborators").Get(getRepoCollaborators).Post(bindJSON(repoCollaboratorRequest{}), postRepoCollaborator)
				f.Combo("/collaborators/{username}").
					Patch(bindJSON(repoCollaboratorModeRequest{}), patchRepoCollaborator).
					Delete(deleteRepoCollaborator)
				f.Combo("/keys").Get(getRepoDeployKeys).Post(bindJSON(userSSHKeyRequest{}), postRepoDeployKey)
				f.Delete("/keys/{id}", deleteRepoDeployKey)
				f.Group("", func() {
					f.Get("/branches", getRepoBranchSettings)
					f.Put("/default-branch", bindJSON(repoDefaultBranchRequest{}), putRepoDefaultBranch)
					f.Combo("/branches/{branch: **}", withRepoProtectedBranch).
						Get(getRepoProtectedBranch).
						Put(bindJSON(repoProtectedBranchRequest{}), putRepoProtectedBranch)
				}, withRepoBranchSettings)
				f.Group("/git-hooks", func() {
					f.Get("", getRepoGitHooks)
					f.Combo("/{name}", withRepoGitHook).
						Get(getRepoGitHook).
						Put(bindJSON(repoGitHookRequest{}), putRepoGitHook)
				}, withRepoGitHooks)
				f.Group("/hooks", func() {
					f.Combo("").Get(getWebhooks).Post(bindJSON(webhookRequest{}), postWebhook)
					f.Group("/{id}", func() {
						f.Combo("").Get(getWebhook).Patch(bindJSON(webhookRequest{}), patchWebhook).Delete(deleteWebhook)
						f.Post("/test", postRepoWebhookTest)
						f.Post("/deliveries/{uuid}/redeliver", postRepoWebhookRedelivery)
					}, withWebhook)
				}, withRepoWebhooks)
			})
		}, requireSignIn, withRepoContext, withRepoAdmin)
		f.Group("/explore", func() {
			f.Get("/repos", getExploreRepos)
			f.Get("/users", getExploreUsers)
			f.Get("/organizations", getExploreOrganizations)
		}, requireViewAccess)
		f.Group("/users/{username}", func() {
			f.Get("", getUserProfile)
			f.Get("/repos", getUserProfileRepos)
			f.Get("/activity", requireIndividualUser, getUserProfileActivity)
			f.Get("/followers", requireIndividualUser, getUserFollowers)
			f.Get("/following", requireIndividualUser, getUserFollowing)
			f.Combo("/follow", requireIndividualUser).Post(postUserFollow).Delete(deleteUserFollow)
		}, requireViewAccess, withProfileUser)
		dashboardRoutes := func() {
			f.Get("", getDashboard)
			f.Get("/activity", getDashboardActivity)
			f.Get("/issues", getDashboardIssues)
			f.Get("/pulls", getDashboardPulls)
		}
		f.Group("/dashboard", dashboardRoutes, requireSignIn, withDashboardOwner)
		f.Group("/orgs/{org}/dashboard", dashboardRoutes, requireSignIn, withDashboardOwner)
		f.Group("/admin", func() {
			f.Get("/dashboard", getAdminDashboard)
			f.Post("/operations/{name}", postAdminOperation)
			f.Get("/config", getAdminConfig)
			f.Post("/config/test-mail", bindJSON(adminTestMailRequest{}), postAdminTestMail)
			f.Get("/monitor", getAdminMonitor)
			f.Combo("/auths").Get(getAdminAuthSources).Post(bindJSON(adminAuthSourceRequest{}), postAdminAuthSource)
			f.Combo("/auths/{id}", withAdminTargetAuthSource).
				Get(getAdminAuthSource).
				Patch(bindJSON(adminAuthSourceRequest{}), patchAdminAuthSource).
				Delete(deleteAdminAuthSource)
			f.Combo("/users").Get(getAdminUsers).Post(bindJSON(adminUserCreateRequest{}), postAdminUser)
			f.Get("/user-options", getAdminUserOptions)
			f.Combo("/users/{id}", withAdminTargetUser).
				Get(getAdminUser).
				Patch(bindJSON(adminUserUpdateRequest{}), patchAdminUser).
				Delete(deleteAdminUser)
			f.Get("/orgs", getAdminOrgs)
			f.Get("/repos", getAdminRepos)
			f.Delete("/repos/{id}", deleteAdminRepo)
			f.Combo("/notices").Get(getAdminNotices).Delete(bindJSON(adminNoticesDeleteRequest{}), deleteAdminNotices)
			f.Delete("/notices/all", deleteAllAdminNotices)
		}, requireSignIn, requireAdmin)
		f.Group("/repo", func() {
			f.Get("/new", getRepoNew)
			f.Post("/create", bindJSON(repoCreateRequest{}), postRepoCreate)
			f.Post("/migrate", bindJSON(repoMigrateRequest{}), postRepoMigrate)
			f.Combo("/fork/{id}", withForkBaseRepo).Get(getRepoFork).Post(bindJSON(repoForkRequest{}), postRepoFork)
		}, requireSignIn)
		f.Post("/orgs", requireSignIn, bindJSON(orgCreateRequest{}), postOrg)
		f.Group("/orgs/{org}", func() {
			f.Delete("", bindJSON(orgDeleteRequest{}), deleteOrg)
			f.Combo("/settings").Get(getOrgSettings).Patch(bindJSON(orgSettingsRequest{}), patchOrgSettings)
			f.Group("/settings/hooks", func() {
				f.Combo("").Get(getWebhooks).Post(bindJSON(webhookRequest{}), postWebhook)
				f.Combo("/{id}", withWebhook).Get(getWebhook).Patch(bindJSON(webhookRequest{}), patchWebhook).Delete(deleteWebhook)
			}, withOrgWebhooks)
			f.Get("/members", getOrgMembers)
			f.Combo("/members/{username}").Put(putOrgMember).Delete(deleteOrgMember)
			f.Combo("/members/{username}/public").Put(putOrgMemberPublic).Delete(deleteOrgMemberPublic)
			f.Combo("/teams").Get(getOrgTeams).Post(bindJSON(orgTeamRequest{}), postOrgTeam)
			f.Group("/teams/{team}", func() {
				f.Combo("").
					Get(getOrgTeam).
					Patch(bindJSON(orgTeamRequest{}), patchOrgTeam).
					Delete(deleteOrgTeam)
				f.Get("/members", getOrgTeamMembers)
				f.Combo("/members/{username}").Put(putOrgTeamMember).Delete(deleteOrgTeamMember)
				f.Get("/repos", getOrgTeamRepos)
				f.Combo("/repos/{repo}").Put(putOrgTeamRepo).Delete(deleteOrgTeamRepo)
			}, withOrgTeam)
		}, requireSignIn, withOrgMembership)
	}, enforceWebAPIMaxBodySize)
	f.Group("/api/web/{owner}/{repo}", func() {
		f.Post("/markdown", withIssueTracker, bindJSON(markdownRequest{}), postRepoMarkdown)
		f.Group("/issues", func() {
			f.Get("/new", getRepoNewIssue)
			f.Post("", bindJSON(repoNewIssueRequest{}), postRepoIssue)
		}, withRepoIssues)
		// Issues and pull requests take the same changes.
		issueChanges := func() {
			f.Patch("", requireIssueEditor, bindJSON(repoIssueRequest{}), patchRepoIssue)
			f.Combo("/closed", requireIssueEditor).Put(putRepoIssueClosed).Delete(deleteRepoIssueClosed)
			f.Post("/comments", bindJSON(repoIssueCommentRequest{}), postRepoIssueComment)
			f.Combo("/comments/{id}", withIssueComment).
				Patch(bindJSON(repoIssueCommentRequest{}), patchRepoIssueComment).
				Delete(deleteRepoIssueComment)
			f.Group("", func() {
				f.Put("/labels", bindJSON(repoIssueLabelsRequest{}), putRepoIssueLabels)
				f.Put("/milestone", bindJSON(repoIssueIDRequest{}), putRepoIssueMilestone)
				f.Put("/assignee", bindJSON(repoIssueIDRequest{}), putRepoIssueAssignee)
			}, withRepoWriter)
		}
		f.Group("/issues/{index}", issueChanges, withRepoIssues, withRepoIssue)
		f.Group("/pulls", func() {
			f.Get("/new/{range: **}", getRepoNewPull)
			f.Post("", bindJSON(repoNewPullRequest{}), postRepoPull)
		}, withRepoPulls)
		f.Group("/pulls/{index}", func() {
			issueChanges()
			f.Group("", func() {
				f.Post("/merge", bindJSON(repoPullMergeRequest{}), postRepoPullMerge)
				f.Delete("/branch", deleteRepoPullBranch)
			}, withRepoWriter)
		}, withRepoPulls, withRepoPull)
		f.Group("/releases", func() {
			f.Post("", bindJSON(repoReleaseRequest{}), postRepoRelease)
			f.Patch("/edit/{tag: **}", withRepoRelease, bindJSON(repoReleaseRequest{}), patchRepoRelease)
		}, withRepoWriter, withRepoCode)
		f.Combo("/wiki", withRepoWiki, withWikiWriter).
			Post(bindJSON(repoWikiPageRequest{}), postRepoWikiPage).
			Patch(bindJSON(repoWikiPageRequest{}), patchRepoWikiPage)
	}, limitIssueContent, requireSignIn, withRepoContext)
	f.Post("/api/web/{owner}/{repo}/issues/attachments", limitAttachmentUpload(issueAttachmentLimits()), requireSignIn, withRepoContext, withRepoIssuesOrPulls, postRepoIssueAttachment)
	f.Group("/api/web/{owner}/{repo}/editor", func() {
		f.Get("/{path: **}", getRepoEditorTarget)
		f.Post("/files", limitEditorContent, bindJSON(repoEditorFileRequest{}), postRepoEditorFile)
		f.Post("/delete", limitEditorContent, bindJSON(repoEditorDeleteRequest{}), postRepoEditorDelete)
		f.Post("/upload", limitEditorContent, bindJSON(repoEditorUploadRequest{}), postRepoEditorUpload)
		f.Post("/uploads", limitAttachmentUpload(repoUploadLimits()), postRepoEditorUploadFile)
		f.Delete("/uploads/{uuid}", deleteRepoEditorUploadFile)
	}, requireSignIn, withRepoContext, withRepoEditor)
	f.Get("/api/web/{owner}/{repo}/editor-indent", requireSignIn, withRepoContext, withRepoEditor, getRepoEditorIndent)
	f.Post("/api/web/{owner}/{repo}/releases/attachments", limitAttachmentUpload(releaseAttachmentLimits()), requireSignIn, withRepoContext, withRepoWriter, withRepoCode, postRepoReleaseAttachment)
	f.Combo("/api/web/user/settings/avatar", limitAvatarUpload, requireSignIn).
		Put(putUserAvatar).
		Delete(deleteUserAvatar)
	f.Combo("/api/web/orgs/{org}/settings/avatar", limitAvatarUpload, requireSignIn, withOrgMembership).
		Put(putOrgAvatar).
		Delete(deleteOrgAvatar)
	f.Combo("/api/web/{owner}/{repo}/settings/avatar", limitAvatarUpload, requireSignIn, withRepoContext, withRepoAdmin).
		Put(putRepoAvatar).
		Delete(deleteRepoAvatar)
}

// fieldErrors maps JSON field names to per-field localized messages. A non-nil
// value renders inline under the input. A nil value marks the input as
// invalid (highlight + focus eligibility) without duplicating text. Used in
// concert with bindingErrorResponse.Error to surface one banner message while
// highlighting multiple inputs.
type fieldErrors map[string]*string

// bindingErrorResponse carries form-validation failures. Error is the top-level
// message shown as a banner above the form (used when the failure is not tied
// to a specific input, e.g. malformed body, bad credentials).
type bindingErrorResponse struct {
	Error  string      `json:"error,omitempty"`
	Fields fieldErrors `json:"fields,omitempty"`
}

// ruleSuffixKeys maps a validator tag to the shared "form.*_error" suffix key
// (e.g. "max" -> "form.max_size_error"). Messages are composed as
// <field label> + <suffix>, mirroring the legacy Macaron binding behavior.
var ruleSuffixKeys = map[string]string{
	"required":     "form.require_error",
	"max":          "form.max_size_error",
	"min":          "form.min_size_error",
	"len":          "form.size_error",
	"email":        "form.email_error",
	"url":          "form.url_error",
	"alphadashdot": "form.alpha_dash_dot_error",
}

// renderBindingErrors maps binding.Errors to the response shape, looking up
// localized messages via the request's locale. The per-field label comes from
// "form.<StructField>" (e.g. "form.UserName"); the rule suffix comes from
// ruleSuffixKeys. Rule parameters (e.g. "254" for `max=254`) are passed
// through to the suffix translation for %s expansion. Always HTTP 400.
func renderBindingErrors(l i18n.Locale, errs binding.Errors) *bindingErrorResponse {
	for _, e := range errs {
		if e.Category == binding.ErrorCategoryDeserialization {
			return &bindingErrorResponse{Error: l.Tr("form.invalid_request") + ": " + e.Err.Error()}
		}
	}

	out := make(fieldErrors)
	for _, e := range errs {
		var ves validator.ValidationErrors
		ok := errors.As(e.Err, &ves)
		if !ok {
			continue
		}
		for _, ve := range ves {
			field := ve.Field()
			if _, exists := out[field]; exists {
				// Keep the first rule that failed for a given field so the client renders one
				// message per input. Subsequent rules surface only after the first is fixed.
				continue
			}
			label := l.Tr("form." + ve.StructField())
			suffixKey, known := ruleSuffixKeys[ve.Tag()]
			var msg string
			switch {
			case !known:
				msg = l.Tr("form.unknown_error") + " " + ve.Tag()
			case ve.Param() != "":
				msg = label + l.Tr(suffixKey, ve.Param())
			default:
				msg = label + l.Tr(suffixKey)
			}
			out[field] = &msg
		}
	}
	return &bindingErrorResponse{Fields: out}
}
