package lfs

import (
	"net/http"
	"strings"

	"github.com/flamego/flamego"
	"unknwon.dev/x/logx"

	"gogs.io/gogs/internal/auth"
	"gogs.io/gogs/internal/authx"
	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/lfsx"
)

// RegisterRoutes registers LFS routes using given router, and inherits all
// groups and middleware. The routes take the repository from the "owner" and
// "repo" path parameters.
func RegisterRoutes(logger *logx.Logger, r flamego.Router) {
	verifyAccept := verifyHeader(logger, "Accept", contentType, http.StatusNotAcceptable)
	verifyContentTypeJSON := verifyHeader(logger, "Content-Type", contentType, http.StatusBadRequest)
	verifyContentTypeStream := verifyHeader(logger, "Content-Type", "application/octet-stream", http.StatusBadRequest)

	store := NewStore(logger)
	r.Group("", func() {
		r.Post("/objects/batch", authorize(logger, store, database.AccessModeRead), verifyAccept, verifyContentTypeJSON, serveBatch(logger, store))
		r.Group("/objects/basic", func() {
			r.Combo("/{oid}", verifyOID(logger)).
				Get(authorize(logger, store, database.AccessModeRead), (*basicHandler).serveDownload).
				Put(authorize(logger, store, database.AccessModeWrite), verifyContentTypeStream, (*basicHandler).serveUpload)
			r.Post("/verify", authorize(logger, store, database.AccessModeWrite), verifyAccept, verifyContentTypeJSON, (*basicHandler).serveVerify)
		}, func(c flamego.Context, lfsOpts conf.LFSOpts) {
			c.Map(&basicHandler{
				logger:         logger,
				store:          store,
				defaultStorage: lfsx.Storage(lfsOpts.Storage),
				storagers: map[lfsx.Storage]lfsx.Storager{
					lfsx.StorageLocal: &lfsx.LocalStorage{Root: lfsOpts.ObjectsPath, TempDir: lfsOpts.ObjectsTempPath},
				},
			})
		})
	}, authenticate(logger, store))
}

// askCredentials responds that the client needs to provide credentials.
func askCredentials(logger *logx.Logger, w http.ResponseWriter) {
	w.Header().Set("Lfs-Authenticate", `Basic realm="Git LFS"`)
	responseJSON(logger, w, http.StatusUnauthorized, responseError{
		Message: "Credentials needed",
	})
}

// authenticate tries to authenticate the user via HTTP Basic Auth. It first tries to authenticate
// with plain username and password, then uses the username as an access token if that failed.
// A request without credentials continues as an anonymous user, whose ID is 0,
// unless signing in is required to view anything.
func authenticate(logger *logx.Logger, store Store) flamego.Handler {
	return func(c flamego.Context, authOpts conf.AuthOpts) {
		// Like Git over HTTP, a user named by a trusted reverse proxy goes before
		// Basic authentication.
		user, viaProxy, err := context.ReverseProxyUser(authOpts, store, c.Request().Request)
		if err != nil {
			internalServerError(logger, c.ResponseWriter())
			logger.Error("Failed to authenticate by reverse proxy header", "error", err)
			return
		} else if viaProxy && user == nil {
			// Like Git over HTTP, which serves public repositories without
			// authentication, the viewer may still download what anyone can.
			if authOpts.RequireSigninView {
				c.ResponseWriter().WriteHeader(http.StatusNotFound)
				return
			}
			c.Map(&database.User{Name: "anonymous"})
			return
		}

		if !viaProxy {
			username, password := authx.DecodeBasic(c.Request().Header)
			if username == "" {
				if authOpts.RequireSigninView {
					askCredentials(logger, c.ResponseWriter())
					return
				}
				c.Map(&database.User{Name: "anonymous"})
				return
			}

			user, err = store.AuthenticateUser(c.Request().Context(), username, password, -1)
			if err != nil && !auth.IsErrBadCredentials(err) {
				internalServerError(logger, c.ResponseWriter())
				logger.Error("Failed to authenticate user", "userName", username, "error", err)
				return
			}

			if err == nil && store.IsTwoFactorEnabled(c.Request().Context(), user.ID) {
				w := c.ResponseWriter()
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte("Users with 2FA enabled are not allowed to authenticate via username and password."))
				return
			}

			// If username and password combination failed, try again using either username
			// or password as the token.
			if auth.IsErrBadCredentials(err) {
				user, err = context.AuthenticateByToken(c.Request().Context(), logger, store, username)
				if err != nil && !database.IsErrAccessTokenNotExist(err) {
					internalServerError(logger, c.ResponseWriter())
					logger.Error("Failed to authenticate by access token via username", "error", err)
					return
				} else if database.IsErrAccessTokenNotExist(err) {
					// Try again using the password field as the token.
					user, err = context.AuthenticateByToken(c.Request().Context(), logger, store, password)
					if err != nil {
						if database.IsErrAccessTokenNotExist(err) {
							askCredentials(logger, c.ResponseWriter())
						} else {
							c.ResponseWriter().WriteHeader(http.StatusInternalServerError)
							logger.Error("Failed to authenticate by access token via password", "error", err)
						}
						return
					}
				}
			}
		}

		if user.IsDisabled(authOpts.RequireEmailConfirmation) {
			responseJSON(logger, c.ResponseWriter(), http.StatusForbidden, responseError{
				Message: "User account is disabled",
			})
			return
		}

		logger.Debug("Authenticated user", "userName", user.Name)

		c.Map(user)
	}
}

// authorize tries to authorize the user to the context repository with given access mode.
func authorize(logger *logx.Logger, store Store, mode database.AccessMode) flamego.Handler {
	return func(c flamego.Context, authOpts conf.AuthOpts, actor *database.User) {
		username := c.Param("owner")
		reponame := strings.TrimSuffix(c.Param("repo"), ".git")

		owner, err := store.GetUserByUsername(c.Request().Context(), username)
		if err != nil {
			if database.IsErrUserNotExist(err) {
				c.ResponseWriter().WriteHeader(http.StatusNotFound)
			} else {
				internalServerError(logger, c.ResponseWriter())
				logger.Error("Failed to get user", "userName", username, "error", err)
			}
			return
		}

		repo, err := store.GetRepositoryByName(c.Request().Context(), owner.ID, reponame)
		if err != nil {
			if database.IsErrRepoNotExist(err) {
				c.ResponseWriter().WriteHeader(http.StatusNotFound)
			} else {
				internalServerError(logger, c.ResponseWriter())
				logger.Error("Failed to get repository", "ownerID", owner.ID, "repoName", reponame, "error", err)
			}
			return
		}

		if !store.AuthorizeRepositoryAccess(c.Request().Context(), actor.ID, repo.ID, mode,
			database.AccessModeOptions{
				OwnerID: repo.OwnerID,
				Private: repo.IsPrivate,
			},
		) {
			// An anonymous user may have access after signing in, unless it is a
			// user named by a trusted reverse proxy that has no account, which
			// credentials cannot change.
			if _, viaProxy := context.ReverseProxyUsername(authOpts, c.Request().Request); actor.ID <= 0 && !viaProxy {
				askCredentials(logger, c.ResponseWriter())
				return
			}
			c.ResponseWriter().WriteHeader(http.StatusNotFound)
			return
		}

		logger.Debug("Authorized user", "userName", actor.Name, "ownerName", username, "repoName", reponame)

		c.Map(owner) // NOTE: Override actor, i.e., later handlers get the owner as *database.User.
		c.Map(repo)
	}
}

// verifyHeader checks if the HTTP header contains given value.
// When not, it responds with "failCode" as the status code.
func verifyHeader(logger *logx.Logger, key, value string, failCode int) flamego.Handler {
	return func(c flamego.Context) {
		vals := c.Request().Header.Values(key)
		for _, val := range vals {
			if strings.Contains(val, value) {
				return
			}
		}

		logger.Debug("HTTP header does not contain expected value", "header", key, "expectedValue", value)
		c.ResponseWriter().WriteHeader(failCode)
	}
}

// verifyOID checks if the "oid" URL parameter is valid.
func verifyOID(logger *logx.Logger) flamego.Handler {
	return func(c flamego.Context) {
		oid := lfsx.OID(c.Param("oid"))
		if !lfsx.ValidOID(oid) {
			responseJSON(logger, c.ResponseWriter(), http.StatusBadRequest, responseError{
				Message: "Invalid oid",
			})
			return
		}

		c.Map(oid)
	}
}

func internalServerError(logger *logx.Logger, w http.ResponseWriter) {
	responseJSON(logger, w, http.StatusInternalServerError, responseError{
		Message: "Internal server error",
	})
}
