package web

import (
	gocontext "context"
	"crypto/hkdf"
	"crypto/sha256"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"html"
	"net"
	"net/http"
	"net/http/fcgi"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/flamego/altcha"
	"github.com/flamego/cache"
	"github.com/flamego/captcha"
	"github.com/flamego/flamego"
	"github.com/flamego/session"
	"github.com/klauspost/compress/gzhttp"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	"unknwon.dev/x/logx"

	apiv1 "gogs.io/gogs/cmd/gogs/internal/api/v1"
	"gogs.io/gogs/cmd/gogs/internal/lfs"
	"gogs.io/gogs/cmd/gogs/internal/options"
	"gogs.io/gogs/cmd/gogs/internal/tracing"
	embedconf "gogs.io/gogs/conf"
	"gogs.io/gogs/internal/app"
	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/cron"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/email"
	"gogs.io/gogs/internal/git"
	"gogs.io/gogs/internal/i18n"
	"gogs.io/gogs/internal/lfsx"
	"gogs.io/gogs/internal/markup"
	"gogs.io/gogs/internal/osx"
	"gogs.io/gogs/internal/ssh"
	"gogs.io/gogs/internal/telemetry"
	"gogs.io/gogs/internal/urlx"
	"gogs.io/gogs/public"
	"gogs.io/gogs/templates"
)

// Run starts the web server of the application version with the given
// configuration path and port override, and stops it when the context is
// done. Once logging is configured, it reports a failure through the
// configured logger and exits.
func Run(ctx gocontext.Context, logger *logx.Logger, appVersion, configPath string, portOverride int) (err error) {
	cfg, gitVer, configured, err := initServices(logger, appVersion, configPath, portOverride)
	if configured == nil {
		return errors.Wrap(err, "initialize application")
	}
	logger = configured
	// The caller only has the console logger, which skips the file and webhook
	// sinks.
	defer func() {
		if err != nil {
			logger.FatalContext(ctx, "Failed to start application", "error", err)
		}
	}()
	if err != nil {
		return errors.Wrap(err, "initialize application")
	}
	var traceStore *telemetry.Store
	var spanProcessors []sdktrace.SpanProcessor
	if cfg.Tracing.EnableAdminCollector {
		traceStore = telemetry.NewStore(telemetry.Options{
			MaxMemory:                cfg.Tracing.MaxMemoryMiB << 20,
			MaxAge:                   cfg.Tracing.MaxAge,
			SlowThreshold:            cfg.Tracing.SlowThreshold,
			SamplesPerRoutePerMinute: cfg.Tracing.SamplesPerRoutePerMinute,
		})
		spanProcessors = append(spanProcessors, traceStore)
	}
	shutdownTracing, err := tracing.Init(ctx, logger, cfg.Tracing.Enabled, spanProcessors...)
	if err != nil {
		return errors.Wrap(err, "initialize tracing")
	}
	if shutdownTracing != nil {
		// Stopping Gogs must not wait long on a collector that is down.
		defer shutdownTracing(5 * time.Second)
	}
	health := newHealth(cfg, traceStore)
	go health.Run(ctx)

	sessionOpts, err := parseSessionOptions(logger, cfg.Session, cfg.Server.Subpath, cfg.Security.LoginRememberDays)
	if err != nil {
		return errors.Wrap(err, "parse session options")
	}
	localizer, err := newLocalizer(cfg.Server.Subpath, cfg.I18n)
	if err != nil {
		return errors.Wrap(err, "initialize localizer")
	}
	f, err := newRoutingHandler(logger, cfg, gitVer, localizer, session.Sessioner(sessionOpts), authenticator(context.NewStore(logger)), traceStore, health)
	if err != nil {
		return errors.Wrap(err, "initialize web handler")
	}

	var listenAddr string
	if cfg.Server.Protocol == "unix" {
		listenAddr = cfg.Server.HTTPAddr
	} else {
		listenAddr = fmt.Sprintf("%s:%d", cfg.Server.HTTPAddr, cfg.Server.HTTPPort)
	}
	logger.Info("Server is available", "externalURL", cfg.Server.ExternalURL)

	var handler http.Handler = f
	if cfg.Server.EnableGzip {
		handler = gzhttp.GzipHandler(handler)
	}
	if shutdownTracing != nil {
		handler = newTracingHandler(handler)
	}

	server := &http.Server{Addr: listenAddr, Handler: handler}
	serve := server.ListenAndServe
	stop := server.Close
	switch cfg.Server.Protocol {
	case "http":

	case "https":
		tlsMinVersion := tls.VersionTLS12
		switch cfg.Server.TLSMinVersion {
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
			return server.ListenAndServeTLS(cfg.Server.CertFile, cfg.Server.KeyFile)
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

		if err = os.Chmod(listenAddr, cfg.Server.UnixSocketMode); err != nil {
			return errors.Wrap(err, "change permission of Unix domain socket")
		}
		serve = func() error { return server.Serve(listener) }

	default:
		return errors.Newf("unexpected server protocol: %s", cfg.Server.Protocol)
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

// newCrossOriginProtection returns the middleware that turns away the requests
// browsers send to change something from another site, e.g., a form on another
// site that posts to Gogs. Browsers mark where each request comes from with
// the Sec-Fetch-Site header, or the Origin header in older browsers. API v1 is
// exempt, because the site can allow other origins to call it
// (`[http] ACCESS_CONTROL_ALLOW_ORIGIN`).
func newCrossOriginProtection(externalURL *url.URL) (flamego.Handler, error) {
	p := http.NewCrossOriginProtection()
	// A reverse proxy may pass its own address as the Host header, which older
	// browsers without Sec-Fetch-Site are checked against.
	if externalURL.Host != "" {
		err := p.AddTrustedOrigin(externalURL.Scheme + "://" + externalURL.Host)
		if err != nil {
			return nil, errors.Wrap(err, "trust external URL")
		}
	}
	p.AddInsecureBypassPattern("/api/v1/")
	return func(w http.ResponseWriter, r *http.Request) {
		if err := p.Check(r); err != nil {
			http.Error(w, "Cross-origin request denied", http.StatusForbidden)
		}
	}, nil
}

// newHealth returns the measurements of the server that the admin panel shows,
// including the requests the store records when it is not nil.
func newHealth(cfg *conf.Config, traceStore *telemetry.Store) *telemetry.Health {
	count := func(key string, read func() int64) telemetry.Gauge {
		return telemetry.Gauge{Key: key, Read: func() (float64, bool) { return float64(read()), true }}
	}
	perMinute := func(key string, read func() int64) telemetry.Gauge {
		return telemetry.CounterGauge(key, time.Minute, func() float64 { return float64(read()) })
	}

	var lastWaits database.ConnectionStats
	gauges := []telemetry.Gauge{
		{Key: "dbConnections", Limit: float64(database.GetConnectionStats().Limit), Read: func() (float64, bool) {
			return float64(database.GetConnectionStats().InUse), true
		}},
		count("dbIdleConnections", func() int64 { return int64(database.GetConnectionStats().Idle) }),
		perMinute("dbWaits", func() int64 { return database.GetConnectionStats().WaitCount }),
		// How long a query that waited for a connection since the last sample
		// waited on average, in milliseconds.
		{Key: "dbWaitTime", Read: func() (float64, bool) {
			stats := database.GetConnectionStats()
			waits, waited := stats.WaitCount-lastWaits.WaitCount, stats.WaitDuration-lastWaits.WaitDuration
			lastWaits = stats
			if waits <= 0 {
				return 0, true
			}
			return float64(waited) / float64(waits) / float64(time.Millisecond), true
		}},
		count("gitProcesses", git.RunningCommands),
		{Key: "codeSearches", Limit: float64(cap(codeSearchSlots)), Read: func() (float64, bool) {
			return float64(len(codeSearchSlots)), true
		}},
		count("mirrorQueue", func() int64 { return int64(database.MirrorQueue.Len()) }),
		count("hookQueue", func() int64 { return int64(database.HookQueue.Len()) }),
		count("pullRequestQueue", func() int64 { return int64(database.PullRequestQueue.Len()) }),
		perMinute("failedHookDeliveries", database.FailedHookDeliveries),
		perMinute("notices", database.NoticesCreated),
	}
	if cfg.SSH.StartBuiltinServer {
		gauges = append(gauges, count("sshConnections", ssh.Connections))
	}
	if cfg.Email.Enabled {
		gauges = append(gauges, count("mailQueue", email.QueueLen))
	}
	disks := []telemetry.Disk{
		{Key: "diskRepositories", Path: cfg.Repository.Root},
		{Key: "diskData", Path: cfg.Server.AppDataPath},
	}
	if lfsx.Storage(cfg.LFS.Storage) == lfsx.StorageLocal {
		disks = append(disks, telemetry.Disk{Key: "diskLFS", Path: cfg.LFS.ObjectsPath})
	}
	gauges = append(gauges, telemetry.DiskGauges(disks...)...)
	return telemetry.NewHealth(traceStore, gauges...)
}

// overrideHTTPPort makes the server of the options listen on the given port
// instead of HTTP_PORT, e.g., when HTTP_PORT is taken on the first run.
//
// When the external URL points to HTTP_PORT, users reach Gogs directly, so the
// external URL moves to the given port too. When it points to another port,
// users reach Gogs through a reverse proxy or a port mapping, so the external
// URL stays as it is. A URL without a port points to the default port of its
// scheme, i.e., 80 for HTTP and 443 for HTTPS.
func overrideHTTPPort(server *conf.ServerOpts, port int) {
	externalPort := server.URL.Port()
	if externalPort == "" {
		externalPort = map[string]string{"http": "80", "https": "443"}[server.URL.Scheme]
	}
	reachedDirectly := externalPort == strconv.Itoa(server.HTTPPort)
	// Rewriting the URL for the same port would only add its default port to it.
	if reachedDirectly && port != server.HTTPPort {
		server.URL.Host = net.JoinHostPort(server.URL.Hostname(), strconv.Itoa(port))
		server.ExternalURL = server.URL.String()
	}
	server.HTTPPort = port
}

// newTracingHandler returns a handler that records a span for every request, so
// that the spans of what the request runs, e.g., Git operations, are grouped
// under it.
//
// Every request starts a trace of its own, with a link to the trace it carries,
// if any. A client that got to pick the trace of its requests would put any
// number of them into one trace, or into the trace of someone else.
func newTracingHandler(h http.Handler) http.Handler {
	// The route is only known once the request was routed, after the span
	// started, so it is added to the span then.
	routed := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.ServeHTTP(w, r)
		if _, route, ok := strings.Cut(r.Pattern, " "); ok {
			trace.SpanFromContext(r.Context()).SetAttributes(attribute.String("http.route", route))
		}
	})
	return otelhttp.NewHandler(routed, "",
		otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
			if r.Pattern != "" {
				return r.Pattern
			}
			return r.Method + " " + r.URL.Path
		}),
		otelhttp.WithPublicEndpointFn(func(*http.Request) bool { return true }),
	)
}

// newRoutingHandler returns the handler of all routes, which are given the
// values of the configuration that handlers declare. The localizer maps the
// i18n.Locale of each request. Except for the health check, metrics, Git over
// HTTP and LFS, which authenticate requests on their own, the routes have
// sessions started by sessioner, and authenticate maps the authenticated user
// (*database.User, nil for anonymous visitors) and the authMethod. The admin
// panel shows the traces of the store and the measurements of health, either of
// which may be nil.
func newRoutingHandler(logger *logx.Logger, cfg *conf.Config, gitVersion gitVersion, localizer, sessioner, authenticate flamego.Handler, traceStore *telemetry.Store, health *telemetry.Health) (*flamego.Flame, error) {
	f := flamego.New()
	f.Map(logger)
	f.Map(injected(cfg, gitVersion)...)
	f.Map(traceStore, health)
	f.AutoHead(true)
	if cfg.Server.Protocol == "fcgi" && cfg.Server.Subpath != "" {
		f.Before(func(_ http.ResponseWriter, r *http.Request) bool {
			r.URL.Path = strings.TrimPrefix(r.URL.Path, cfg.Server.Subpath)
			return false
		})
	}
	if !cfg.Server.DisableRouterLog {
		f.Use(flamego.Logger())
	}
	f.Use(flamego.Recovery(flamego.RecoveryOptions{PlainText: true}))
	f.Use(func(w http.ResponseWriter) {
		// 🚨 SECURITY: Prevent MIME type sniffing in some browsers,
		// see https://github.com/gogs/gogs/issues/5397 for details.
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "deny")
	})

	// Serve files under the custom directory first to make it possible to
	// override files under "public".
	f.Use(flamego.Static(flamego.StaticOptions{
		Directory:     filepath.Join(conf.CustomDir(), "public"),
		EnableLogging: !cfg.Server.DisableRouterLog,
	}))
	var publicFS http.FileSystem
	if !cfg.Server.LoadAssetsFromDisk {
		publicFS = http.FS(public.Files)
	}
	f.Use(flamego.Static(flamego.StaticOptions{
		Directory:     filepath.Join(conf.WorkDir(), "public"),
		FileSystem:    publicFS,
		SetETag:       true,
		EnableLogging: !cfg.Server.DisableRouterLog,
	}))

	f.Use(localizer)

	cacherOpts, err := parseCacheOptions(cfg.Cache)
	if err != nil {
		return nil, errors.Wrap(err, "parse cache options")
	}
	f.Use(cache.Cacher(cacherOpts))

	f.ReturnHandler(func(c flamego.Context, statusCode int, resp any, err error) {
		w := c.ResponseWriter()
		w.Header().Set("Cache-Control", "no-store")
		if err != nil {
			if statusCode >= http.StatusInternalServerError {
				telemetry.RecordFailure(c.Request().Context(), err)
			}
			msg := err.Error()
			if statusCode >= http.StatusInternalServerError && cfg.App.IsProdMode() {
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

	crossOrigin, err := newCrossOriginProtection(cfg.Server.URL)
	if err != nil {
		return nil, errors.Wrap(err, "initialize cross-origin protection")
	}

	antiBot, err := antiBotMiddlewares(cfg.Auth.AntiBotVerification, cfg.Security.SecretKey)
	if err != nil {
		return nil, errors.Wrap(err, "initialize anti-bot verification")
	}

	f.Get("/healthcheck", healthCheck)
	f.Get("/-/metrics", app.MetricsFilter(cfg.Prometheus), promhttp.Handler().ServeHTTP)
	mountGitHTTPRoutes(logger, f)

	var webApp http.Handler
	f.Group("", func() {
		f.Get("/redirect", getRedirect)
		f.Get("/robots.txt", getRobotsTxt(cfg.HasRobotsTxt))

		// The anti-bot middlewares write the responses. These routes exist so the
		// requests reach them.
		switch cfg.Auth.AntiBotVerification {
		case conf.AntiBotVerificationAltcha:
			f.Get(altchaURLPrefix+"challenge.json", func() {})
			f.Get(altchaURLPrefix+"worker.js", func() {})
		case conf.AntiBotVerificationCaptcha:
			f.Get(captchaURLPrefix+"image.png", func() {})
		}

		f.Group("/{owner}/{repo}", func() {
			f.Get("/commit/{sha: /[0-9a-f]{7,64}/}.{format: /(diff|patch)/}", getRepoCommitRaw)
			f.Get("/compare/"+compareRangeRoute+".diff", getRepoCompareRaw)
			f.Get("/pulls/{index: /[0-9]+/}.diff", withRepoPulls, withRepoPull, getRepoPullRaw)
			f.Get("/raw/{ref}/{filepath: **}", getRepoRawFile)
		}, requireViewAccess, withRepoContext)

		mountWebLocaleRoutes(f)
		mountWebAPIRoutes(f)
		webApp, err = mountWebAppRoutes(logger, f, options.Markup(cfg), indexOptions{
			landingURL:                 cfg.Server.LandingURL,
			externalURL:                cfg.Server.ExternalURL,
			requireAuthenticatedViewer: cfg.Auth.RequireSigninView,
		})
		if err != nil {
			return
		}

		// The web app renders every other path and asks the web API what the viewer
		// may see. These routes serve the files and redirects next to its pages.
		f.Map(webAppPages{Handler: webApp, isProdMode: cfg.App.IsProdMode(), version: cfg.App.Version})
		f.Get("/explore", func(c flamego.Context) {
			c.Redirect(cfg.Server.Subpath + "/explore/repos")
		})
		f.Get("/attachments/{uuid}", requireViewAccess, getAttachment)
		f.Get("/-/avatars/{id}", getUserAvatar)
		f.Get("/-/avatars/seed/{seed}", getGeneratedAvatar)
		f.Get("/-/repo-avatars/{id}", getRepoAvatar)
		f.Get("/{username}.keys", requireViewAccess, getUserPublicKeys)
		f.Any("/user/activate_email", activateEmail)
		f.Get("/user/email2user", email2User)
		f.Group("/{owner}/{repo}", func() {
			f.Get("", serveGoGet, webApp.ServeHTTP)
			f.Get("/archive/{**}", requireViewAccess, withRepoContext, getRepoArchive)
			f.Get("/issues", maybeRedirectToExternalTracker, webApp.ServeHTTP)
			f.Get("/issues/{index}", maybeRedirectToExternalTracker, webApp.ServeHTTP)
			f.Get("/wiki", maybeRedirectToExternalWiki, webApp.ServeHTTP)
			f.Get("/wiki/{page}",
				maybeRedirectToExternalWiki,
				func(c flamego.Context) {
					if isWikiPageName(c.Param("page")) {
						webApp.ServeHTTP(c.ResponseWriter(), c.Request().Request)
					}
				},
				requireViewAccess, withRepoContext, withRepoWiki, getRepoWikiFile, webApp.ServeHTTP,
			)
			f.Get("/wiki/{page}/_edit", maybeRedirectToExternalWiki, webApp.ServeHTTP)
		})
		f.Any("/api/v1/{**}", apiV1Access, apiV1Bridger(newAPIV1Handler(logger, cfg, gitVersion)))
	}, append([]flamego.Handler{crossOrigin, sessioner, authenticate, corsHeaders, withWebContext}, antiBot...)...)
	if err != nil {
		return nil, errors.Wrap(err, "mount web app routes")
	}
	return f, nil
}

// altchaURLPrefix is where the altcha middleware serves challenges and the
// script that solves them.
const altchaURLPrefix = "/-/altcha/"

// captchaURLPrefix is where the captcha middleware serves captcha images.
const captchaURLPrefix = "/-/captcha/"

// antiBotVerifier reports whether the payload of a sign-up, i.e., the captcha
// text or the altcha solution, passes anti-bot verification.
type antiBotVerifier func(payload string) bool

// antiBotMiddlewares returns the middlewares of [auth] ANTI_BOT_VERIFICATION
// that serve what the sign-up page needs and map an antiBotVerifier. Only the
// configured one, i.e., verification, is mounted, and none when it is empty.
// The altcha secret is derived from secretKey.
func antiBotMiddlewares(verification, secretKey string) ([]flamego.Handler, error) {
	switch verification {
	case "":
		return []flamego.Handler{func(c flamego.Context) {
			c.Map(antiBotVerifier(func(string) bool { return true }))
		}}, nil
	case conf.AntiBotVerificationAltcha:
		// The middleware requires a secret of at least 16 bytes, which the secret
		// key may be shorter than, so it gets a key derived from the secret key.
		secret, err := hkdf.Key(sha256.New, []byte(secretKey), nil, "altcha secret", 32)
		if err != nil {
			return nil, errors.Wrap(err, "derive altcha secret")
		}
		return []flamego.Handler{
			altcha.Challenger(altcha.Options{URLPrefix: altchaURLPrefix, Secret: secret}),
			func(c flamego.Context, challenge altcha.Challenge) {
				c.Map(antiBotVerifier(func(payload string) bool { return challenge.Verify(payload) == nil }))
			},
		}, nil
	case conf.AntiBotVerificationCaptcha:
		return []flamego.Handler{
			captcha.Captchaer(captcha.Options{URLPrefix: captchaURLPrefix}),
			func(c flamego.Context, cpt captcha.Captcha) {
				c.Map(antiBotVerifier(cpt.ValidText))
			},
		}, nil
	default:
		return nil, errors.Newf("unknown anti-bot verification %q", verification)
	}
}

// mountGitHTTPRoutes mounts the routes of Git over HTTP, Git LFS, and the task
// trigger of Git hooks.
func mountGitHTTPRoutes(logger *logx.Logger, f *flamego.Flame) {
	f.Group("/{owner}/{repo}", func() {
		f.Get("/tasks/trigger", triggerTask)

		f.Group("/info/lfs", func() {
			lfs.RegisterRoutes(logger.Scoped("lfs"), f)
		})

		gitHTTP := []flamego.Handler{serveGoGet, gitHTTPContexter(logger.Scoped("git"), newGitHTTPStore(logger)), serveGitHTTP}
		f.Routes("/info/refs", "GET,OPTIONS", gitHTTP...)
		f.Routes("/git-upload-pack", "POST,OPTIONS", gitHTTP...)
		f.Routes("/git-receive-pack", "POST,OPTIONS", gitHTTP...)
	})
}

// newAPIV1Handler returns the handler of API v1 routes. It is separate from the
// routing handler so that apiV1Bridger can normalize the request path before
// the routes are matched. Its handlers are given the same values of the
// configuration as the routing handler.
func newAPIV1Handler(logger *logx.Logger, cfg *conf.Config, gitVersion gitVersion) http.Handler {
	f := flamego.New()
	f.Map(logger)
	f.Map(injected(cfg, gitVersion)...)
	f.Use(flamego.Recovery(flamego.RecoveryOptions{PlainText: true}))
	f.Use(apiV1Injector)
	apiv1.RegisterRoutes(f)
	return f
}

func getRedirect(c flamego.Context, server conf.ServerOpts) {
	to := c.Request().URL.Query().Get("to")
	if !urlx.IsSameSite(to) {
		to = server.Subpath + "/"
	}
	c.Redirect(to, http.StatusSeeOther)
}

// getRobotsTxt returns the handler that serves the "robots.txt" of the custom
// directory, which is found when hasRobotsTxt is true.
func getRobotsTxt(hasRobotsTxt bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !hasRobotsTxt {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		http.ServeFile(w, r, filepath.Join(conf.CustomDir(), "robots.txt"))
	}
}

// newLocalizer returns the middleware that maps the i18n.Locale of each request
// to a site under subpath.
func newLocalizer(subpath string, i18nOpts conf.I18nOpts) (flamego.Handler, error) {
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
	webLocales, err = loadWebLocales(customLocaleDir, i18nOpts.Langs)
	if err != nil {
		return nil, errors.Wrap(err, "load web locales")
	}
	return i18n.Localizer(i18n.Options{
		SubURL:          subpath,
		Files:           localeFiles,
		CustomDirectory: customLocaleDir,
		Langs:           i18nOpts.Langs,
		Names:           i18nOpts.Names,
		DefaultLang:     "en-US",
	})
}

// injectContent holds the custom content spliced into the React shell.
type injectContent struct {
	head   string
	footer string
}

// readInjectContent reads the head and footer injection templates from the
// custom directory, falling back to the empty embedded defaults.
func readInjectContent(customDir string) (injectContent, error) {
	templatesDir := filepath.Join(customDir, "templates")
	head, err := templates.ReadInjectFile(templatesDir, "head.tmpl")
	if err != nil {
		return injectContent{}, errors.Wrap(err, "read head injection")
	}
	footer, err := templates.ReadInjectFile(templatesDir, "footer.tmpl")
	if err != nil {
		return injectContent{}, errors.Wrap(err, "read footer injection")
	}
	return injectContent{head: string(head), footer: string(footer)}, nil
}

// indexOptions are the settings of the site that the index.html shell passes
// to the web app.
type indexOptions struct {
	// landingURL is where the web app sends unauthenticated viewers who open the
	// home page.
	landingURL string
	// externalURL is the external URL of the server, ending with "/".
	externalURL string
	// requireAuthenticatedViewer sends unauthenticated viewers to sign in.
	requireAuthenticatedViewer bool
}

// renderIndex returns the index.html shell with per-request substitutions
// applied for the given WebContext. The inject argument supplies the custom
// head and footer content to splice into the shell.
func renderIndex(index []byte, wc context.WebContext, inject injectContent, opts indexOptions) ([]byte, error) {
	var landingURL string
	if opts.landingURL != "/" {
		landingURL = opts.landingURL
	}
	// json.Marshal escapes <, >, and &, so the payload cannot break out of the surrounding <script>.
	payload, err := json.Marshal(struct {
		Lang      string `json:"lang"`
		SubURL    string `json:"subURL"`
		LocaleURL string `json:"localeURL"`
		Page      string `json:"page,omitempty"`
		Error     string `json:"error,omitempty"`
		Version   string `json:"version,omitempty"`
		// The web app sends unauthenticated viewers to these pages.
		LandingURL                 string `json:"landingURL,omitempty"`
		RequireAuthenticatedViewer bool   `json:"requireAuthenticatedViewer,omitempty"`
		Notice                     string `json:"notice,omitempty"`
		ExternalURL                string `json:"externalURL"`
	}{
		Lang:      wc.Lang,
		SubURL:    wc.SubURL,
		LocaleURL: wc.SubURL + webLocaleURL(wc.Lang),
		Page:      wc.Page,
		Error:     wc.Error,
		Version:   wc.Version,

		LandingURL:                 landingURL,
		RequireAuthenticatedViewer: opts.requireAuthenticatedViewer,
		Notice:                     wc.Notice,
		ExternalURL:                opts.externalURL,
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

// gitVersion is the version of the Git found on startup, which the admin panel
// shows.
type gitVersion string

// injected returns the values that handlers declare as parameters, for mapping
// into each flamego.Flame of the package. On top of options.Injected, the admin
// panel shows the sections that only startup reads otherwise.
func injected(cfg *conf.Config, gitVersion gitVersion) []any {
	return append(options.Injected(cfg),
		cfg.Database, cfg.Cache, cfg.Log, gitVersion,
		newSignInCookies(cfg.Session, cfg.Security, cfg.Server.Subpath),
	)
}

// initServices loads the configuration from customConf, applies the HTTP port
// override when it is positive, and starts the services that it configures. It
// returns the configuration, the version of Git, and the logger built from the
// configuration, which replaces the given console logger. It also returns that
// logger with errors that happen after logging is configured.
func initServices(logger *logx.Logger, appVersion, customConf string, portOverride int) (_ *conf.Config, _ gitVersion, configured *logx.Logger, _ error) {
	cfg, err := conf.Init(logger, customConf, false)
	if err != nil {
		return nil, "", nil, errors.Wrap(err, "init configuration")
	}
	cfg.App.Version = appVersion
	// The port override works around a port conflict on the first run. Apply it
	// before anything reads the external URL.
	if portOverride > 0 {
		overrideHTTPPort(&cfg.Server, portOverride)
	}

	logger, err = conf.InitLogging(logger, cfg, false)
	if err != nil {
		return nil, "", nil, errors.Wrap(err, "init logging")
	}
	logger.Info("Starting application", "brandName", cfg.App.BrandName, "version", cfg.App.Version)
	logger.Debug("Work directory", "workDir", conf.WorkDir())
	logger.Debug("Custom path", "customDir", conf.CustomDir())
	logger.Debug("Custom config", "customConf", cfg.CustomConf)
	logger.Debug("Log path", "logPath", cfg.Log.RootPath)
	logger.Debug("Build time", "buildTime", conf.BuildTime)
	logger.Debug("Build commit", "buildCommit", conf.BuildCommit)

	if cfg.App.IsProdMode() {
		flamego.SetEnv(flamego.EnvTypeProd)
		git.SetOutput(nil)
	} else {
		git.SetOutput(os.Stdout)
	}
	logger.Info("Run mode", "runMode", strings.Title(string(flamego.Env())))

	if cfg.Email.Enabled {
		logger.Debug("Email service is enabled")
	}

	email.NewContext(logger, cfg.Email)

	markup.NewSanitizer(cfg.Markdown.CustomURLSchemes)
	authorizedKeys := options.AuthorizedKeys(cfg)
	if err := database.NewEngine(logger.Scoped("db"), cfg.Database, cfg.App.IsProdMode(), authorizedKeys.Path); err != nil {
		return cfg, "", logger, errors.Wrap(err, "initialize ORM engine")
	}

	database.LoadRepoConfig(logger, cfg.Repository.PreferredLicenses)
	version := gitVersion(database.NewRepoContext(logger, cfg.Server.AppDataPath, cfg.LFS.ObjectsTempPath))

	cron.NewContext(logger, cfg.Cron, cfg.Repository.Root, cfg.Repository.ForcePrivate)
	database.InitSyncMirrors(logger, options.SyncMirrors(cfg))
	database.InitDeliverHooks(logger, options.DeliverHooks(cfg))
	database.InitTestPullRequests(logger, cfg.Repository.Root, cfg.Server.AppDataPath)

	if conf.HasMinWinSvc {
		logger.Info("Builtin Windows Service is supported")
	}
	if cfg.Server.LoadAssetsFromDisk {
		logger.Debug("Assets are loaded from disk")
	}

	if cfg.SSH.StartBuiltinServer {
		ssh.Listen(logger, cfg.SSH, cfg.Server.AppDataPath, cfg.App.IsProdMode(), options.Links(cfg), options.SSHServe(cfg, false))
		logger.Info("Builtin SSH server started", "listenHost", cfg.SSH.ListenHost, "listenPort", cfg.SSH.ListenPort)
		logger.Debug("SSH server cipher list", "serverCiphers", cfg.SSH.ServerCiphers)
		logger.Debug("SSH server MAC list", "serverMACs", cfg.SSH.ServerMACs)
		logger.Debug("SSH server algorithms", "serverAlgorithms", cfg.SSH.ServerAlgorithms)
	}

	if cfg.SSH.RewriteAuthorizedKeysAtStart {
		if err := database.Handle.PublicKeys().RewriteAuthorizedKeys(authorizedKeys); err != nil {
			logger.Warn("Failed to rewrite authorized_keys file", "error", err)
		}
	}

	return cfg, version, logger, nil
}
