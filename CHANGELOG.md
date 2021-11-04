# Changelog

All notable changes to Gogs are documented in this file.

## 0.13.0+dev (`main`)

### Added

- An unlisted option is added when create or migrate a repository. Unlisted repositories are public but not being listed for users without direct access in the UI. [#5733](https://github.com/gogs/gogs/issues/5733)
- New configuration option `[git.timeout] DIFF` for customizing operation timeout of `git diff`. [#6315](https://github.com/gogs/gogs/issues/6315)
- New configuration option `[server] SSH_SERVER_MACS` for setting list of accepted MACs for connections to builtin SSH server. [#6434](https://github.com/gogs/gogs/issues/6434)
- New languages support: Mongolian. [#6510](https://github.com/gogs/gogs/pull/6510)

### Changed

- The default branch has been changed to `main`. [#6285](https://github.com/gogs/gogs/pull/6285)
- MSSQL as database backend is deprecated, installation page no longer shows it as an option. Existing installations and manually craft configuration file continue to work. [#6295](https://github.com/gogs/gogs/pull/6295)
- Use [Task](https://github.com/go-task/task) as the default build tool for development. [#6297](https://github.com/gogs/gogs/pull/6297)

### Fixed

- Add `X-Frame-Options` header to prevent Clickjacking. [#6409](https://github.com/gogs/gogs/issues/6409)
- [Security] Potential SSRF attack by CRLF injection via repository migration. [#6413](https://github.com/gogs/gogs/issues/6413)


### Removed

- ⚠️ Migrations before 0.12 are removed, installations not on 0.12 should upgrade to it to run the migrations and then upgrade to 0.13.
- Configuration section `[mailer]` is no longer used, please use `[email]`.
- Configuration section `[service]` is no longer used, please use `[auth]`.
- Configuration option `APP_NAME` is no longer used, please use `BRAND_NAME`.
- Configuration option `[security] REVERSE_PROXY_AUTHENTICATION_USER` is no longer used, please use `[auth] REVERSE_PROXY_AUTHENTICATION_HEADER`.
- Configuration option `[auth] ACTIVE_CODE_LIVE_MINUTES` is no longer used, please use `[auth] ACTIVATE_CODE_LIVES`.
- Configuration option `[auth] RESET_PASSWD_CODE_LIVE_MINUTES` is no longer used, please use `[auth] RESET_PASSWORD_CODE_LIVES`.
- Configuration option `[auth] ENABLE_CAPTCHA` is no longer used, please use `[auth] ENABLE_REGISTRATION_CAPTCHA`.
- Configuration option `[auth] ENABLE_NOTIFY_MAIL` is no longer used, please use `[user] ENABLE_EMAIL_NOTIFICATION`.
- Configuration option `[auth] REGISTER_EMAIL_CONFIRM` is no longer used, please use `[auth] REQUIRE_EMAIL_CONFIRMATION`.
- Configuration option `[session] GC_INTERVAL_TIME` is no longer used, please use `[session] GC_INTERVAL`.
- Configuration option `[session] SESSION_LIFE_TIME` is no longer used, please use `[session] MAX_LIFE_TIME`.
- Configuration option `[server] ROOT_URL` is no longer used, please use `[server] EXTERNAL_URL`.
- Configuration option `[server] LANDING_PAGE` is no longer used, please use `[server] LANDING_URL`.
- Configuration option `[database] DB_TYPE` is no longer used, please use `[database] TYPE`.
- Configuration option `[database] PASSWD` is no longer used, please use `[database] PASSWORD`.

## 0.12.3

### Fixed

- _Regression:_ When running Gogs on Windows, push commits no longer fail on a daily basis with the error "pre-receive hook declined". [#6316](https://github.com/gogs/gogs/issues/6316)
- Auto-linked commit SHAs now have correct links. [#6300](https://github.com/gogs/gogs/issues/6300)
- Git LFS client (with version >= 2.5.0) wasn't able to upload files with known format (e.g. PNG, JPEG), and the server is expecting the HTTP Header `Content-Type` to be `application/octet-stream`. The server now tells the LFS client to always use `Content-Type: application/octet-stream` when upload files.

## 0.12.2

### Fixed

- _Regression:_ Pages are correctly rendered when requesting `?go-get=1` for subdirectories. [#6314](https://github.com/gogs/gogs/issues/6314)
- _Regression:_ Submodule with a relative path is linked correctly. [#6319](https://github.com/gogs/gogs/issues/6319)
- Backup can be processed when `--target` is specified on Windows. [#6339](https://github.com/gogs/gogs/issues/6339)
- Commit message contains keywords look like an issue reference no longer fails the push entirely. [#6289](https://github.com/gogs/gogs/issues/6289)

## 0.12.1

### Fixed

- The `updated_at` field is now correctly updated when updates an issue. [#6209](https://github.com/gogs/gogs/issues/6209)
- Fixed a regression which created `login_source.cfg` column to have `VARCHAR(255)` instead of `TEXT` in MySQL. [#6280](https://github.com/gogs/gogs/issues/6280)

## 0.12.0

### Added

- Support for Git LFS, you can read documentation for both [user](https://github.com/gogs/gogs/blob/main/docs/user/lfs.md) and [admin](https://github.com/gogs/gogs/blob/main/docs/admin/lfs.md). [#1322](https://github.com/gogs/gogs/issues/1322)
- Allow admin to remove observers from the repository. [#5803](https://github.com/gogs/gogs/pull/5803)
- Use `Last-Modified` HTTP header for raw files. [#5811](https://github.com/gogs/gogs/issues/5811)
- Support syntax highlighting for SAS code files (i.e. `.r`, `.sas`, `.tex`, `.yaml`). [#5856](https://github.com/gogs/gogs/pull/5856)
- Able to fill in pull request title with a template. [#5901](https://github.com/gogs/gogs/pull/5901)
- Able to override static files under `public/` directory, please refer to [documentation](https://gogs.io/docs/features/custom_template) for usage. [#5920](https://github.com/gogs/gogs/pull/5920)
- New API endpoint `GET /admin/teams/:teamid/members` to list members of a team. [#5877](https://github.com/gogs/gogs/issues/5877)
- Support backup with retention policy for Docker deployments. [#6140](https://github.com/gogs/gogs/pull/6140)

### Changed

- The organization profile page has changed to display at most 12 members. [#5506](https://github.com/gogs/gogs/issues/5506)
- The required Go version to compile source code changed to 1.14.
- All assets are now embedded into binary and served from memory by default. Set `[server] LOAD_ASSETS_FROM_DISK = true` to load them from disk. [#5920](https://github.com/gogs/gogs/pull/5920)
- Application and Go versions are removed from page footer and only show in the admin dashboard.
- Build tag for running as Windows Service has been changed from `miniwinsvc` to `minwinsvc`.
- Configuration option `APP_NAME` is deprecated and will end support in 0.13.0, please start using `BRAND_NAME`.
- Configuration option `[server] ROOT_URL` is deprecated and will end support in 0.13.0, please start using `[server] EXTERNAL_URL`.
- Configuration option `[server] LANDING_PAGE` is deprecated and will end support in 0.13.0, please start using `[server] LANDING_URL`.
- Configuration option `[database] DB_TYPE` is deprecated and will end support in 0.13.0, please start using `[database] TYPE`.
- Configuration option `[database] PASSWD` is deprecated and will end support in 0.13.0, please start using `[database] PASSWORD`.
- Configuration option `[security] REVERSE_PROXY_AUTHENTICATION_USER` is deprecated and will end support in 0.13.0, please start using `[auth] REVERSE_PROXY_AUTHENTICATION_HEADER`.
- Configuration section `[mailer]` is deprecated and will end support in 0.13.0, please start using `[email]`.
- Configuration section `[service]` is deprecated and will end support in 0.13.0, please start using `[auth]`.
- Configuration option `[auth] ACTIVE_CODE_LIVE_MINUTES` is deprecated and will end support in 0.13.0, please start using `[auth] ACTIVATE_CODE_LIVES`.
- Configuration option `[auth] RESET_PASSWD_CODE_LIVE_MINUTES` is deprecated and will end support in 0.13.0, please start using `[auth] RESET_PASSWORD_CODE_LIVES`.
- Configuration option `[auth] REGISTER_EMAIL_CONFIRM` is deprecated and will end support in 0.13.0, please start using `[auth] REQUIRE_EMAIL_CONFIRMATION`.
- Configuration option `[auth] ENABLE_CAPTCHA` is deprecated and will end support in 0.13.0, please start using `[auth] ENABLE_REGISTRATION_CAPTCHA`.
- Configuration option `[auth] ENABLE_NOTIFY_MAIL` is deprecated and will end support in 0.13.0, please start using `[user] ENABLE_EMAIL_NOTIFICATION`.
- Configuration option `[session] GC_INTERVAL_TIME` is deprecated and will end support in 0.13.0, please start using `[session] GC_INTERVAL`.
- Configuration option `[session] SESSION_LIFE_TIME` is deprecated and will end support in 0.13.0, please start using `[session] MAX_LIFE_TIME`.
- The name `-` is reserved and cannot be used for users or organizations.

### Fixed

- [Security] Potential open redirection with i18n.
- [Security] Potential ability to delete files outside a repository.
- [Security] Potential ability to set primary email on others' behalf from their verified emails.
- [Security] Potential XSS attack via `.ipynb`. [#5170](https://github.com/gogs/gogs/issues/5170)
- [Security] Potential SSRF attack via webhooks. [#5366](https://github.com/gogs/gogs/issues/5366)
- [Security] Potential CSRF attack in admin panel. [#5367](https://github.com/gogs/gogs/issues/5367)
- [Security] Potential stored XSS attack in some browsers. [#5397](https://github.com/gogs/gogs/issues/5397)
- [Security] Potential RCE on mirror repositories. [#5767](https://github.com/gogs/gogs/issues/5767)
- [Security] Potential XSS attack with raw markdown API. [#5907](https://github.com/gogs/gogs/pull/5907)
- File both modified and renamed within a commit treated as separate files. [#5056](https://github.com/gogs/gogs/issues/5056)
- Unable to restore the database backup to MySQL 8.0 with syntax error. [#5602](https://github.com/gogs/gogs/issues/5602)
- Open/close milestone redirects to a 404 page. [#5677](https://github.com/gogs/gogs/issues/5677)
- Disallow multiple tokens with same name. [#5587](https://github.com/gogs/gogs/issues/5587) [#5820](https://github.com/gogs/gogs/pull/5820)
- Enable Federated Avatar Lookup could cause server to crash. [#5848](https://github.com/gogs/gogs/issues/5848)
- Private repositories are hidden in the organization's view. [#5869](https://github.com/gogs/gogs/issues/5869)
- Users have access to base repository cannot view commits in forks. [#5878](https://github.com/gogs/gogs/issues/5878)
- Server error when changing email address in user settings page. [#5899](https://github.com/gogs/gogs/issues/5899)
- Fall back to use RFC 3339 as time layout when misconfigured. [#6098](https://github.com/gogs/gogs/issues/6098)
- Unable to update team with server error. [#6185](https://github.com/gogs/gogs/issues/6185)
- Webhooks are not fired after push when `[service] REQUIRE_SIGNIN_VIEW = true`.
- Files with identical content are randomly displayed one of them.

### Removed

- Configuration option `[other] SHOW_FOOTER_VERSION`
- Configuration option `[server] STATIC_ROOT_PATH`
- Configuration option `[repository] MIRROR_QUEUE_LENGTH`
- Configuration option `[repository] PULL_REQUEST_QUEUE_LENGTH`
- Configuration option `[session] ENABLE_SET_COOKIE`
- Configuration option `[release.attachment] PATH`
- Configuration option `[webhook] QUEUE_LENGTH`
- Build tag `sqlite`, which means CGO is now required.

---

**Older change logs can be found on [GitHub](https://github.com/gogs/gogs/releases?after=v0.12.0).**
