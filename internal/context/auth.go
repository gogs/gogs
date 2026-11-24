package context

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net"
	"net/http"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/flamego/session"
	"github.com/google/uuid"
	"unknwon.dev/x/logx"

	"gogs.io/gogs/internal/auth"
	"gogs.io/gogs/internal/authx"
	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/database"
)

type AuthStore interface {
	// GetAccessTokenBySHA1 returns the access token with given SHA1. It returns
	// database.ErrAccessTokenNotExist when not found.
	GetAccessTokenBySHA1(ctx context.Context, sha1 string) (*database.AccessToken, error)
	// TouchAccessTokenByID updates the updated time of the given access token to
	// the current time.
	TouchAccessTokenByID(ctx context.Context, id int64) error

	// IsTwoFactorEnabled returns true if the user has enabled 2FA.
	IsTwoFactorEnabled(ctx context.Context, userID int64) bool

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
	// It returns auth.ErrBadCredentials when the credentials are not valid.
	//
	// When the "loginSourceID" is not positive, it aborts the process and returns
	// auth.ErrBadCredentials if the user was not found in the database.
	//
	// When the "loginSourceID" is non-negative, it returns
	// database.ErrLoginSourceMismatch if the user has different login source ID
	// than the "loginSourceID".
	//
	// When the "loginSourceID" is positive, it tries to authenticate via given
	// login source and creates a new user when not yet exists in the database.
	AuthenticateUser(ctx context.Context, login, password string, loginSourceID int64) (*database.User, error)
}

// SessionKeyPasswordFingerprint is the session key of the PasswordFingerprint
// of the signed-in user when they signed in, so that changing or resetting the
// password signs out every session.
const SessionKeyPasswordFingerprint = "passwordFingerprint"

// PasswordFingerprint returns a value that changes whenever the password of the
// user changes, without revealing the password hash.
func PasswordFingerprint(u *database.User) string {
	sum := sha256.Sum256([]byte("session:" + u.Salt + ":" + u.Password))
	return hex.EncodeToString(sum[:16])
}

// authenticatedUserID returns the ID of the authenticated user, along with the
// ID of the access token it authenticated with, which is 0 when the user does
// not use token authentication.
func authenticatedUserID(logger *logx.Logger, store AuthStore, r *http.Request, sess session.Session) (_, tokenID int64) {
	// Access tokens are for API v1 only. The web API under "/api/web" serves the
	// browser, including the routes that change the credentials and two-factor
	// authentication of an account, which a leaked token must not reach.
	if strings.HasPrefix(r.URL.Path, "/api/v1/") {
		var tokenSHA string
		auths := strings.Fields(r.Header.Get("Authorization"))
		if len(auths) == 2 && auths[0] == "token" {
			tokenSHA = auths[1]
		}

		if tokenSHA != "" {
			t, err := store.GetAccessTokenBySHA1(r.Context(), tokenSHA)
			if err != nil {
				if !database.IsErrAccessTokenNotExist(err) {
					logger.Error("Failed to get access token by SHA1", "error", err)
				}
				return 0, 0
			}
			if err = store.TouchAccessTokenByID(r.Context(), t.ID); err != nil {
				logger.Error("Failed to touch access token", "tokenID", t.ID, "error", err)
			}
			return t.UserID, t.ID
		}
	}

	uid := sess.Get("uid")
	if uid == nil {
		return 0, 0
	}
	if id, ok := uid.(int64); ok {
		u, err := store.GetUserByID(r.Context(), id)
		if err != nil {
			if !database.IsErrUserNotExist(err) {
				logger.Error("Failed to get user by ID", "userID", id, "error", err)
			}
			return 0, 0
		}

		fingerprint := PasswordFingerprint(u)
		switch sess.Get(SessionKeyPasswordFingerprint) {
		case fingerprint:
		case nil:
			// Sessions from before the fingerprint was stored stay signed in until the
			// password changes.
			sess.Set(SessionKeyPasswordFingerprint, fingerprint)
		default:
			sess.Flush()
			return 0, 0
		}
		return id, 0
	}
	return 0, 0
}

// AuthenticatedUser returns the user object of the authenticated user, along
// with whether the user uses HTTP Basic Authentication, and the ID of the access
// token the user authenticated with, which is 0 when the user does not use token
// authentication.
func AuthenticatedUser(logger *logx.Logger, store AuthStore, r *http.Request, sess session.Session) (_ *database.User, isBasicAuth bool, tokenID int64) {
	uid, tokenID := authenticatedUserID(logger, store, r, sess)

	if uid <= 0 {
		user, ok, err := ReverseProxyUser(store, r)
		if err != nil {
			logger.Error("Failed to authenticate by reverse proxy header", "error", err)
			return nil, false, 0
		} else if ok {
			return user, false, 0
		}

		// Check with basic auth. The web API under "/api/web" serves the browser,
		// which signs in with the session, and includes the routes that change
		// the credentials and two-factor authentication of an account. A password
		// alone, without the second factor, must not reach them.
		//
		// The check uses the route the request matched rather than its path,
		// because the router also matches paths written differently, e.g.,
		// "//api/web/...". Before routing, there is no route and basic auth is
		// refused.
		_, route, _ := strings.Cut(r.Pattern, " ")
		if route != "" && !strings.HasPrefix(route, "/api/web/") {
			uname, passwd := authx.DecodeBasic(r.Header)
			if uname != "" || passwd != "" {
				u, err := store.AuthenticateUser(r.Context(), uname, passwd, -1)
				if err != nil {
					if !auth.IsErrBadCredentials(err) {
						logger.Error("Failed to authenticate user", "error", err)
					}
					return nil, false, 0
				}
				// A password alone must not stand in for a sign-in that also asks for
				// the second factor. Such users authenticate with an access token.
				if u.IsDisabled() || store.IsTwoFactorEnabled(r.Context(), u.ID) {
					return nil, false, 0
				}

				return u, true, 0
			}
		}
		return nil, false, 0
	}

	u, err := store.GetUserByID(r.Context(), uid)
	if err != nil {
		logger.Error("Failed to get user by ID", "userID", uid, "error", err)
		return nil, false, 0
	}
	// Organizations cannot sign in, even with a session or an access token that
	// belongs to one.
	if u.IsOrganization() || (tokenID > 0 && u.IsDisabled()) {
		return nil, false, 0
	}
	return u, false, tokenID
}

// ReverseProxyUser returns the user named in the reverse proxy authentication
// header (`[auth] REVERSE_PROXY_AUTHENTICATION_HEADER`), creating it when
// auto-registration is enabled. The bool is false when the header does not
// apply to the request: the feature is off, the request did not come from a
// trusted proxy, or the header is empty. Otherwise the header decides who the
// viewer is, and the user is nil when it names a user that does not exist, an
// organization or, with auto-registration, a name that is not allowed.
func ReverseProxyUser(store AuthStore, r *http.Request) (_ *database.User, ok bool, _ error) {
	username, ok := ReverseProxyUsername(r)
	if !ok {
		return nil, false, nil
	}

	user, err := store.GetUserByUsername(r.Context(), username)
	if err == nil {
		if user.IsOrganization() {
			return nil, true, nil
		}
		return user, true, nil
	} else if !database.IsErrUserNotExist(err) {
		return nil, true, errors.Wrap(err, "get user by name")
	}

	if !conf.Auth.EnableReverseProxyAutoRegistration {
		return nil, true, nil
	}
	user, err = store.CreateUser(
		r.Context(),
		username,
		uuid.New().String()+"@localhost",
		database.CreateUserOptions{
			Activated: true,
		},
	)
	if err == nil {
		return user, true, nil
	} else if database.IsErrNameNotAllowed(err) {
		return nil, true, nil
	} else if database.IsErrUserAlreadyExist(err) {
		// Another request has just registered the same user.
		user, err = store.GetUserByUsername(r.Context(), username)
		if err != nil {
			return nil, true, errors.Wrap(err, "get user by name after concurrent registration")
		} else if user.IsOrganization() {
			return nil, true, nil
		}
		return user, true, nil
	}
	return nil, true, errors.Wrapf(err, "create user %q", username)
}

// ReverseProxyUsername returns the username in the reverse proxy
// authentication header. The bool is false when the header does not apply to
// the request, as described for ReverseProxyUser.
func ReverseProxyUsername(r *http.Request) (string, bool) {
	if !conf.Auth.EnableReverseProxyAuthentication || !isRequestFromTrustedProxy(r) {
		return "", false
	}
	username := r.Header.Get(conf.Auth.ReverseProxyAuthenticationHeader)
	return username, username != ""
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
	// Normalize IPv4-mapped IPv6 (e.g., "::ffff:127.0.0.1" on dual-stack listeners)
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
// exist or belongs to an organization, which cannot sign in.
func AuthenticateByToken(ctx context.Context, logger *logx.Logger, store AuthStore, token string) (*database.User, error) {
	t, err := store.GetAccessTokenBySHA1(ctx, token)
	if err != nil {
		return nil, errors.Wrap(err, "get access token by SHA1")
	}
	if err = store.TouchAccessTokenByID(ctx, t.ID); err != nil {
		// NOTE: There is no need to fail the auth flow if we can't touch the token.
		logger.Error("Failed to touch access token", "tokenID", t.ID, "error", err)
	}

	user, err := store.GetUserByID(ctx, t.UserID)
	if err != nil {
		return nil, errors.Wrapf(err, "get user by ID [user_id: %d]", t.UserID)
	} else if user.IsOrganization() {
		return nil, database.ErrAccessTokenNotExist{}
	}
	return user, nil
}
