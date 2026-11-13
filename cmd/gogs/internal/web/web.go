package web

import (
	gocontext "context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/http/fcgi"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/flamego/cache"
	"github.com/flamego/captcha"
	"github.com/flamego/flamego"
	"github.com/go-macaron/gzip"
	"github.com/go-macaron/i18n"
	"github.com/go-macaron/session"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"gopkg.in/macaron.v1"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/cmd/gogs/internal/tracing"
	embedconf "gogs.io/gogs/conf"
	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/cron"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/email"
	"gogs.io/gogs/internal/git"
	"gogs.io/gogs/internal/markup"
	"gogs.io/gogs/internal/osx"
	apiv1 "gogs.io/gogs/internal/route/api/v1"
	"gogs.io/gogs/internal/route/lfs"
	"gogs.io/gogs/internal/route/repo"
	"gogs.io/gogs/internal/ssh"
	"gogs.io/gogs/internal/template/highlight"
	"gogs.io/gogs/internal/urlx"
	"gogs.io/gogs/public"
	"gogs.io/gogs/templates"
)

// Run starts the web server with the given configuration path and port
// override, and stops it when the context is done.
func Run(ctx gocontext.Context, configPath string, portOverride int) error {
	err := initServices(configPath)
	if err != nil {
		return errors.Wrap(err, "initialize application")
	}
	shutdownTracing, err := tracing.Init(ctx)
	if err != nil {
		return errors.Wrap(err, "initialize tracing")
	}
	if shutdownTracing != nil {
		// Stopping Gogs must not wait long on a collector that is down.
		defer shutdownTracing(5 * time.Second)
	}

	m, err := newMacaron()
	if err != nil {
		return errors.Wrap(err, "initialize macaron")
	}

	webHandler, err := newRoutingHandler()
	if err != nil {
		return errors.Wrap(err, "initialize web handler")
	}
	apiV1Handler := newAPIV1Handler()

	ignSignIn := context.Toggle(&context.ToggleOptions{SignInRequired: conf.Auth.RequireSigninView})

	m.SetAutoHead(true)

	m.Group("", func() {
		// TODO: Without session
		m.Any("/api/v1/*", ignSignIn, apiV1Bridger(apiV1Handler))
		m.Any("/", flamegoBridger(webHandler))
		m.Any("/*", flamegoBridger(webHandler))
	},
		session.Sessioner(session.Options{
			Provider:       conf.Session.Provider,
			ProviderConfig: conf.Session.ProviderConfig,
			CookieName:     conf.Session.CookieName,
			CookiePath:     conf.Server.Subpath,
			Gclifetime:     conf.Session.GCInterval,
			Maxlifetime:    conf.Session.MaxLifeTime,
			Secure:         conf.Session.CookieSecure,
			CookieLifeTime: 86400 * conf.Security.LoginRememberDays,
		}),
		context.Contexter(context.NewStore(), webHandler),
	)

	// Git over HTTP and LFS authenticate requests on their own, without sessions.
	m.Group("/:username/:reponame", func() {
		m.Get("/tasks/trigger", webHandler.ServeHTTP)
		m.Any("/info/lfs/*", webHandler.ServeHTTP)
		m.Route("/info/refs", "GET,OPTIONS", webHandler.ServeHTTP)
		m.Route("/git-upload-pack", "POST,OPTIONS", webHandler.ServeHTTP)
		m.Route("/git-receive-pack", "POST,OPTIONS", webHandler.ServeHTTP)
	})

	// Flag for port number in case first time run conflict.
	if portOverride > 0 {
		port := strconv.Itoa(portOverride)
		conf.Server.URL.Host = strings.Replace(conf.Server.URL.Host, ":"+conf.Server.URL.Port(), ":"+port, 1)
		conf.Server.ExternalURL = conf.Server.URL.String()
		conf.Server.HTTPPort = portOverride
	}

	var listenAddr string
	if conf.Server.Protocol == "unix" {
		listenAddr = conf.Server.HTTPAddr
	} else {
		listenAddr = fmt.Sprintf("%s:%d", conf.Server.HTTPAddr, conf.Server.HTTPPort)
	}
	log.Info("Available on %s", conf.Server.ExternalURL)

	var handler http.Handler = m
	if shutdownTracing != nil {
		handler = newTracingHandler(m)
	}

	server := &http.Server{Addr: listenAddr, Handler: handler}
	serve := server.ListenAndServe
	stop := server.Close
	switch conf.Server.Protocol {
	case "http":

	case "https":
		tlsMinVersion := tls.VersionTLS12
		switch conf.Server.TLSMinVersion {
		case "TLS13":
			tlsMinVersion = tls.VersionTLS13
		case "TLS12":
			tlsMinVersion = tls.VersionTLS12
		case "TLS11":
			tlsMinVersion = tls.VersionTLS11
		case "TLS10":
			tlsMinVersion = tls.VersionTLS10
		}
		server.TLSConfig = &tls.Config{
			MinVersion:               uint16(tlsMinVersion),
			CurvePreferences:         []tls.CurveID{tls.X25519, tls.CurveP256, tls.CurveP384, tls.CurveP521},
			PreferServerCipherSuites: true,
			CipherSuites: []uint16{
				tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
				tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
				tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
				tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
				tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
				tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
			},
		}
		serve = func() error {
			return server.ListenAndServeTLS(conf.Server.CertFile, conf.Server.KeyFile)
		}

	case "fcgi":
		// The web server is started by a FastCGI server, which passes the listener
		// as standard input.
		listener, err := net.FileListener(os.Stdin)
		if err != nil {
			return errors.Wrap(err, "listen on standard input")
		}
		serve = func() error { return fcgi.Serve(listener, handler) }
		stop = listener.Close

	case "unix":
		if osx.Exist(listenAddr) {
			err = os.Remove(listenAddr)
			if err != nil {
				return errors.Wrap(err, "remove existing Unix domain socket")
			}
		}

		var listener *net.UnixListener
		listener, err = net.ListenUnix("unix", &net.UnixAddr{Name: listenAddr, Net: "unix"})
		if err != nil {
			return errors.Wrap(err, "listen on Unix network")
		}

		if err = os.Chmod(listenAddr, conf.Server.UnixSocketMode); err != nil {
			return errors.Wrap(err, "change permission of Unix domain socket")
		}
		serve = func() error { return server.Serve(listener) }

	default:
		return errors.Newf("unexpected server protocol: %s", conf.Server.Protocol)
	}

	served := make(chan error, 1)
	go func() {
		served <- serve()
	}()
	select {
	case err = <-served:
		return errors.Wrap(err, "start server")
	case <-ctx.Done():
		if err = stop(); err != nil {
			return errors.Wrap(err, "stop server")
		}
		return nil
	}
}

// newTracingHandler returns a handler that records a span for every request, so
// that the spans of what the request runs, e.g., Git operations, are grouped
// under it.
func newTracingHandler(h http.Handler) http.Handler {
	return otelhttp.NewHandler(h, "", otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
		if r.Pattern != "" {
			return r.Pattern
		}
		return r.Method + " " + r.URL.Path
	}))
}

func newRoutingHandler() (http.Handler, error) {
	f := flamego.New()
	f.AutoHead(true)
	// Like Macaron did, a trailing slash of a page path is ignored, e.g.,
	// "/alice/widgets/" is the page of "/alice/widgets".
	f.Before(func(_ http.ResponseWriter, r *http.Request) bool {
		if p := r.URL.Path; len(p) > 1 && strings.HasSuffix(p, "/") && !strings.HasPrefix(p, "/api/") {
			r.URL.Path = strings.TrimRight(p, "/")
			r.URL.RawPath = ""
		}
		return false
	})
	f.Use(flamego.Recovery(flamego.RecoveryOptions{PlainText: true}))
	f.Use(flamegoInjector)
	f.Use(captcha.Captchaer(captcha.Options{URLPrefix: "/captcha/"}))

	cacherOpts, err := parseCacheOptions(conf.Cache)
	if err != nil {
		return nil, errors.Wrap(err, "parse cache options")
	}
	f.Use(cache.Cacher(cacherOpts))

	f.ReturnHandler(func(c flamego.Context, statusCode int, resp any, err error) {
		w := c.ResponseWriter()
		w.Header().Set("Cache-Control", "no-store")
		if err != nil {
			msg := err.Error()
			if statusCode >= http.StatusInternalServerError && conf.IsProdMode() {
				msg = "Internal server error"
			}
			resp = map[string]any{"error": msg}
		}
		if resp == nil {
			w.WriteHeader(statusCode)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(statusCode)
		_ = json.NewEncoder(w).Encode(resp)
	})

	f.Get("/redirect", getRedirect)
	f.Get("/robots.txt", getRobotsTxt)

	// The captcha middleware writes the response. This route exists so the request reaches it.
	f.Get("/captcha/image.jpeg", func() {})

	f.Group("/{owner}/{repo}", func() {
		f.Get("/commit/{sha: /[0-9a-f]{7,64}/}.{format: /(diff|patch)/}", getRepoCommitRaw)
		f.Get("/compare/"+compareRangeRoute+".diff", getRepoCompareRaw)
		f.Get("/pulls/{index: /[0-9]+/}.diff", withRepoPulls, withRepoPull, getRepoPullRaw)
		f.Get("/raw/{ref}/{filepath: **}", getRepoRawFile)
	}, requireViewAccess, withRepoContext)

	mountWebLocaleRoutes(f)
	mountWebAPIRoutes(f)
	webApp, err := mountWebAppRoutes(f)
	if err != nil {
		return nil, errors.Wrap(err, "mount web app routes")
	}
	mountPageRoutes(f, webApp)
	mountGitHTTPRoutes(f)
	return f, nil
}

// mountGitHTTPRoutes mounts the routes of Git over HTTP, Git LFS, and the task
// trigger of Git hooks.
func mountGitHTTPRoutes(f *flamego.Flame) {
	f.Group("/{owner}/{repo}", func() {
		f.Get("/tasks/trigger", repo.TriggerTask)

		f.Group("/info/lfs", func() {
			lfs.RegisterRoutes(f)
		})

		gitHTTP := []flamego.Handler{serveGoGet, repo.HTTPContexter(repo.NewStore()), repo.HTTP}
		f.Routes("/info/refs", "GET,OPTIONS", gitHTTP...)
		f.Routes("/git-upload-pack", "POST,OPTIONS", gitHTTP...)
		f.Routes("/git-receive-pack", "POST,OPTIONS", gitHTTP...)
	})
}

// newAPIV1Handler returns the handler of API v1 routes. It is separate from the
// web handler so that requests the web handler receives, e.g., through
// Context.ServeWeb, never reach API v1 routes without the sign-in and
// authentication checks done by Macaron.
func newAPIV1Handler() http.Handler {
	f := flamego.New()
	f.Use(flamego.Recovery(flamego.RecoveryOptions{PlainText: true}))
	f.Use(flamegoInjector)
	apiv1.RegisterRoutes(f)
	return f
}

func getRedirect(c flamego.Context) {
	to := c.Request().URL.Query().Get("to")
	if !urlx.IsSameSite(to) {
		to = conf.Server.Subpath + "/"
	}
	c.Redirect(to, http.StatusSeeOther)
}

func getRobotsTxt(w http.ResponseWriter, r *http.Request) {
	if !conf.HasRobotsTxt {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	http.ServeFile(w, r, filepath.Join(conf.CustomDir(), "robots.txt"))
}

// newMacaron initializes Macaron instance.
func newMacaron() (*macaron.Macaron, error) {
	m := macaron.New()
	if !conf.Server.DisableRouterLog {
		m.Use(macaron.Logger())
	}
	m.Use(macaron.Recovery())
	if conf.Server.EnableGzip {
		m.Use(gzip.Gziper())
	}
	if conf.Server.Protocol == "fcgi" {
		m.SetURLPrefix(conf.Server.Subpath)
	}

	// Register custom middleware first to make it possible to override files under "public".
	m.Use(macaron.Static(
		filepath.Join(conf.CustomDir(), "public"),
		macaron.StaticOptions{
			SkipLogging: conf.Server.DisableRouterLog,
		},
	))
	var publicFs http.FileSystem
	if !conf.Server.LoadAssetsFromDisk {
		publicFs = http.FS(public.Files)
	}
	m.Use(macaron.Static(
		filepath.Join(conf.WorkDir(), "public"),
		macaron.StaticOptions{
			ETag:        true,
			SkipLogging: conf.Server.DisableRouterLog,
			FileSystem:  publicFs,
		},
	))

	m.Use(macaron.Static(
		conf.Picture.AvatarUploadPath,
		macaron.StaticOptions{
			ETag:        true,
			Prefix:      conf.UsersAvatarPathPrefix,
			SkipLogging: conf.Server.DisableRouterLog,
		},
	))
	m.Use(macaron.Static(
		conf.Picture.RepositoryAvatarUploadPath,
		macaron.StaticOptions{
			ETag:        true,
			Prefix:      database.RepoAvatarURLPrefix,
			SkipLogging: conf.Server.DisableRouterLog,
		},
	))

	// The sign-in checks, the go-get page and the public SSH keys still write
	// JSON and plain text through Macaron's renderer, which no page template is
	// rendered with anymore.
	m.Use(macaron.Renderer(macaron.RenderOptions{TemplateFileSystem: noTemplates{}}))

	localeNames, err := embedconf.FileNames("locale")
	if err != nil {
		return nil, errors.Wrap(err, "list locale files")
	}
	localeFiles := make(map[string][]byte)
	for _, name := range localeNames {
		localeFiles[name], err = embedconf.Files.ReadFile("locale/" + name)
		if err != nil {
			return nil, errors.Wrapf(err, "read locale file %q", name)
		}
	}
	customLocaleDir := filepath.Join(conf.CustomDir(), "conf", "locale")
	m.Use(i18n.I18n(i18n.Options{
		SubURL:          conf.Server.Subpath,
		Files:           localeFiles,
		CustomDirectory: customLocaleDir,
		Langs:           conf.I18n.Langs,
		Names:           conf.I18n.Names,
		DefaultLang:     "en-US",
		Redirect:        true,
	}))
	webLocales, err = loadWebLocales(customLocaleDir, conf.I18n.Langs)
	if err != nil {
		return nil, errors.Wrap(err, "load web locales")
	}
	m.Route("/healthcheck", http.MethodHead+","+http.MethodGet, healthCheck)
	return m, nil
}

// noTemplates is a template file system without templates.
type noTemplates struct{}

func (noTemplates) ListFiles() []macaron.TemplateFile { return nil }

func (noTemplates) Get(name string) (io.Reader, error) {
	return nil, errors.Newf("template %q does not exist", name)
}

// injectContent holds the custom content spliced into the React shell.
type injectContent struct {
	head   string
	footer string
}

// readInjectContent reads the head and footer injection templates from the
// custom directory, falling back to the empty embedded defaults.
func readInjectContent() (injectContent, error) {
	customDir := filepath.Join(conf.CustomDir(), "templates")
	head, err := templates.ReadInjectFile(customDir, "head.tmpl")
	if err != nil {
		return injectContent{}, errors.Wrap(err, "read head injection")
	}
	footer, err := templates.ReadInjectFile(customDir, "footer.tmpl")
	if err != nil {
		return injectContent{}, errors.Wrap(err, "read footer injection")
	}
	return injectContent{head: string(head), footer: string(footer)}, nil
}

// renderIndex returns the index.html shell with per-request substitutions
// applied for the given WebContext. The inject argument supplies the custom
// head and footer content to splice into the shell.
func renderIndex(index []byte, wc context.WebContext, inject injectContent) ([]byte, error) {
	// json.Marshal escapes <, >, and &, so the payload cannot break out of the surrounding <script>.
	payload, err := json.Marshal(struct {
		Lang      string `json:"lang"`
		SubURL    string `json:"subURL"`
		LocaleURL string `json:"localeURL"`
		Page      string `json:"page,omitempty"`
		Error     string `json:"error,omitempty"`
		Version   string `json:"version,omitempty"`
	}{
		Lang:      wc.Lang,
		SubURL:    wc.SubURL,
		LocaleURL: wc.SubURL + webLocaleURL(wc.Lang),
		Page:      wc.Page,
		Error:     wc.Error,
		Version:   wc.Version,
	})
	if err != nil {
		return nil, errors.Wrap(err, "marshal web context")
	}
	// Start loading the translations alongside the bundle that needs them.
	script := `<link rel="preload" as="fetch" crossorigin href="` + html.EscapeString(wc.SubURL+webLocaleURL(wc.Lang)) + `">` +
		`<script>window.__webContext=` + string(payload) +
		`;document.documentElement.lang=window.__webContext.lang;</script>`

	pairs := []string{
		// Append the head injection after the web context script so custom
		// tags land inside <head>. The footer injection replaces the closing
		// body tag to land just before </body>.
		"{{.WebContext}}", script + inject.head,
		"</body>", inject.footer + "</body>",
		// Make entrypoint paths absolute, so pages nested below the root (e.g.,
		// "/user/sign-in") load them from the assets directory rather than one
		// relative to the page. Other bundled assets stay relative to the
		// entrypoint that references them.
		`src="./assets/`, `src="` + wc.SubURL + `/assets/`,
		`href="./assets/`, `href="` + wc.SubURL + `/assets/`,
	}
	if wc.SubURL != "" {
		// Prefix Vite's absolute root paths with the subpath for non-root mounts.
		pairs = append(pairs,
			`src="/src/`, `src="`+wc.SubURL+`/src/`,
			`href="/img/`, `href="`+wc.SubURL+`/img/`,
		)
	}
	return []byte(strings.NewReplacer(pairs...).Replace(string(index))), nil
}

func healthCheck(w http.ResponseWriter, r *http.Request) {
	if err := database.Ping(); err != nil {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = fmt.Fprintf(w, "* Database connection: %s\n", err)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return
	}
	_, _ = w.Write([]byte("* Database connection: OK\n"))
}

func initServices(customConf string) error {
	err := conf.Init(customConf)
	if err != nil {
		return errors.Wrap(err, "init configuration")
	}

	conf.InitLogging(false)
	log.Info("%s %s", conf.App.BrandName, conf.App.Version)
	log.Trace("Work directory: %s", conf.WorkDir())
	log.Trace("Custom path: %s", conf.CustomDir())
	log.Trace("Custom config: %s", conf.CustomConf)
	log.Trace("Log path: %s", conf.Log.RootPath)
	log.Trace("Build time: %s", conf.BuildTime)
	log.Trace("Build commit: %s", conf.BuildCommit)

	if conf.IsProdMode() {
		macaron.Env = macaron.PROD
		flamego.SetEnv(flamego.EnvTypeProd)
		macaron.ColorLog = false
		git.SetOutput(nil)
	} else {
		git.SetOutput(os.Stdout)
	}
	log.Info("Run mode: %s", strings.Title(macaron.Env))

	if conf.Email.Enabled {
		log.Trace("Email service is enabled")
	}

	email.NewContext()

	highlight.NewContext()
	markup.NewSanitizer()
	if err := database.NewEngine(); err != nil {
		return errors.Wrap(err, "initialize ORM engine")
	}

	database.LoadRepoConfig()
	database.NewRepoContext()

	cron.NewContext()
	database.InitSyncMirrors()
	database.InitDeliverHooks()
	database.InitTestPullRequests()

	if conf.HasMinWinSvc {
		log.Info("Builtin Windows Service is supported")
	}
	if conf.Server.LoadAssetsFromDisk {
		log.Trace("Assets are loaded from disk")
	}

	if conf.SSH.StartBuiltinServer {
		ssh.Listen(conf.SSH, conf.Server.AppDataPath)
		log.Info("SSH server started on %s:%v", conf.SSH.ListenHost, conf.SSH.ListenPort)
		log.Trace("SSH server cipher list: %v", conf.SSH.ServerCiphers)
		log.Trace("SSH server MAC list: %v", conf.SSH.ServerMACs)
		log.Trace("SSH server algorithms: %v", conf.SSH.ServerAlgorithms)
	}

	if conf.SSH.RewriteAuthorizedKeysAtStart {
		if err := database.RewriteAuthorizedKeys(); err != nil {
			log.Warn("Failed to rewrite authorized_keys file: %v", err)
		}
	}

	return nil
}
