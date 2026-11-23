package web

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/flamego/flamego"
	"unknwon.dev/x/logx"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/cron"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/email"
	"gogs.io/gogs/internal/i18n"
)

// initTime is the time when the application was initialized.
var initTime = time.Now()

// adminReportRow is one setting or measurement of a report in the admin panel,
// with its label already translated.
type adminReportRow struct {
	Label string `json:"label"`
	// Kind tells how to show the value: "code", "block" for multi-line code,
	// "bool", or "unset" when the setting has no value.
	Kind  string `json:"kind"`
	Value string `json:"value,omitempty"`
	// On is the value of a "bool" row.
	On bool `json:"on,omitempty"`
	// Unit follows a "code" value as text, e.g., seconds.
	Unit string `json:"unit,omitempty"`
	// Hint is a note shown after the value, e.g., which database types use it.
	Hint string `json:"hint,omitempty"`
}

// adminReportSection is a titled part of a report. Groups are shown with a
// divider between them.
type adminReportSection struct {
	ID     string             `json:"id"`
	Title  string             `json:"title"`
	Groups [][]adminReportRow `json:"groups"`
}

// adminReport builds the rows of a report, translating their labels.
type adminReport struct {
	l i18n.Locale
}

// withHint returns the row with a translated note shown after its value.
func (r adminReport) withHint(row adminReportRow, hintKey string) adminReportRow {
	row.Hint = r.l.Tr(hintKey)
	return row
}

// code is a value shown as code, e.g., as it is set in the configuration.
func (r adminReport) code(key string, value any) adminReportRow {
	return adminReportRow{Label: r.l.Tr(key), Kind: "code", Value: fmt.Sprint(value)}
}

// codeUnit is a code value followed by the unit of unitKey in its plural form
// for the value, e.g., minutes.
func (r adminReport) codeUnit(key string, value any, unitKey string) adminReportRow {
	row := r.code(key, value)
	row.Unit = r.l.Tr(unitKey, "count", value)
	return row
}

// optional is a code row, or "unset" when the value is empty.
func (r adminReport) optional(key, value string) adminReportRow {
	if value == "" {
		return adminReportRow{Label: r.l.Tr(key), Kind: "unset", Value: r.l.Tr("admin::config.not_set")}
	}
	return r.code(key, value)
}

func (r adminReport) list(key string, values []string) adminReportRow {
	return r.optional(key, strings.Join(values, ", "))
}

func (r adminReport) flag(key string, on bool) adminReportRow {
	value := r.l.Tr("modal::no")
	if on {
		value = r.l.Tr("modal::yes")
	}
	return adminReportRow{Label: r.l.Tr(key), Kind: "bool", Value: value, On: on}
}

func (r adminReport) section(id, titleKey string, groups ...[]adminReportRow) adminReportSection {
	return adminReportSection{ID: id, Title: r.l.Tr(titleKey), Groups: groups}
}

func rows(rs ...adminReportRow) []adminReportRow { return rs }

type adminDashboard struct {
	AppVersion  string `json:"appVersion"`
	GitVersion  string `json:"gitVersion"`
	GoVersion   string `json:"goVersion"`
	BuildTime   string `json:"buildTime"`
	BuildCommit string `json:"buildCommit"`
	// StartedAt is when the server started, which its uptime counts from.
	StartedAt time.Time `json:"startedAt"`
	// UptimeSeconds is how long the server has run. A browser cannot count it
	// from StartedAt, since its clock may differ from the clock of the server.
	UptimeSeconds int64                 `json:"uptimeSeconds"`
	Statistics    []adminStatisticGroup `json:"statistics"`
	SystemStatus  adminReportSection    `json:"systemStatus"`
}

// adminStatisticGroup is a set of related statistics, with its title already
// translated.
type adminStatisticGroup struct {
	Title string           `json:"title"`
	Items []adminStatistic `json:"items"`
}

// adminStatistic is the number of records of one kind in the database, with
// its label already translated. The key names the kind, e.g., "public_keys".
type adminStatistic struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Count int64  `json:"count"`
}

func getAdminDashboard(logger *logx.Logger, c flamego.Context, l i18n.Locale) (statusCode int, resp *adminDashboard, err error) {
	s := database.GetStatistic(c.Request().Context(), logger).Counter
	r := adminReport{l: l}
	// The key of a statistic also names its label in the locale files.
	statistic := func(key string, count int64) adminStatistic {
		return adminStatistic{Key: key, Label: l.Tr("admin::dashboard.statistic_" + key), Count: count}
	}
	// Measurements show the whole value, with its unit, as code.
	size := func(key string, n uint64) adminReportRow {
		return r.code(key, fileSize(int64(n)))
	}
	seconds := func(key string, ns uint64, precision int) adminReportRow {
		return r.code(key, strconv.FormatFloat(float64(ns)/float64(time.Second), 'f', precision, 64)+"s")
	}

	m := new(runtime.MemStats)
	runtime.ReadMemStats(m)
	status := r.section("system-status", "admin::dashboard.system_status",
		rows(
			r.code("admin::dashboard.current_goroutine", runtime.NumGoroutine()),
			size("admin::dashboard.current_memory_usage", m.Alloc),
			size("admin::dashboard.total_memory_allocated", m.TotalAlloc),
			size("admin::dashboard.memory_obtained", m.Sys),
			r.code("admin::dashboard.pointer_lookup_times", m.Lookups),
			r.code("admin::dashboard.memory_allocate_times", m.Mallocs),
			r.code("admin::dashboard.memory_free_times", m.Frees),
		),
		rows(
			size("admin::dashboard.current_heap_usage", m.HeapAlloc),
			size("admin::dashboard.heap_memory_obtained", m.HeapSys),
			size("admin::dashboard.heap_memory_idle", m.HeapIdle),
			size("admin::dashboard.heap_memory_in_use", m.HeapInuse),
			size("admin::dashboard.heap_memory_released", m.HeapReleased),
			r.code("admin::dashboard.heap_objects", m.HeapObjects),
		),
		rows(
			size("admin::dashboard.bootstrap_stack_usage", m.StackInuse),
			size("admin::dashboard.stack_memory_obtained", m.StackSys),
			size("admin::dashboard.mspan_structures_usage", m.MSpanInuse),
			size("admin::dashboard.mspan_structures_obtained", m.MSpanSys),
			size("admin::dashboard.mcache_structures_usage", m.MCacheInuse),
			size("admin::dashboard.mcache_structures_obtained", m.MCacheSys),
			size("admin::dashboard.profiling_bucket_hash_table_obtained", m.BuckHashSys),
			size("admin::dashboard.gc_metadata_obtained", m.GCSys),
			size("admin::dashboard.other_system_allocation_obtained", m.OtherSys),
		),
		rows(
			size("admin::dashboard.next_gc_recycle", m.NextGC),
			seconds("admin::dashboard.last_gc_time", uint64(time.Now().UnixNano())-m.LastGC, 1),
			seconds("admin::dashboard.total_gc_pause", m.PauseTotalNs, 1),
			seconds("admin::dashboard.last_gc_pause", m.PauseNs[(m.NumGC+255)%256], 3),
			r.code("admin::dashboard.gc_times", m.NumGC),
		),
	)

	return http.StatusOK, &adminDashboard{
		AppVersion:    conf.App.Version,
		GitVersion:    conf.Git.Version,
		GoVersion:     runtime.Version(),
		BuildTime:     conf.BuildTime,
		BuildCommit:   conf.BuildCommit,
		StartedAt:     initTime,
		UptimeSeconds: int64(time.Since(initTime).Seconds()),
		Statistics: []adminStatisticGroup{
			{l.Tr("admin::dashboard.statistic_group_people"), []adminStatistic{
				statistic("users", s.User),
				statistic("organizations", s.Org),
				statistic("social_accounts", s.Oauth),
				statistic("follows", s.Follow),
				statistic("teams", s.Team),
			}},
			{l.Tr("admin::dashboard.statistic_group_repositories"), []adminStatistic{
				statistic("repositories", s.Repo),
				statistic("watches", s.Watch),
				statistic("stars", s.Star),
				statistic("accesses", s.Access),
				statistic("mirrors", s.Mirror),
				statistic("releases", s.Release),
			}},
			{l.Tr("admin::dashboard.statistic_group_issues"), []adminStatistic{
				statistic("actions", s.Action),
				statistic("issues", s.Issue),
				statistic("comments", s.Comment),
				statistic("milestones", s.Milestone),
				statistic("labels", s.Label),
				statistic("attachments", s.Attachment),
			}},
			{l.Tr("admin::dashboard.statistic_group_access"), []adminStatistic{
				statistic("public_keys", s.PublicKey),
				statistic("login_sources", s.LoginSource),
				statistic("webhooks", s.Webhook),
				statistic("hook_tasks", s.HookTask),
				statistic("update_tasks", s.UpdateTask),
			}},
		},
		SystemStatus: status,
	}, nil
}

// adminOperations are the maintenance tasks an admin can run from the
// dashboard, by name, with the key of the message shown when one succeeds.
var adminOperations = map[string]struct {
	run        func(*logx.Logger) error
	successKey string
}{
	"delete-inactive-users": {func(*logx.Logger) error { return database.Handle.Users().DeleteInactivated() }, "admin::dashboard.delete_inactivate_accounts_success"},
	"delete-repo-archives":  {func(*logx.Logger) error { return database.DeleteRepositoryArchives() }, "admin::dashboard.delete_repo_archives_success"},
	"delete-missing-repos":  {database.DeleteMissingRepositories, "admin::dashboard.delete_missing_repos_success"},
	"git-gc-repos":          {func(*logx.Logger) error { return database.GitGcRepos() }, "admin::dashboard.git_gc_repos_success"},
	"resync-ssh-keys":       {func(*logx.Logger) error { return database.Handle.PublicKeys().RewriteAuthorizedKeys() }, "admin::dashboard.resync_all_sshkeys_success"},
	"resync-repo-hooks":     {func(*logx.Logger) error { return database.SyncRepositoryHooks() }, "admin::dashboard.resync_all_hooks_success"},
	"reinit-missing-repos":  {database.ReinitMissingRepositories, "admin::dashboard.reinit_missing_repos_success"},
}

type adminMessage struct {
	Message string `json:"message"`
}

func postAdminOperation(logger *logx.Logger, c flamego.Context, l i18n.Locale, admin *database.User) (statusCode int, resp any, err error) {
	name := c.Param("name")
	op, ok := adminOperations[name]
	if !ok {
		return http.StatusNotFound, nil, errors.New("operation not found")
	}
	if err = op.run(logger); err != nil {
		logger.Error("Failed to run admin operation", "operation", name, "error", err)
		// Admins need the reason to fix what failed, e.g., the repository that
		// could not be collected.
		return http.StatusInternalServerError, &bindingErrorResponse{Error: err.Error()}, nil
	}
	logger.Debug("Admin operation run", "operation", name, "adminName", admin.Name)
	return http.StatusOK, &adminMessage{Message: l.Tr(op.successKey)}, nil
}

type adminConfig struct {
	Sections []adminReportSection `json:"sections"`
	// EmailEnabled is true when the site can send a test email.
	EmailEnabled bool `json:"emailEnabled"`
}

func getAdminConfig(l i18n.Locale) (statusCode int, resp *adminConfig, err error) {
	r := adminReport{l: l}
	var sections []adminReportSection

	sections = append(sections, r.section("server", "admin::config.server_config",
		rows(
			r.code("admin::config.brand_name", conf.App.BrandName),
			r.code("admin::config.run_user", conf.App.RunUser),
			r.code("admin::config.run_mode", conf.App.RunMode),
		),
		rows(
			r.code("admin::config.server.external_url", conf.Server.ExternalURL),
			r.code("admin::config.server.domain", conf.Server.Domain),
			r.code("admin::config.server.protocol", conf.Server.Protocol),
			r.code("admin::config.server.http_addr", conf.Server.HTTPAddr),
			r.code("admin::config.server.http_port", conf.Server.HTTPPort),
			r.code("admin::config.server.cert_file", conf.Server.CertFile),
			r.code("admin::config.server.key_file", conf.Server.KeyFile),
			r.code("admin::config.server.tls_min_version", conf.Server.TLSMinVersion),
			r.code("admin::config.server.unix_socket_permission", conf.Server.UnixSocketPermission),
			r.code("admin::config.server.local_root_url", conf.Server.LocalRootURL),
		),
		rows(
			r.flag("admin::config.server.disable_router_log", conf.Server.DisableRouterLog),
			r.flag("admin::config.server.enable_gzip", conf.Server.EnableGzip),
		),
		rows(
			r.code("admin::config.server.app_data_path", conf.Server.AppDataPath),
			r.flag("admin::config.server.load_assets_from_disk", conf.Server.LoadAssetsFromDisk),
		),
		rows(r.code("admin::config.server.landing_url", conf.Server.LandingURL)),
	))

	ssh := r.section("ssh", "admin::config.ssh_config", rows(r.flag("admin::config.ssh.enabled", !conf.SSH.Disabled)))
	if !conf.SSH.Disabled {
		keySizes := make([]string, 0, len(conf.SSH.MinimumKeySizes))
		for typ, size := range conf.SSH.MinimumKeySizes {
			keySizes = append(keySizes, typ+": "+strconv.Itoa(size))
		}
		slices.Sort(keySizes)
		ssh.Groups[0] = append(ssh.Groups[0],
			r.code("admin::config.ssh.domain", conf.SSH.Domain),
			r.code("admin::config.ssh.port", conf.SSH.Port),
			r.code("admin::config.ssh.root_path", conf.SSH.RootPath),
			r.code("admin::config.ssh.keygen_path", conf.SSH.KeygenPath),
			r.code("admin::config.ssh.key_test_path", conf.SSH.KeyTestPath),
			r.flag("admin::config.ssh.minimum_key_size_check", conf.SSH.MinimumKeySizeCheck),
			r.list("admin::config.ssh.minimum_key_sizes", keySizes),
			r.flag("admin::config.ssh.rewrite_authorized_keys_at_start", conf.SSH.RewriteAuthorizedKeysAtStart),
		)
		ssh.Groups = append(ssh.Groups, rows(
			r.flag("admin::config.ssh.start_builtin_server", conf.SSH.StartBuiltinServer),
			r.code("admin::config.ssh.listen_host", conf.SSH.ListenHost),
			r.code("admin::config.ssh.listen_port", conf.SSH.ListenPort),
			r.list("admin::config.ssh.server_ciphers", conf.SSH.ServerCiphers),
			r.list("admin::config.ssh.server_macs", conf.SSH.ServerMACs),
			r.list("admin::config.ssh.server_algorithms", conf.SSH.ServerAlgorithms),
		))
	}
	sections = append(sections, ssh)

	repo := conf.Repository
	sections = append(sections, r.section("repository", "admin::config.repo_config",
		rows(
			r.code("admin::config.repo.root_path", repo.Root),
			r.code("admin::config.repo.script_type", repo.ScriptType),
			r.optional("admin::config.repo.ansi_chatset", repo.ANSICharset),
			r.flag("admin::config.repo.force_private", repo.ForcePrivate),
			r.code("admin::config.repo.max_creation_limit", repo.MaxCreationLimit),
			r.list("admin::config.repo.preferred_licenses", repo.PreferredLicenses),
			r.flag("admin::config.repo.disable_http_git", repo.DisableHTTPGit),
			r.flag("admin::config.repo.enable_local_path_migration", repo.EnableLocalPathMigration),
			r.flag("admin::config.repo.enable_raw_file_render_mode", repo.EnableRawFileRenderMode),
			r.code("admin::config.repo.commits_fetch_concurrency", repo.CommitsFetchConcurrency),
		),
		rows(
			r.list("admin::config.repo.editor.line_wrap_extensions", repo.Editor.LineWrapExtensions),
			r.list("admin::config.repo.editor.previewable_file_modes", repo.Editor.PreviewableFileModes),
		),
		rows(
			r.flag("admin::config.repo.upload.enabled", repo.Upload.Enabled),
			r.code("admin::config.repo.upload.temp_path", repo.Upload.TempPath),
			r.list("admin::config.repo.upload.allowed_types", repo.Upload.AllowedTypes),
			r.codeUnit("admin::config.repo.upload.file_max_size", repo.Upload.FileMaxSize, "MB"),
			r.code("admin::config.repo.upload.max_files", repo.Upload.MaxFiles),
		),
	))

	db := conf.Database
	sections = append(sections, r.section("database", "admin::config.db_config",
		rows(
			r.code("admin::config.db.type", db.Type),
			r.code("admin::config.db.host", db.Host),
			r.code("admin::config.db.name", db.Name),
			r.withHint(r.code("admin::config.db.schema", db.Schema), "admin::config.db.schema_helper"),
			r.code("admin::config.db.user", db.User),
			r.withHint(r.code("admin::config.db.ssl_mode", db.SSLMode), "admin::config.db.ssl_mode_helper"),
			r.withHint(r.code("admin::config.db.path", db.Path), "admin::config.db.path_helper"),
		),
		rows(
			r.code("admin::config.db.max_open_conns", db.MaxOpenConns),
			r.code("admin::config.db.max_idle_conns", db.MaxIdleConns),
		),
	))

	sec := conf.Security
	sections = append(sections, r.section("security", "admin::config.security_config", rows(
		r.code("admin::config.security.login_remember_days", sec.LoginRememberDays),
		r.flag("admin::config.security.enable_login_status_cookie", sec.EnableLoginStatusCookie),
		r.code("admin::config.security.login_status_cookie_name", sec.LoginStatusCookieName),
		r.list("admin::config.security.local_network_allowlist", sec.LocalNetworkAllowlist),
	)))

	mail := r.section("email", "admin::config.email_config", rows(r.flag("admin::config.email.enabled", conf.Email.Enabled)))
	if conf.Email.Enabled {
		e := conf.Email
		mail.Groups[0] = append(mail.Groups[0],
			r.code("admin::config.email.subject_prefix", e.SubjectPrefix),
			r.code("admin::config.email.host", e.Host),
			r.code("admin::config.email.from", e.From),
			r.code("admin::config.email.user", e.User),
		)
		mail.Groups = append(mail.Groups,
			rows(r.optional("admin::config.email.helo_hostname", e.HELOHostname)),
			rows(
				r.flag("admin::config.email.skip_verify", e.SkipVerify),
				r.flag("admin::config.email.use_certificate", e.UseCertificate),
				r.code("admin::config.email.cert_file", e.CertFile),
				r.code("admin::config.email.key_file", e.KeyFile),
			),
			rows(
				r.flag("admin::config.email.use_plain_text", e.UsePlainText),
				r.flag("admin::config.email.add_plain_text_alt", e.AddPlainTextAlt),
			),
		)
	}
	sections = append(sections, mail)

	auth := conf.Auth
	sections = append(sections, r.section("auth", "admin::config.auth_config",
		rows(
			r.codeUnit("admin::config.auth.activate_code_lives", auth.ActivateCodeLives, "tool::raw_minutes"),
			r.codeUnit("admin::config.auth.reset_password_code_lives", auth.ResetPasswordCodeLives, "tool::raw_minutes"),
			r.flag("admin::config.auth.require_email_confirm", auth.RequireEmailConfirmation),
			r.flag("admin::config.auth.require_sign_in_view", auth.RequireSigninView),
			r.flag("admin::config.auth.disable_registration", auth.DisableRegistration),
			r.optional("admin::config.auth.anti_bot_verification", auth.AntiBotVerification),
		),
		rows(
			r.flag("admin::config.auth.enable_reverse_proxy_authentication", auth.EnableReverseProxyAuthentication),
			r.flag("admin::config.auth.enable_reverse_proxy_auto_registration", auth.EnableReverseProxyAutoRegistration),
			r.code("admin::config.auth.reverse_proxy_authentication_header", auth.ReverseProxyAuthenticationHeader),
			r.list("admin::config.auth.trusted_proxy_ips", auth.TrustedProxyIPs),
			r.optional("admin::config.auth_custom_logout_url", auth.CustomLogoutURL),
		),
	))

	sections = append(sections, r.section("user", "admin::config.user_config", rows(
		r.flag("admin::config.user.enable_email_notify", conf.User.EnableEmailNotification),
	)))

	sess := conf.Session
	sections = append(sections, r.section("session", "admin::config.session_config", rows(
		r.code("admin::config.session.provider", sess.Provider),
		r.code("admin::config.session.provider_config", sess.ProviderConfig),
		r.code("admin::config.session.cookie_name", sess.CookieName),
		r.flag("admin::config.session.https_only", sess.CookieSecure),
		r.codeUnit("admin::config.session.gc_interval", sess.GCInterval, "tool::raw_seconds"),
		r.codeUnit("admin::config.session.max_life_time", sess.MaxLifeTime, "tool::raw_seconds"),
	)))

	// The cache host is left out because it can hold credentials, e.g., a
	// Redis password.
	sections = append(sections, r.section("cache", "admin::config.cache_config", rows(
		r.code("admin::config.cache.adapter", conf.Cache.Adapter),
		r.codeUnit("admin::config.cache.interval", conf.Cache.Interval, "tool::raw_seconds"),
	)))

	att := conf.Attachment
	sections = append(sections, r.section("attachment", "admin::config.attachment_config", rows(
		r.flag("admin::config.attachment.enabled", att.Enabled),
		r.code("admin::config.attachment.path", att.Path),
		r.list("admin::config.attachment.allowed_types", att.AllowedTypes),
		r.codeUnit("admin::config.attachment.max_size", att.MaxSize, "MB"),
		r.code("admin::config.attachment.max_files", att.MaxFiles),
	)))

	rel := conf.Release.Attachment
	sections = append(sections, r.section("release", "admin::config.release_config", rows(
		r.flag("admin::config.release.attachment.enabled", rel.Enabled),
		r.list("admin::config.release.attachment.allowed_types", rel.AllowedTypes),
		r.codeUnit("admin::config.release.attachment.max_size", rel.MaxSize, "MB"),
		r.code("admin::config.release.attachment.max_files", rel.MaxFiles),
	)))

	pic := conf.Picture
	sections = append(sections, r.section("picture", "admin::config.picture_config", rows(
		r.code("admin::config.picture.avatar_upload_path", pic.AvatarUploadPath),
		r.code("admin::config.picture.repo_avatar_upload_path", pic.RepositoryAvatarUploadPath),
	)))

	sections = append(sections,
		r.section("http", "admin::config.http_config", rows(
			r.optional("admin::config.http.access_control_allow_origin", conf.HTTP.AccessControlAllowOrigin),
		)),
		r.section("mirror", "admin::config.mirror_config", rows(
			r.codeUnit("admin::config.mirror.default_interval", conf.Mirror.DefaultInterval, "tool::raw_hours"),
		)),
		r.section("webhook", "admin::config.webhook_config", rows(
			r.list("admin::config.webhook.types", conf.Webhook.Types),
			r.codeUnit("admin::config.webhook.deliver_timeout", conf.Webhook.DeliverTimeout, "tool::raw_seconds"),
			r.flag("admin::config.webhook.skip_tls_verify", conf.Webhook.SkipTLSVerify),
		)),
	)

	git := conf.Git
	sections = append(sections, r.section("git", "admin::config.git_config",
		rows(
			r.optional("admin::config.git.gc_args", strings.Join(git.GCArgs, " ")),
		),
		rows(
			r.codeUnit("admin::config.git.migrate_timeout", git.Timeout.Migrate, "tool::raw_seconds"),
			r.codeUnit("admin::config.git.mirror_timeout", git.Timeout.Mirror, "tool::raw_seconds"),
			r.codeUnit("admin::config.git.gc_timeout", git.Timeout.GC, "tool::raw_seconds"),
		),
	))

	sections = append(sections, r.section("lfs", "admin::config.lfs_config", rows(
		r.code("admin::config.lfs.storage", conf.LFS.Storage),
		r.code("admin::config.lfs.objects_path", conf.LFS.ObjectsPath),
	)))

	logs := r.section("log", "admin::config.log_config", rows(r.code("admin::config.log_file_root_path", conf.Log.RootPath)))
	for i, mode := range conf.Log.Modes {
		options, _ := json.MarshalIndent(conf.Log.Configs[i], "", "  ")
		logs.Groups = append(logs.Groups, rows(
			r.code("admin::config.log_mode", mode),
			adminReportRow{Label: l.Tr("admin::config.log_options"), Kind: "block", Value: string(options)},
		))
	}
	sections = append(sections, logs)

	return http.StatusOK, &adminConfig{Sections: sections, EmailEnabled: conf.Email.Enabled}, nil
}

type adminTestMailRequest struct {
	Email string `json:"email" validate:"required,email,max=254"`
}

func postAdminTestMail(logger *logx.Logger, l i18n.Locale, req adminTestMailRequest) (statusCode int, resp any, err error) {
	if !conf.Email.Enabled {
		return http.StatusNotFound, nil, errors.New("email is not enabled")
	}
	if err = email.SendTestMail(logger, req.Email); err != nil {
		logger.Error("Failed to send test mail", "email", req.Email, "error", err)
		// The mail server refused or could not be reached, and the reason helps
		// the admin fix the settings.
		return http.StatusBadGateway, &bindingErrorResponse{Error: l.Tr("admin::config.email.test_mail_failed", "email", req.Email, "error", err)}, nil
	}
	return http.StatusOK, &adminMessage{Message: l.Tr("admin::config.email.test_mail_sent", "email", req.Email)}, nil
}

type adminCronTask struct {
	Description string `json:"description"`
	Schedule    string `json:"schedule"`
	// Next is when the task runs next, or null when it never will.
	Next *time.Time `json:"next"`
	// Prev is when the task last ran, or null when it has not run yet.
	Prev      *time.Time `json:"prev"`
	ExecTimes int        `json:"execTimes"`
}

type adminMonitor struct {
	CronTasks []adminCronTask `json:"cronTasks"`
}

func getAdminMonitor() (statusCode int, resp *adminMonitor, err error) {
	timeOrNil := func(t time.Time) *time.Time {
		if t.IsZero() {
			return nil
		}
		return &t
	}
	entries := cron.ListTasks()
	tasks := make([]adminCronTask, 0, len(entries))
	for _, e := range entries {
		tasks = append(tasks, adminCronTask{
			Description: e.Description,
			Schedule:    e.Spec,
			Next:        timeOrNil(e.Next),
			Prev:        timeOrNil(e.Prev),
			ExecTimes:   e.ExecTimes,
		})
	}
	return http.StatusOK, &adminMonitor{CronTasks: tasks}, nil
}

// fileSize formats the size in bytes as a user-friendly string, e.g., "1.5 MB".
func fileSize(s int64) string {
	if s < 10 {
		return fmt.Sprintf("%d B", s)
	}
	sizes := []string{"B", "KB", "MB", "GB", "TB", "PB", "EB"}
	e := math.Floor(math.Log(float64(s)) / math.Log(1024))
	val := float64(s) / math.Pow(1024, e)
	f := "%.0f"
	if val < 10 {
		f = "%.1f"
	}
	return fmt.Sprintf(f+" %s", val, sizes[int(e)])
}
