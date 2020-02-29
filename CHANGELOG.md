# Changelog

All notable changes to Gogs are documented in this file.

## 0.12.0+dev (`master`)

### Added

- Allow admin to remove observers from the repository. [#5803](https://github.com/gogs/gogs/pull/5803)
- Use `Last-Modified` HTTP header for raw files. [#5811](https://github.com/gogs/gogs/issues/5811)
- Support syntax highlighting for SAS code files (i.e. `.r`, `.sas`, `.tex`, `.yaml`). [#5856](https://github.com/gogs/gogs/pull/5856)
- Able to fill in pull request title with a template. [#5901](https://github.com/gogs/gogs/pull/5901)
- Able to override static files under `public/` directory, please refer to [documentation](https://gogs.io/docs/features/custom_template) for usage. [#5920](https://github.com/gogs/gogs/pull/5920)

### Changed

- The required Go version to compile source code changed to 1.13.
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
- Configuration option `[auth] ENABLE_CAPTCHA` is deprecated and will end support in 0.13.0, please start using `[auth] ENABLE_REGISTRATION_CAPTCHA`.
- Configuration option `[auth] ENABLE_NOTIFY_MAIL` is deprecated and will end support in 0.13.0, please start using `[user] ENABLE_EMAIL_NOTIFICATION`.
- Configuration option `[session] GC_INTERVAL_TIME` is deprecated and will end support in 0.13.0, please start using `[session] GC_INTERVAL`.
- Configuration option `[session] SESSION_LIFE_TIME` is deprecated and will end support in 0.13.0, please start using `[session] MAX_LIFE_TIME`.

### Fixed

- [Security] Potential open redirection with i18n.
- [Security] Potential ability to delete files outside a repository.
- [Security] Potential RCE on mirror repositories. [#5767](https://github.com/gogs/gogs/issues/5767)
- [Security] Potential XSS attack with raw markdown API. [#5907](https://github.com/gogs/gogs/pull/5907)
- Open/close milestone redirects to a 404 page. [#5677](https://github.com/gogs/gogs/issues/5677)
- Disallow multiple tokens with same name. [#5587](https://github.com/gogs/gogs/issues/5587) [#5820](https://github.com/gogs/gogs/pull/5820)
- Enable Federated Avatar Lookup could cause server to crash. [#5848](https://github.com/gogs/gogs/issues/5848)
- Private repositories are hidden in the organization's view. [#5869](https://github.com/gogs/gogs/issues/5869)
- Server error when changing email address in user settings page. [#5899](https://github.com/gogs/gogs/issues/5899)

### Removed

- Configuration option `[other] SHOW_FOOTER_VERSION`
- Configuration option `[server] STATIC_ROOT_PATH`
- Configuration option `[repository] MIRROR_QUEUE_LENGTH`
- Configuration option `[repository] PULL_REQUEST_QUEUE_LENGTH`
- Configuration option `[session] ENABLE_SET_COOKIE`
- Configuration option `[release.attachment] PATH`
- Configuration option `[webhook] QUEUE_LENGTH`

---

**Older change logs can be found on [GitHub](https://github.com/gogs/gogs/releases?after=v0.12.0).**
