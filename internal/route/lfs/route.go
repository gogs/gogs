package lfs

import (
	"net/http"
	"strings"
	"time"

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
			basic := &basicHandler{
				logger:         logger,
				store:          store,
				defaultStorage: lfsx.Storage(conf.LFS.Storage),
				storagers: map[lfsx.Storage]lfsx.Storager{
					lfsx.StorageLocal: &lfsx.LocalStorage{Root: conf.LFS.ObjectsPath, TempDir: conf.LFS.ObjectsTempPath},
				},
			}
			r.Combo("/{oid}", verifyOID(logger)).
				Get(authorize(logger, store, database.AccessModeRead), basic.serveDownload).
				Put(authorize(logger, store, database.AccessModeWrite), verifyContentTypeStream, basic.serveUpload)
			r.Post("/verify", authorize(logger, store, database.AccessModeWrite), verifyAccept, verifyContentTypeJSON, basic.serveVerify)
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
// unless signing in is required to view anything. A request with a bearer
// token, which "gogs serv" gives to clients that authenticated over SSH, is
// limited to what the token grants, which is mapped as *lfsx.Token.
func authenticate(logger *logx.Logger, store Store) flamego.Handler {
	return func(c flamego.Context) {
		if bearer, ok := strings.CutPrefix(c.Request().Header.Get("Authorization"), "Bearer "); ok {
			authenticateToken(c, logger, store, bearer)
			return
		}
		c.Map((*lfsx.Token)(nil))

		// Like Git over HTTP, a user named by a trusted reverse proxy goes before
		// Basic authentication.
		user, viaProxy, err := context.ReverseProxyUser(store, c.Request().Request)
		if err != nil {
			internalServerError(logger, c.ResponseWriter())
			logger.Error("Failed to authenticate by reverse proxy header", "error", err)
			return
		} else if viaProxy && user == nil {
			// Like Git over HTTP, which serves public repositories without
			// authentication, the viewer may still download what anyone can.
			if conf.Auth.RequireSigninView {
				c.ResponseWriter().WriteHeader(http.StatusNotFound)
				return
			}
			c.Map(&database.User{Name: "anonymous"})
			return
		}

		if !viaProxy {
			username, password := authx.DecodeBasic(c.Request().Header)
			if username == "" {
				if conf.Auth.RequireSigninView {
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

		if user.IsDisabled() {
			responseJSON(logger, c.ResponseWriter(), http.StatusForbidden, responseError{
				Message: "User account is disabled",
			})
			return
		}

		logger.Debug("Authenticated user", "userName", user.Name)

		c.Map(user)
	}
}

// authenticateToken authenticates the user of the bearer token.
func authenticateToken(c flamego.Context, logger *logx.Logger, store Store, bearer string) {
	token, err := lfsx.ParseToken(conf.Security.SecretKey, bearer, time.Now())
	if err != nil {
		logger.Debug("Invalid LFS token", "error", err)
		askCredentials(logger, c.ResponseWriter())
		return
	}

	user, err := store.GetUserByID(c.Request().Context(), token.UserID)
	if err != nil {
		if database.IsErrUserNotExist(err) {
			askCredentials(logger, c.ResponseWriter())
		} else {
			internalServerError(logger, c.ResponseWriter())
			logger.Error("Failed to get user", "userID", token.UserID, "error", err)
		}
		return
	}
	if user.IsDisabled() {
		responseJSON(logger, c.ResponseWriter(), http.StatusForbidden, responseError{
			Message: "User account is disabled",
		})
		return
	}

	logger.Debug("Authenticated user by token", "userName", user.Name)
	c.Map(user)
	c.Map(token)
}

// authorize tries to authorize the user to the context repository with given
// access mode, within what the token grants when the user authenticated with
// one.
func authorize(logger *logx.Logger, store Store, mode database.AccessMode) flamego.Handler {
	return func(c flamego.Context, actor *database.User, token *lfsx.Token) {
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

		if token != nil && (token.RepoID != repo.ID || mode > database.AccessModeRead && !token.Upload) {
			responseJSON(logger, c.ResponseWriter(), http.StatusForbidden, responseError{
				Message: "The token does not grant this access",
			})
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
			if _, viaProxy := context.ReverseProxyUsername(c.Request().Request); actor.ID <= 0 && !viaProxy {
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
