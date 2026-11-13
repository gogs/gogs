package context

import (
	"context"
	"net"
	"net/http"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/flamego/session"
	"github.com/google/uuid"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/auth"
	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/tool"
)

type AuthStore interface {
	// GetAccessTokenBySHA1 returns the access token with given SHA1. It returns
	// database.ErrAccessTokenNotExist when not found.
	GetAccessTokenBySHA1(ctx context.Context, sha1 string) (*database.AccessToken, error)
	// TouchAccessTokenByID updates the updated time of the given access token to
	// the current time.
	TouchAccessTokenByID(ctx context.Context, id int64) error

	// GetUserByID returns the user with given ID. It returns
	// database.ErrUserNotExist when not found.
	GetUserByID(ctx context.Context, id int64) (*database.User, error)
	// GetUserByUsername returns the user with given username. It returns
	// database.ErrUserNotExist when not found.
	GetUserByUsername(ctx context.Context, username string) (*database.User, error)
	// CreateUser creates a new user and persists to database. It returns
	// database.ErrNameNotAllowed if the given name or pattern of the name is not
	// allowed as a username, or database.ErrUserAlreadyExist when a user with same
	// name already exists, or database.ErrEmailAlreadyUsed if the email has been
	// verified by another user.
	CreateUser(ctx context.Context, username, email string, opts database.CreateUserOptions) (*database.User, error)
	// AuthenticateUser validates username and password via given login source ID.
	// It returns database.ErrUserNotExist when the user was not found.
	//
	// When the "loginSourceID" is negative, it aborts the process and returns
	// database.ErrUserNotExist if the user was not found in the database.
	//
	// When the "loginSourceID" is non-negative, it returns
	// database.ErrLoginSourceMismatch if the user has different login source ID
	// than the "loginSourceID".
	//
	// When the "loginSourceID" is positive, it tries to authenticate via given
	// login source and creates a new user when not yet exists in the database.
	AuthenticateUser(ctx context.Context, login, password string, loginSourceID int64) (*database.User, error)
}

// authenticatedUserID returns the ID of the authenticated user, along with a bool value
// which indicates whether the user uses token authentication.
func authenticatedUserID(store AuthStore, r *http.Request, sess session.Session) (_ int64, isTokenAuth bool) {
	// Check access token.
	if strings.HasPrefix(r.URL.Path, "/api/") {
		var tokenSHA string
		auHead := r.Header.Get("Authorization")
		if auHead != "" {
			auths := strings.Fields(auHead)
			if len(auths) == 2 && auths[0] == "token" {
				tokenSHA = auths[1]
			}
		}

		// Let's see if token is valid.
		if len(tokenSHA) > 0 {
			t, err := store.GetAccessTokenBySHA1(r.Context(), tokenSHA)
			if err != nil {
				if !database.IsErrAccessTokenNotExist(err) {
					log.Error("GetAccessTokenBySHA: %v", err)
				}
				return 0, false
			}
			if err = store.TouchAccessTokenByID(r.Context(), t.ID); err != nil {
				log.Error("Failed to touch access token: %v", err)
			}
			return t.UserID, true
		}
	}

	uid := sess.Get("uid")
	if uid == nil {
		return 0, false
	}
	if id, ok := uid.(int64); ok {
		_, err := store.GetUserByID(r.Context(), id)
		if err != nil {
			if !database.IsErrUserNotExist(err) {
				log.Error("Failed to get user by ID: %v", err)
			}
			return 0, false
		}
		return id, false
	}
	return 0, false
}

// AuthenticatedUser returns the user object of the authenticated user, along with two bool values
// which indicate whether the user uses HTTP Basic Authentication or token authentication respectively.
func AuthenticatedUser(store AuthStore, r *http.Request, sess session.Session) (_ *database.User, isBasicAuth, isTokenAuth bool) {
	uid, isTokenAuth := authenticatedUserID(store, r, sess)

	if uid <= 0 {
		if conf.Auth.EnableReverseProxyAuthentication && isRequestFromTrustedProxy(r) {
			webAuthUser := r.Header.Get(conf.Auth.ReverseProxyAuthenticationHeader)
			if len(webAuthUser) > 0 {
				user, err := store.GetUserByUsername(r.Context(), webAuthUser)
				if err != nil {
					if !database.IsErrUserNotExist(err) {
						log.Error("Failed to get user by name: %v", err)
						return nil, false, false
					}

					// Check if enabled auto-registration.
					if conf.Auth.EnableReverseProxyAutoRegistration {
						user, err = store.CreateUser(
							r.Context(),
							webAuthUser,
							uuid.New().String()+"@localhost",
							database.CreateUserOptions{
								Activated: true,
							},
						)
						if err != nil {
							log.Error("Failed to create user %q: %v", webAuthUser, err)
							return nil, false, false
						}
					}
				}
				return user, false, false
			}
		}

		// Check with basic auth.
		baHead := r.Header.Get("Authorization")
		if len(baHead) > 0 {
			auths := strings.Fields(baHead)
			if len(auths) == 2 && auths[0] == "Basic" {
				uname, passwd, _ := tool.BasicAuthDecode(auths[1])

				u, err := store.AuthenticateUser(r.Context(), uname, passwd, -1)
				if err != nil {
					if !auth.IsErrBadCredentials(err) {
						log.Error("Failed to authenticate user: %v", err)
					}
					return nil, false, false
				}
				if u.IsDisabled() {
					return nil, false, false
				}

				return u, true, false
			}
		}
		return nil, false, false
	}

	u, err := store.GetUserByID(r.Context(), uid)
	if err != nil {
		log.Error("GetUserByID: %v", err)
		return nil, false, false
	}
	if isTokenAuth && u.IsDisabled() {
		return nil, false, false
	}
	return u, false, isTokenAuth
}

// isRequestFromTrustedProxy reports whether the request's immediate remote
// address falls within one of the configured trusted proxy CIDR ranges. The
// reverse proxy authentication header is only honored for such requests so an
// attacker reaching Gogs directly cannot forge it.
func isRequestFromTrustedProxy(req *http.Request) bool {
	host, _, err := net.SplitHostPort(req.RemoteAddr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	// Normalize IPv4-mapped IPv6 (e.g. "::ffff:127.0.0.1" on dual-stack listeners)
	// to its IPv4 form so it matches IPv4 CIDRs like 127.0.0.0/8.
	if v4 := ip.To4(); v4 != nil {
		ip = v4
	}
	for _, cidr := range conf.Auth.TrustedProxyCIDRs {
		if cidr.Contains(ip) {
			return true
		}
	}
	return false
}

// AuthenticateByToken attempts to authenticate a user by the given access
// token. It returns database.ErrAccessTokenNotExist when the access token does not
// exist.
func AuthenticateByToken(store AuthStore, ctx context.Context, token string) (*database.User, error) {
	t, err := store.GetAccessTokenBySHA1(ctx, token)
	if err != nil {
		return nil, errors.Wrap(err, "get access token by SHA1")
	}
	if err = store.TouchAccessTokenByID(ctx, t.ID); err != nil {
		// NOTE: There is no need to fail the auth flow if we can't touch the token.
		log.Error("Failed to touch access token [id: %d]: %v", t.ID, err)
	}

	user, err := store.GetUserByID(ctx, t.UserID)
	if err != nil {
		return nil, errors.Wrapf(err, "get user by ID [user_id: %d]", t.UserID)
	}
	return user, nil
}
