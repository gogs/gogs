package repo

import (
	"bytes"
	"compress/gzip"
	gocontext "context"
	"fmt"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/flamego/flamego"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/auth"
	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/git"
	"gogs.io/gogs/internal/lazyregexp"
	"gogs.io/gogs/internal/pathx"
	"gogs.io/gogs/internal/tool"
)

type HTTPContext struct {
	flamego.Context
	OwnerName string
	OwnerSalt string
	RepoID    int64
	RepoName  string
	AuthUser  *database.User
}

// writeError writes the status and the text of an error response.
func writeError(w http.ResponseWriter, status int, text string) {
	w.WriteHeader(status)
	if text != "" {
		_, _ = w.Write([]byte(text))
	}
}

// askCredentials responses HTTP header and status which informs client to provide credentials.
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

func HTTPContexter(store Store) flamego.Handler {
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
		repoName = strings.TrimSuffix(repoName, ".wiki")

		action := gitHTTPAction(c)
		isPull := gitHTTPIsPull(c, action)

		owner, err := store.GetUserByUsername(c.Request().Context(), ownerName)
		if err != nil {
			if database.IsErrUserNotExist(err) {
				c.ResponseWriter().WriteHeader(http.StatusNotFound)
			} else {
				c.ResponseWriter().WriteHeader(http.StatusInternalServerError)
				log.Error("Failed to get user [name: %s]: %v", ownerName, err)
			}
			return
		}

		repo, err := store.GetRepositoryByName(c.Request().Context(), owner.ID, repoName)
		if err != nil {
			if database.IsErrRepoNotExist(err) {
				c.ResponseWriter().WriteHeader(http.StatusNotFound)
			} else {
				c.ResponseWriter().WriteHeader(http.StatusInternalServerError)
				log.Error("Failed to get repository [owner_id: %d, name: %s]: %v", owner.ID, repoName, err)
			}
			return
		}

		// Authentication is not required for pulling from public repositories.
		if isPull && !repo.IsPrivate && !conf.Auth.RequireSigninView {
			c.Map(&HTTPContext{
				Context: c,
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

		// Handle HTTP Basic Authentication
		authHead := c.Request().Header.Get("Authorization")
		if authHead == "" {
			askCredentials(c, http.StatusUnauthorized, "")
			return
		}

		auths := strings.Fields(authHead)
		if len(auths) != 2 || auths[0] != "Basic" {
			askCredentials(c, http.StatusUnauthorized, "")
			return
		}
		authUsername, authPassword, err := tool.BasicAuthDecode(auths[1])
		if err != nil {
			askCredentials(c, http.StatusUnauthorized, "")
			return
		}

		authUser, err := store.AuthenticateUser(c.Request().Context(), authUsername, authPassword, -1)
		if err != nil && !auth.IsErrBadCredentials(err) {
			c.ResponseWriter().WriteHeader(http.StatusInternalServerError)
			log.Error("Failed to authenticate user [name: %s]: %v", authUsername, err)
			return
		}

		// If username and password combination failed, try again using either username
		// or password as the token.
		if authUser == nil {
			authUser, err = context.AuthenticateByToken(store, c.Request().Context(), authUsername)
			if err != nil && !database.IsErrAccessTokenNotExist(err) {
				c.ResponseWriter().WriteHeader(http.StatusInternalServerError)
				log.Error("Failed to authenticate by access token via username: %v", err)
				return
			} else if database.IsErrAccessTokenNotExist(err) {
				// Try again using the password field as the token.
				authUser, err = context.AuthenticateByToken(store, c.Request().Context(), authPassword)
				if err != nil {
					if database.IsErrAccessTokenNotExist(err) {
						askCredentials(c, http.StatusUnauthorized, "")
					} else {
						c.ResponseWriter().WriteHeader(http.StatusInternalServerError)
						log.Error("Failed to authenticate by access token via password: %v", err)
					}
					return
				}
			}
		} else if store.IsTwoFactorEnabled(c.Request().Context(), authUser.ID) {
			askCredentials(c, http.StatusUnauthorized, `User with two-factor authentication enabled cannot perform HTTP/HTTPS operations via plain username and password
Please create and use personal access token on user settings page`)
			return
		}

		if authUser.IsDisabled() {
			askCredentials(c, http.StatusForbidden, "User account is disabled")
			return
		}

		log.Trace("[Git] Authenticated user: %s", authUser.Name)

		mode := database.AccessModeWrite
		if isPull {
			mode = database.AccessModeRead
		}
		if !database.Handle.Permissions().Authorize(c.Request().Context(), authUser.ID, repo.ID, mode,
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

		c.Map(&HTTPContext{
			Context:   c,
			OwnerName: ownerName,
			OwnerSalt: owner.Salt,
			RepoID:    repo.ID,
			RepoName:  repoName,
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

func serviceRPC(h serviceHandler, service string) {
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
			log.Error("HTTP.Get: fail to create gzip reader: %v", err)
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
	// running hooks after refs were updated.
	ctx := h.r.Context()
	if service == "receive-pack" {
		ctx = gocontext.WithoutCancel(ctx)
	}
	var stderr bytes.Buffer
	err = git.RunService(ctx, git.Service(service), h.dir, git.ServiceOptions{
		StatelessRPC: true,
		Stdin:        reqBody,
		Stdout:       h.w,
		Stderr:       &stderr,
		Protocol:     h.r.Header.Get("Git-Protocol"),
		Envs:         envs,
	})
	if err != nil {
		log.Error("HTTP.serviceRPC: fail to serve RPC '%s': %v - %s", service, err, stderr.String())
		h.w.WriteHeader(http.StatusInternalServerError)
		return
	}
}

func serviceUploadPack(h serviceHandler) {
	serviceRPC(h, "upload-pack")
}

func serviceReceivePack(h serviceHandler) {
	serviceRPC(h, "receive-pack")
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

func getInfoRefs(h serviceHandler) {
	h.setHeaderNoCache()
	service := getServiceType(h.r)
	if service != "upload-pack" && service != "receive-pack" {
		http.Error(h.w, "Missing or unsupported service", http.StatusBadRequest)
		return
	}

	var refs, stderr bytes.Buffer
	err := git.RunService(h.r.Context(), git.Service(service), h.dir, git.ServiceOptions{
		StatelessRPC:  true,
		AdvertiseRefs: true,
		Stdout:        &refs,
		Stderr:        &stderr,
		Protocol:      h.r.Header.Get("Git-Protocol"),
	})
	if err != nil {
		log.Error("HTTP.getInfoRefs: fail to advertise refs for '%s': %v - %s", service, err, stderr.String())
	}
	h.w.Header().Set("Content-Type", fmt.Sprintf("application/x-git-%s-advertisement", service))
	h.w.WriteHeader(http.StatusOK)
	_, _ = h.w.Write(packetWrite("# service=git-" + service + "\n"))
	_, _ = h.w.Write([]byte("0000"))
	_, _ = h.w.Write(refs.Bytes())
}

var routes = []struct {
	re      *lazyregexp.Regexp
	method  string
	handler func(serviceHandler)
}{
	{lazyregexp.New("(.*?)/git-upload-pack$"), "POST", serviceUploadPack},
	{lazyregexp.New("(.*?)/git-receive-pack$"), "POST", serviceReceivePack},
	{lazyregexp.New("(.*?)/info/refs$"), "GET", getInfoRefs},
}

func getGitRepoPath(dir string) (string, error) {
	if !strings.HasSuffix(dir, ".git") {
		dir += ".git"
	}

	filename := filepath.Join(conf.Repository.Root, dir)
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		return "", err
	}

	return filename, nil
}

func HTTP(c *HTTPContext) {
	for _, route := range routes {
		reqPath := strings.ToLower(c.Request().URL.Path)
		m := route.re.FindStringSubmatch(reqPath)
		if m == nil {
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

		// 🚨 SECURITY: Prevent path traversal.
		cleaned := pathx.Clean(m[1])
		if m[1] != "/"+cleaned {
			writeError(c.ResponseWriter(), http.StatusBadRequest, "Request path contains suspicious characters")
			return
		}

		dir, err := getGitRepoPath(cleaned)
		if err != nil {
			log.Warn("HTTP.getGitRepoPath: %v", err)
			c.ResponseWriter().WriteHeader(http.StatusNotFound)
			return
		}

		route.handler(serviceHandler{
			w:   c.ResponseWriter(),
			r:   c.Request().Request,
			dir: dir,

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
