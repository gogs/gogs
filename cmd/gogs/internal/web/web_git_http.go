package web

import (
	"bytes"
	"compress/gzip"
	stdctx "context"
	"fmt"
	"net/http"
	"path"
	"strconv"
	"strings"

	"github.com/flamego/flamego"
	"unknwon.dev/x/logx"

	"gogs.io/gogs/internal/auth"
	"gogs.io/gogs/internal/authx"
	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/cryptox"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/git"
	"gogs.io/gogs/internal/lazyregexp"
	"gogs.io/gogs/internal/osx"
)

type gitHTTPContext struct {
	flamego.Context
	OwnerName string
	OwnerSalt string
	RepoID    int64
	RepoName  string
	// RepoPath is the path of the Git repository the request is for, i.e., the
	// repository or its wiki.
	RepoPath string
	AuthUser *database.User
}

// writeError writes the status and the text of an error response.
func writeError(w http.ResponseWriter, status int, text string) {
	w.WriteHeader(status)
	if text != "" {
		_, _ = w.Write([]byte(text))
	}
}

// askCredentials responds with the HTTP header and status that inform the client to provide credentials.
func askCredentials(c flamego.Context, status int, text string) {
	c.ResponseWriter().Header().Set("WWW-Authenticate", "Basic realm=\".\"")
	writeError(c.ResponseWriter(), status, text)
}

// gitHTTPAction returns the Git HTTP path suffix after /{owner}/{repo}/.
func gitHTTPAction(c flamego.Context) string {
	return gitHTTPActionFromPath(
		c.Request().URL.Path,
		conf.Server.Subpath,
		c.Param("owner"),
		c.Param("repo"),
	)
}

func gitHTTPActionFromPath(urlPath, subpath, owner, repo string) string {
	if subpath != "" {
		urlPath = strings.TrimPrefix(urlPath, subpath)
	}
	prefix := path.Join("/", owner, repo) + "/"
	if after, ok := strings.CutPrefix(urlPath, prefix); ok {
		return after
	}
	return ""
}

func gitHTTPIsPull(c flamego.Context, action string) bool {
	if action == "info/refs" {
		return c.Query("service") != "git-receive-pack"
	}
	return action != "git-receive-pack"
}

func gitHTTPContexter(logger *logx.Logger, store gitHTTPStore) flamego.Handler {
	return func(c flamego.Context) {
		if len(conf.HTTP.AccessControlAllowOrigin) > 0 {
			c.ResponseWriter().Header().Set("Access-Control-Allow-Origin", conf.HTTP.AccessControlAllowOrigin)
			c.ResponseWriter().Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, User-Agent, Git-Protocol")
			if c.Request().Method == "OPTIONS" {
				c.ResponseWriter().WriteHeader(http.StatusOK)
				return
			}
		}

		ownerName := c.Param("owner")
		repoName := strings.TrimSuffix(c.Param("repo"), ".git")
		repoName, isWiki := strings.CutSuffix(repoName, ".wiki")

		action := gitHTTPAction(c)
		isPull := gitHTTPIsPull(c, action)

		owner, err := store.GetUserByUsername(c.Request().Context(), ownerName)
		if err != nil {
			if database.IsErrUserNotExist(err) {
				c.ResponseWriter().WriteHeader(http.StatusNotFound)
			} else {
				c.ResponseWriter().WriteHeader(http.StatusInternalServerError)
				logger.Error("Failed to get user", "userName", ownerName, "error", err)
			}
			return
		}

		repo, err := store.GetRepositoryByName(c.Request().Context(), owner.ID, repoName)
		if err != nil {
			if database.IsErrRepoNotExist(err) {
				c.ResponseWriter().WriteHeader(http.StatusNotFound)
			} else {
				c.ResponseWriter().WriteHeader(http.StatusInternalServerError)
				logger.Error("Failed to get repository", "ownerID", owner.ID, "repoName", repoName, "error", err)
			}
			return
		}

		repoPath := database.RepoPath(owner.Name, repo.Name)
		if isWiki {
			repoPath = database.WikiPath(owner.Name, repo.Name)
		}

		// Authentication is not required for pulling from public repositories.
		if isPull && !repo.IsPrivate && !conf.Auth.RequireSigninView {
			c.Map(&gitHTTPContext{
				Context:  c,
				RepoPath: repoPath,
			})
			return
		}

		// In case user requested a wrong URL and not intended to access Git objects.
		if !strings.Contains(action, "git-") &&
			!strings.Contains(action, "info/") &&
			!strings.Contains(action, "HEAD") &&
			!strings.Contains(action, "objects/") {
			writeError(c.ResponseWriter(), http.StatusBadRequest, fmt.Sprintf("Unrecognized action %q", action))
			return
		}

		// A trusted reverse proxy that has authenticated the user names it in a
		// header. Proxies doing Basic authentication themselves often pass the
		// client's "Authorization" header along, so the header goes first.
		authUser, viaProxy, err := context.ReverseProxyUser(store, c.Request().Request)
		if err != nil {
			c.ResponseWriter().WriteHeader(http.StatusInternalServerError)
			logger.Error("Failed to authenticate by reverse proxy header", "error", err)
			return
		} else if viaProxy && authUser == nil {
			// The user has no account, so it sees what a user without access
			// does. Credentials cannot change that, so the client must not be
			// asked for them.
			c.ResponseWriter().WriteHeader(http.StatusNotFound)
			return
		}

		if !viaProxy {
			// Handle HTTP Basic Authentication
			authUsername, authPassword := authx.DecodeBasic(c.Request().Header)
			if authUsername == "" && authPassword == "" {
				askCredentials(c, http.StatusUnauthorized, "")
				return
			}

			authUser, err = store.AuthenticateUser(c.Request().Context(), authUsername, authPassword, -1)
			if err != nil && !auth.IsErrBadCredentials(err) {
				c.ResponseWriter().WriteHeader(http.StatusInternalServerError)
				logger.Error("Failed to authenticate user", "userName", authUsername, "error", err)
				return
			}

			// If username and password combination failed, try again using either username
			// or password as the token.
			if authUser == nil {
				authUser, err = context.AuthenticateByToken(c.Request().Context(), logger, store, authUsername)
				if err != nil && !database.IsErrAccessTokenNotExist(err) {
					c.ResponseWriter().WriteHeader(http.StatusInternalServerError)
					logger.Error("Failed to authenticate by access token via username", "error", err)
					return
				} else if database.IsErrAccessTokenNotExist(err) {
					// Try again using the password field as the token.
					authUser, err = context.AuthenticateByToken(c.Request().Context(), logger, store, authPassword)
					if err != nil {
						if database.IsErrAccessTokenNotExist(err) {
							askCredentials(c, http.StatusUnauthorized, "")
						} else {
							c.ResponseWriter().WriteHeader(http.StatusInternalServerError)
							logger.Error("Failed to authenticate by access token via password", "error", err)
						}
						return
					}
				}
			} else if store.IsTwoFactorEnabled(c.Request().Context(), authUser.ID) {
				askCredentials(c, http.StatusUnauthorized, `User with two-factor authentication enabled cannot perform HTTP/HTTPS operations via plain username and password
Please create and use personal access token on user settings page`)
				return
			}
		}

		if authUser.IsDisabled() {
			askCredentials(c, http.StatusForbidden, "User account is disabled")
			return
		}

		logger.Debug("Authenticated user", "userName", authUser.Name)

		mode := database.AccessModeWrite
		if isPull {
			mode = database.AccessModeRead
		}
		if !database.Handle.Permissions().Authorize(c.Request().Context(), logger, authUser.ID, repo.ID, mode,
			database.AccessModeOptions{
				OwnerID: repo.OwnerID,
				Private: repo.IsPrivate,
			},
		) {
			askCredentials(c, http.StatusForbidden, "User permission denied")
			return
		}

		if !isPull && repo.IsMirror {
			writeError(c.ResponseWriter(), http.StatusForbidden, "Mirror repository is read-only")
			return
		}

		if !isPull && isWiki {
			repo.Owner = owner
			if err := repo.InitWikiForPush(c.Request().Context()); err != nil {
				c.ResponseWriter().WriteHeader(http.StatusInternalServerError)
				logger.Error("Failed to initialize wiki", "repoID", repo.ID, "error", err)
				return
			}
		}

		c.Map(&gitHTTPContext{
			Context:   c,
			OwnerName: ownerName,
			OwnerSalt: owner.Salt,
			RepoID:    repo.ID,
			RepoName:  repoName,
			RepoPath:  repoPath,
			AuthUser:  authUser,
		})
	}
}

type serviceHandler struct {
	w   http.ResponseWriter
	r   *http.Request
	dir string

	authUser  *database.User
	ownerName string
	ownerSalt string
	repoID    int64
	repoName  string
}

func (h *serviceHandler) setHeaderNoCache() {
	h.w.Header().Set("Expires", "Fri, 01 Jan 1980 00:00:00 GMT")
	h.w.Header().Set("Pragma", "no-cache")
	h.w.Header().Set("Cache-Control", "no-cache, max-age=0, must-revalidate")
}

func serviceRPC(logger *logx.Logger, h serviceHandler, service string) {
	defer h.r.Body.Close()

	if h.r.Header.Get("Content-Type") != fmt.Sprintf("application/x-git-%s-request", service) {
		h.w.WriteHeader(http.StatusUnauthorized)
		return
	}
	h.w.Header().Set("Content-Type", fmt.Sprintf("application/x-git-%s-result", service))

	var (
		reqBody = h.r.Body
		err     error
	)

	// Handle GZIP
	if h.r.Header.Get("Content-Encoding") == "gzip" {
		reqBody, err = gzip.NewReader(reqBody)
		if err != nil {
			logger.Error("Failed to create gzip reader", "error", err)
			h.w.WriteHeader(http.StatusInternalServerError)
			return
		}
	}

	var envs []string
	if service == "receive-pack" {
		envs = database.ComposeHookEnvs(database.ComposeHookEnvsOptions{
			AuthUser:  h.authUser,
			OwnerName: h.ownerName,
			OwnerSalt: h.ownerSalt,
			RepoID:    h.repoID,
			RepoName:  h.repoName,
			RepoPath:  h.dir,
		})
	}
	// A client disconnecting must not kill Git midway through a push, e.g., while
	// running hooks after refs were updated, nor stop recording what it pushed.
	ctx := h.r.Context()
	if service == "receive-pack" {
		ctx = stdctx.WithoutCancel(ctx)
	}
	var stderr bytes.Buffer
	opts := git.ServiceOptions{
		StatelessRPC: true,
		Stdin:        reqBody,
		Stdout:       h.w,
		Stderr:       &stderr,
		Protocol:     h.r.Header.Get("Git-Protocol"),
		Envs:         envs,
	}
	if service == "receive-pack" {
		var updates []git.RefUpdate
		updates, err = git.RunReceivePack(ctx, h.dir, opts)
		// TODO: support news feeds for wiki
		if !strings.HasSuffix(h.dir, ".wiki.git") {
			database.PushUpdates(ctx, logger, h.authUser, h.ownerName, h.repoName, updates, func(branch string) {
				database.TriggerPushTasks(logger, h.authUser, h.repoID, branch)
			})
		} else if err := database.UpdateWikiHEAD(ctx, h.dir); err != nil {
			logger.Error("Failed to update wiki HEAD", "repoID", h.repoID, "error", err)
		}
	} else {
		err = git.RunService(ctx, git.Service(service), h.dir, opts)
	}
	if err != nil {
		logger.Error("Failed to serve RPC", "service", service, "error", err, "stderr", stderr.String())
		h.w.WriteHeader(http.StatusInternalServerError)
		return
	}
}

func serviceUploadPack(logger *logx.Logger, h serviceHandler) {
	serviceRPC(logger, h, "upload-pack")
}

func serviceReceivePack(logger *logx.Logger, h serviceHandler) {
	serviceRPC(logger, h, "receive-pack")
}

func getServiceType(r *http.Request) string {
	serviceType := r.FormValue("service")
	if !strings.HasPrefix(serviceType, "git-") {
		return ""
	}
	return strings.TrimPrefix(serviceType, "git-")
}

func packetWrite(str string) []byte {
	s := strconv.FormatInt(int64(len(str)+4), 16)
	if len(s)%4 != 0 {
		s = strings.Repeat("0", 4-len(s)%4) + s
	}
	return []byte(s + str)
}

func getInfoRefs(logger *logx.Logger, h serviceHandler) {
	h.setHeaderNoCache()
	var service git.Service
	switch getServiceType(h.r) {
	case "upload-pack":
		service = git.ServiceUploadPack
	case "receive-pack":
		service = git.ServiceReceivePack
	default:
		http.Error(h.w, "Missing or unsupported service", http.StatusBadRequest)
		return
	}

	var refs, stderr bytes.Buffer
	err := git.RunService(h.r.Context(), service, h.dir, git.ServiceOptions{
		StatelessRPC:  true,
		AdvertiseRefs: true,
		Stdout:        &refs,
		Stderr:        &stderr,
		Protocol:      h.r.Header.Get("Git-Protocol"),
	})
	if err != nil {
		logger.Error("Failed to advertise refs", "service", service, "error", err, "stderr", stderr.String())
	}
	h.w.Header().Set("Content-Type", fmt.Sprintf("application/x-git-%s-advertisement", service))
	h.w.WriteHeader(http.StatusOK)
	_, _ = h.w.Write(packetWrite("# service=git-" + string(service) + "\n"))
	_, _ = h.w.Write([]byte("0000"))
	_, _ = h.w.Write(refs.Bytes())
}

var gitHTTPRoutes = []struct {
	re      *lazyregexp.Regexp
	method  string
	handler func(*logx.Logger, serviceHandler)
}{
	{lazyregexp.New("/git-upload-pack$"), "POST", serviceUploadPack},
	{lazyregexp.New("/git-receive-pack$"), "POST", serviceReceivePack},
	{lazyregexp.New("/info/refs$"), "GET", getInfoRefs},
}

func serveGitHTTP(logger *logx.Logger, c *gitHTTPContext) {
	for _, route := range gitHTTPRoutes {
		if !route.re.MatchString(strings.ToLower(c.Request().URL.Path)) {
			continue
		}

		if conf.Repository.DisableHTTPGit {
			writeError(c.ResponseWriter(), http.StatusForbidden, "Interacting with repositories by HTTP protocol is disabled")
			return
		}

		if route.method != c.Request().Method {
			c.ResponseWriter().WriteHeader(http.StatusNotFound)
			return
		}

		// The wiki may not have been created yet.
		if !osx.IsDir(c.RepoPath) {
			c.ResponseWriter().WriteHeader(http.StatusNotFound)
			return
		}

		route.handler(logger, serviceHandler{
			w:   c.ResponseWriter(),
			r:   c.Request().Request,
			dir: c.RepoPath,

			authUser:  c.AuthUser,
			ownerName: c.OwnerName,
			ownerSalt: c.OwnerSalt,
			repoID:    c.RepoID,
			repoName:  c.RepoName,
		})
		return
	}

	c.ResponseWriter().WriteHeader(http.StatusNotFound)
}

// gitHTTPStore is the data layer that gitHTTPContexter authenticates requests
// and finds their repositories with.
type gitHTTPStore interface {
	context.AuthStore

	// GetRepositoryByName returns the repository with given owner and name. It
	// returns database.ErrRepoNotExist when not found.
	GetRepositoryByName(ctx stdctx.Context, ownerID int64, name string) (*database.Repository, error)
}

type gitHTTPDatabaseStore struct {
	context.Store
}

// newGitHTTPStore returns a new gitHTTPStore using the global database handle.
func newGitHTTPStore(logger *logx.Logger) gitHTTPStore {
	return gitHTTPDatabaseStore{Store: context.NewStore(logger)}
}

func (gitHTTPDatabaseStore) GetRepositoryByName(ctx stdctx.Context, ownerID int64, name string) (*database.Repository, error) {
	return database.Handle.Repositories().GetByName(ctx, ownerID, name)
}

func triggerTask(logger *logx.Logger, c flamego.Context) {
	branch := c.Query("branch")
	pusherID := c.QueryInt64("pusher")
	secret := c.Query("secret")
	if branch == "" || pusherID <= 0 || secret == "" {
		writeError(c.ResponseWriter(), http.StatusBadRequest, "Incomplete branch, pusher or secret")
		return
	}

	username := c.Param("owner")
	reponame := c.Param("repo")

	owner, err := database.Handle.Users().GetByUsername(c.Request().Context(), username)
	if err != nil {
		if database.IsErrUserNotExist(err) {
			writeError(c.ResponseWriter(), http.StatusBadRequest, "Owner does not exist")
		} else {
			c.ResponseWriter().WriteHeader(http.StatusInternalServerError)
			logger.Error("Failed to get user", "userName", username, "error", err)
		}
		return
	}

	// 🚨 SECURITY: No need to check existence of the repository if the client
	// can't even get the valid secret. Most likely not a legitimate request.
	if secret != cryptox.MD5(owner.Salt) {
		writeError(c.ResponseWriter(), http.StatusBadRequest, "Invalid secret")
		return
	}

	repo, err := database.Handle.Repositories().GetByName(c.Request().Context(), owner.ID, reponame)
	if err != nil {
		if database.IsErrRepoNotExist(err) {
			writeError(c.ResponseWriter(), http.StatusBadRequest, "Repository does not exist")
		} else {
			c.ResponseWriter().WriteHeader(http.StatusInternalServerError)
			logger.Error("Failed to get repository", "ownerID", owner.ID, "repoName", reponame, "error", err)
		}
		return
	}

	pusher, err := database.Handle.Users().GetByID(c.Request().Context(), pusherID)
	if err != nil {
		if database.IsErrUserNotExist(err) {
			writeError(c.ResponseWriter(), http.StatusBadRequest, "Pusher does not exist")
		} else {
			c.ResponseWriter().WriteHeader(http.StatusInternalServerError)
			logger.Error("Failed to get user", "userID", pusherID, "error", err)
		}
		return
	}

	logger.Debug("Task triggered", "ownerName", owner.Name, "repoName", repo.Name, "branch", branch, "pusherName", pusher.Name)

	database.TriggerPushTasks(logger, pusher, repo.ID, branch)
	c.ResponseWriter().WriteHeader(http.StatusAccepted)
}
