# Changelog

All notable changes to Gogs are documented in this file.

## 0.15.0+dev (`main`)

### Changed

- Docker builds from `main` are now published only as `gogs/gogs:edge`, using the next-generation `Dockerfile.next`. The legacy `Dockerfile` no longer produces `main` builds. The `gogs/gogs:latest` and `gogs/gogs:next-latest` tags now always point to the highest published stable release, never to a back-patch on an older line. [#8278](https://github.com/gogs/gogs/pull/8278)
- Self-registration is now disabled by default. New instances must set `[auth] DISABLE_REGISTRATION = false` to allow sign-ups. [#8350](https://github.com/gogs/gogs/pull/8350)

### Fixed

- _Security:_ Open redirect on the `/redirect` endpoint and post-action flows via a backslash in the redirect target. [#8391](https://github.com/gogs/gogs/pull/8391) - [GHSA-3g28-2vwg-gxq6](https://github.com/gogs/gogs/security/advisories/GHSA-3g28-2vwg-gxq6)
- _Security:_ Argument injection through a crafted commit or branch reference on several repository API endpoints allowed an authenticated user with read access to leak internal server details to the error logs. [#8393](https://github.com/gogs/gogs/pull/8393) - [GHSA-mxrh-2rxr-6mqc](https://github.com/gogs/gogs/security/advisories/GHSA-mxrh-2rxr-6mqc)
- _Security:_ Argument injection through a crafted branch name when creating a pull request allowed an authenticated user with write access to write files to arbitrary paths on the server. [#8390](https://github.com/gogs/gogs/pull/8390) - [GHSA-2grc-qr7q-6m36](https://github.com/gogs/gogs/security/advisories/GHSA-2grc-qr7q-6m36)
- _Security:_ The "remember me" auto-login cookie was derived from database columns, so an attacker with a database dump could forge a valid cookie for any user. The auto-login cookie path has been removed entirely. Persistence is now provided by the server-issued session cookie. [#8289](https://github.com/gogs/gogs/pull/8289) - [GHSA-4pph-25p3-pw73](https://github.com/gogs/gogs/security/advisories/GHSA-4pph-25p3-pw73)

### Removed

- The web-based first-time install page at `/install`, along with the `[security] INSTALL_LOCK` configuration option. A working `custom/conf/app.ini` is now always required. [#8350](https://github.com/gogs/gogs/pull/8350)
- Support for customizing pages with Go template files stops being possible.
- Support for MSSQL as the database backend. [#8173](https://github.com/gogs/gogs/pull/8173)
- Support for `memcache` as the cache adapter. [#8270](https://github.com/gogs/gogs/pull/8270)
- The `gogs cert` subcommand. [#8153](https://github.com/gogs/gogs/pull/8153)
- The `[email] DISABLE_HELO` configuration option. HELO/EHLO is now always sent during SMTP handshake. [#8164](https://github.com/gogs/gogs/pull/8164)
- The `/debug`, `/debug/pprof/*`, `/debug/profile/*`, and `/urlmap.json` endpoints. [#8271](https://github.com/gogs/gogs/pull/8271)
- CSRF protection and the `[session] CSRF_COOKIE_NAME` configuration option. [#8300](https://github.com/gogs/gogs/pull/8300)
- Support for DSA public keys. The `DSA` entry under `[ssh.minimum_key_sizes]` is no longer recognized. [#8313](https://github.com/gogs/gogs/pull/8313)

## 0.14.3

### Fixed

- _Security:_ Reverse proxy authentication header was honored from any remote address, allowing user impersonation when Gogs was reachable directly. The header is now only trusted from addresses listed in `[auth] TRUSTED_PROXY_IPS`. [#8264](https://github.com/gogs/gogs/pull/8264) - [GHSA-w6j9-vw59-27wv](https://github.com/gogs/gogs/security/advisories/GHSA-w6j9-vw59-27wv)
- _Security:_ Server-side request forgery in webhook deliveries via HTTP redirects to local network addresses. [#8263](https://github.com/gogs/gogs/pull/8263) - [GHSA-c4v7-xg93-qf8g](https://github.com/gogs/gogs/security/advisories/GHSA-c4v7-xg93-qf8g)
- _Security:_ Denial of service when rendering issue references against a malformed external issue tracker URL format. [#8312](https://github.com/gogs/gogs/pull/8312) - [GHSA-4j89-2c4f-44c6](https://github.com/gogs/gogs/security/advisories/GHSA-4j89-2c4f-44c6)
- _Security:_ Stored XSS in Jupyter notebook (`.ipynb`) preview through Markdown links with `javascript:` URLs. [#8319](https://github.com/gogs/gogs/pull/8319) - [GHSA-jq8v-rmf6-65jw](https://github.com/gogs/gogs/security/advisories/GHSA-jq8v-rmf6-65jw)
- _Security:_ Missing authorization check on the attachment download endpoint allowed anyone who knew (or guessed) an attachment UUID to download files belonging to private repositories. [#8320](https://github.com/gogs/gogs/pull/8320) - [GHSA-p9f5-h3rx-j5qw](https://github.com/gogs/gogs/security/advisories/GHSA-p9f5-h3rx-j5qw)
- _Security:_ Organization team and member management actions accepted GET requests, allowing a logged-in owner to be tricked into adding an attacker to the Owners team via a crafted link. [#8321](https://github.com/gogs/gogs/pull/8321) - [GHSA-pwx3-qcgw-vh7h](https://github.com/gogs/gogs/security/advisories/GHSA-pwx3-qcgw-vh7h)
- _Security:_ SSRF via mirror address update bypassing clone address validation. [#8225](https://github.com/gogs/gogs/pull/8225) - [GHSA-wv27-2vqp-j7g5](https://github.com/gogs/gogs/security/advisories/GHSA-wv27-2vqp-j7g5)
- _Security:_ Open redirect on login and other post-action flows via the `redirect_to` query parameter. [#8322](https://github.com/gogs/gogs/pull/8322) - [GHSA-xxhq-69mf-w8cr](https://github.com/gogs/gogs/security/advisories/GHSA-xxhq-69mf-w8cr)
- _Security:_ Privilege escalation to repository owner via collaboration access mode update. [#8227](https://github.com/gogs/gogs/pull/8227) - [GHSA-4565-r4x7-hg8j](https://github.com/gogs/gogs/security/advisories/GHSA-4565-r4x7-hg8j)
- _Security:_ SSRF in repository migration and recurring mirror sync via HTTP redirects and stale host validation on stored mirror URLs. [#8324](https://github.com/gogs/gogs/pull/8324) - [GHSA-g2f5-gjr4-qjvm](https://github.com/gogs/gogs/security/advisories/GHSA-g2f5-gjr4-qjvm)
- _Security:_ Remote command execution via pull request rebase merges with crafted branch names. [#8301](https://github.com/gogs/gogs/pull/8301) - [GHSA-qf6p-p7ww-cwr9](https://github.com/gogs/gogs/security/advisories/GHSA-qf6p-p7ww-cwr9)
- _Security:_ Stored XSS in the milestone dropdown on the new issue page via crafted milestone names. [#8325](https://github.com/gogs/gogs/pull/8325) - [GHSA-vcm5-gvmp-78mp](https://github.com/gogs/gogs/security/advisories/GHSA-vcm5-gvmp-78mp)
- _Security:_ Stored XSS in Jupyter notebook (`.ipynb`) preview through `data:text/html` URIs that bypassed the sanitizer. [#8326](https://github.com/gogs/gogs/pull/8326) - [GHSA-3w28-36p9-w929](https://github.com/gogs/gogs/security/advisories/GHSA-3w28-36p9-w929)
- _Security:_ Write-level collaborators could change admin-only repository settings (issue tracker, wiki, mirror sync) via API. [#8327](https://github.com/gogs/gogs/pull/8327) - [GHSA-268j-37xf-pp52](https://github.com/gogs/gogs/security/advisories/GHSA-268j-37xf-pp52)
- _Security:_ Password reset tokens stayed valid for the account-activation lifetime, ignoring `[auth] RESET_PASSWORD_CODE_LIVES`. [#8328](https://github.com/gogs/gogs/pull/8328) - [GHSA-5c3f-6486-3g7g](https://github.com/gogs/gogs/security/advisories/GHSA-5c3f-6486-3g7g)
- _Security:_ Stored XSS in Jupyter notebook (`.ipynb`) preview through raw HTML in markdown cells. [#8330](https://github.com/gogs/gogs/pull/8330) - [GHSA-6vxv-wg6j-5qwp](https://github.com/gogs/gogs/security/advisories/GHSA-6vxv-wg6j-5qwp)
- _Security:_ Read-only Git HTTP access could be confused with write access during repository pushes. [#8331](https://github.com/gogs/gogs/pull/8331) - [GHSA-wmfg-5p4h-5fw3](https://github.com/gogs/gogs/security/advisories/GHSA-wmfg-5p4h-5fw3)
- _Security:_ Arbitrary file write outside the repository working tree via crafted upload filename routed through a committed directory symlink. [#8332](https://github.com/gogs/gogs/pull/8332) - [GHSA-89mr-xqfv-758m](https://github.com/gogs/gogs/security/advisories/GHSA-89mr-xqfv-758m)
- _Security:_ Cross-repository disclosure of Git LFS object contents by binding a known OID to another repository without proving possession of the bytes. [#8333](https://github.com/gogs/gogs/pull/8333) - [GHSA-6p9m-q3jp-47h4](https://github.com/gogs/gogs/security/advisories/GHSA-6p9m-q3jp-47h4)
- _Security:_ Remote code execution via path traversal in organization names accepted through the API. [#8334](https://github.com/gogs/gogs/pull/8334) - [GHSA-c39w-43gm-34h5](https://github.com/gogs/gogs/security/advisories/GHSA-c39w-43gm-34h5)
- _Security:_ Stalled SSH handshakes pinned a file descriptor and goroutine indefinitely. The built-in SSH server now drops connections that do not complete the handshake within 15 seconds. [#8335](https://github.com/gogs/gogs/pull/8335) - [GHSA-xp79-5mx3-jx52](https://github.com/gogs/gogs/security/advisories/GHSA-xp79-5mx3-jx52)
- _Security:_ Organization metadata and team list endpoints were reachable without authentication. [#8336](https://github.com/gogs/gogs/pull/8336) - [GHSA-744x-3838-5r56](https://github.com/gogs/gogs/security/advisories/GHSA-744x-3838-5r56)

## 0.14.2

### Fixed

- _Security:_ Denial of service in repository and wiki file listing pages via crafted file names. [#8116](https://github.com/gogs/gogs/pull/8116) - [GHSA-3qq3-668m-v9mj](https://github.com/gogs/gogs/security/advisories/GHSA-3qq3-668m-v9mj)
- _Security:_ Cross-repository LFS object overwrite via missing content hash verification. [#8166](https://github.com/gogs/gogs/pull/8166) - [GHSA-gmf8-978x-2fg2](https://github.com/gogs/gogs/security/advisories/GHSA-gmf8-978x-2fg2)
- _Security:_ Stored XSS via data URI in issue comments. [#8174](https://github.com/gogs/gogs/pull/8174) - [GHSA-xrcr-gmf5-2r8j](https://github.com/gogs/gogs/security/advisories/GHSA-xrcr-gmf5-2r8j)
- _Security:_ Release tag option injection in release deletion. [#8175](https://github.com/gogs/gogs/pull/8175) - [GHSA-v9vm-r24h-6rqm](https://github.com/gogs/gogs/security/advisories/GHSA-v9vm-r24h-6rqm)
- _Security:_ Stored XSS in branch and wiki views through author and committer names. [#8176](https://github.com/gogs/gogs/pull/8176) - [GHSA-vgvf-m4fw-938j](https://github.com/gogs/gogs/security/advisories/GHSA-vgvf-m4fw-938j)
- _Security:_ DOM-based XSS via issue meta selection on the issue page. [#8178](https://github.com/gogs/gogs/pull/8178) - [GHSA-vgjm-2cpf-4g7c](https://github.com/gogs/gogs/security/advisories/GHSA-vgjm-2cpf-4g7c)
- Unable to update files via web editor and API. [#8184](https://github.com/gogs/gogs/pull/8184)

### Removed

- Support for passing API access tokens via URL query parameters (`token`, `access_token`). Use the `Authorization` header instead. [#8177](https://github.com/gogs/gogs/pull/8177) - [GHSA-x9p5-w45c-7ffc](https://github.com/gogs/gogs/security/advisories/GHSA-x9p5-w45c-7ffc)

## 0.14.1

### Added

- Support comparing tags in addition to branches. [#6141](https://github.com/gogs/gogs/issues/6141)
- Show file name in browser tab title when viewing files. [#5896](https://github.com/gogs/gogs/pull/5896)
- Support using TLS for Redis session provider using `[session] PROVIDER_CONFIG = ...,tls=true`. [#7860](https://github.com/gogs/gogs/pull/7860)
- Support expanading values in `app.ini` from environment variables, e.g. `[database] PASSWORD = ${DATABASE_PASSWORD}`. [#8057](https://github.com/gogs/gogs/pull/8057)
- Support custom logout URL that users get redirected to after sign out using `[auth] CUSTOM_LOGOUT_URL`. [#8089](https://github.com/gogs/gogs/pull/8089)
- Start publishing next-generation, security-focused Docker image via `gogs/gogs:next-latest`, which will become the default image distribution (`gogs/gogs:latest`) starting 0.16.0. While not all container options support have been added in the next-generation image, the use of current legacy Docker image is deprecated, it will be published as `gogs/gogs:legacy-latest` starting 0.16.0, and be completely removed no earlier than 0.17.0. [#8061](https://github.com/gogs/gogs/pull/8061)

### Changed

- The required Go version to compile source code changed to 1.25.
- The build tag `cert` has been removed, and the `gogs cert` subcommand is now always available. [#7883](https://github.com/gogs/gogs/pull/7883)
- Switched to pure-Go SQLite driver, CGO is no longer required to compile Gogs. [#7882](https://github.com/gogs/gogs/issues/7882)
- Updated Mermaid JS to 11.9.0. [#8009](https://github.com/gogs/gogs/pull/8009)
- Halt the repository creation and leave the directory untouched if the repository root already exists. [#8091](https://github.com/gogs/gogs/pull/8091)

### Fixed

- _Security:_ Unauthenticated file upload. [#8128](https://github.com/gogs/gogs/pull/8128) - [GHSA-fc3h-92p8-h36f](https://github.com/gogs/gogs/security/advisories/GHSA-fc3h-92p8-h36f)
- _Security:_ Protected branch bypass in web UI. [#8124](https://github.com/gogs/gogs/pull/8124) - [GHSA-2c6v-8r3v-gh6p](https://github.com/gogs/gogs/security/advisories/GHSA-2c6v-8r3v-gh6p)
- _Security:_ Authorization bypass allows cross-repository label modification. [#8123](https://github.com/gogs/gogs/pull/8123) - [GHSA-cv22-72px-f4gh](https://github.com/gogs/gogs/security/advisories/GHSA-cv22-72px-f4gh)
- _Security:_ Cross-repository comment deletion. [#8119](https://github.com/gogs/gogs/pull/8119) - [GHSA-jj5m-h57j-5gv7](https://github.com/gogs/gogs/security/advisories/GHSA-jj5m-h57j-5gv7)
- 500 error on repository watchers and stargazers pages when using MSSQL. [#5482](https://github.com/gogs/gogs/issues/5482)
- Submodules using `ssh://` protocol and a port number are not rendered correctly. [#4941](https://github.com/gogs/gogs/issues/4941)
- Missing link to user profile on the first commit in commits history page. [#7404](https://github.com/gogs/gogs/issues/7404)
- Unable to delete or display files with special characters in their names. [#7596](https://github.com/gogs/gogs/issues/7596)
- Docker healthcheck fails when `HTTP_PROXY` or `HTTPS_PROXY` environment variables are set. [#7529](https://github.com/gogs/gogs/issues/7529)

## 0.13.4

### Fixed

- _Security:_ DoS in repository mirror sync. [#8065](https://github.com/gogs/gogs/pull/8065) - [GHSA-cr88-6mqm-4g57](https://github.com/gogs/gogs/security/advisories/GHSA-cr88-6mqm-4g57)
- _Security:_ RCE in repository put contents API. [#8082](https://github.com/gogs/gogs/pull/8082) - [GHSA-gg64-xxr9-qhjp](https://github.com/gogs/gogs/security/advisories/GHSA-gg64-xxr9-qhjp)
- _Security:_ Arbitrary file deletion via path traversal in wiki page update. [#8099](https://github.com/gogs/gogs/pull/8099) - [GHSA-jp7c-wj6q-3qf2](https://github.com/gogs/gogs/security/advisories/GHSA-jp7c-wj6q-3qf2)
- _Security:_ 2FA bypass via recovery code. [#8100](https://github.com/gogs/gogs/pull/8100) - [GHSA-p6x6-9mx6-26wj](https://github.com/gogs/gogs/security/advisories/GHSA-p6x6-9mx6-26wj)
- _Security:_ Authorization bypass in repository deletion API. [#8101](https://github.com/gogs/gogs/pull/8101) - [GHSA-rjv5-9px2-fqw6](https://github.com/gogs/gogs/security/advisories/GHSA-rjv5-9px2-fqw6)
- _Security:_ Update repository content via API with read-only permission. [#8102](https://github.com/gogs/gogs/pull/8102) - [GHSA-5qhx-gwfj-6jqr](https://github.com/gogs/gogs/security/advisories/GHSA-5qhx-gwfj-6jqr)
- _Security:_ Arbitrary file read/write via path traversal in Git hook editing. [#8103](https://github.com/gogs/gogs/pull/8103) - [GHSA-mrph-w4hh-gx3g](https://github.com/gogs/gogs/security/advisories/GHSA-mrph-w4hh-gx3g)
- _Security:_ Stored XSS via Mermaid diagrams. [`2c88cd4`](https://github.com/gogs/gogs/commit/2c88cd4d9fdc346d8e06d82f5368d657c10e79c2) - [GHSA-26gq-grmh-6xm6](https://github.com/gogs/gogs/security/advisories/GHSA-26gq-grmh-6xm6)
- Route `GET /api/v1/user/repos` responses 500 when accessible repositories contain forks. [#8069](https://github.com/gogs/gogs/pull/8069)
- Newer Git versions that uses default branch `main` cause wiki initialization to fail. [#8094](https://github.com/gogs/gogs/pull/8094)

## 0.13.3

### Fixed

- _Security:_ Stored XSS in PDF renderer. [GHSA-xh32-cx6c-cp4v](https://github.com/gogs/gogs/security/advisories/GHSA-xh32-cx6c-cp4v)
- _Security:_ Path Traversal in file editing UI. [GHSA-wj44-9vcg-wjq7](https://github.com/gogs/gogs/security/advisories/GHSA-wj44-9vcg-wjq7)
- Randomly timeout on repository file uploads. [#7890](https://github.com/gogs/gogs/pull/7890)
- Unable to override email templates in custom directory. [#7905](https://github.com/gogs/gogs/pull/7905)

## 0.13.2

### Fixed

- _Security:_ Path Traversal in file editing UI. [GHSA-r7j8-5h9c-f6fx](https://github.com/gogs/gogs/security/advisories/GHSA-r7j8-5h9c-f6fx)
- _Security:_ Path Traversal in file update API. [GHSA-qf5v-rp47-55gg](https://github.com/gogs/gogs/security/advisories/GHSA-qf5v-rp47-55gg)
- _Security:_ Argument Injection in the built-in SSH server. [GHSA-vm62-9jw3-c8w3](https://github.com/gogs/gogs/security/advisories/GHSA-vm62-9jw3-c8w3)
- _Security:_ Deletion of internal files. [GHSA-ccqv-43vm-4f3w](https://github.com/gogs/gogs/security/advisories/GHSA-ccqv-43vm-4f3w)
- _Security:_ Argument Injection during changes preview. [GHSA-9pp6-wq8c-3w2c](https://github.com/gogs/gogs/security/advisories/GHSA-9pp6-wq8c-3w2c)
- _Security:_ Argument Injection when tagging new releases. [GHSA-m27m-h5gj-wwmg](https://github.com/gogs/gogs/security/advisories/GHSA-m27m-h5gj-wwmg)
- Use the non-deprecated section name `[email]` during installation for email settings. [#7704](https://github.com/gogs/gogs/pull/7704)
- Use the non-deprecated section name `[email] PASSWORD` during installation for email password. [#7807](https://github.com/gogs/gogs/pull/7807)
- Make purple template label color to actually use the hexcode of purple. [#7722](https://github.com/gogs/gogs/pull/7722)

## 0.13.0

### Added

- Support using personal access token in the password field. [#3866](https://github.com/gogs/gogs/issues/3866)
- An unlisted option is added when create or migrate a repository. Unlisted repositories are public but not being listed for users without direct access in the UI. [#5733](https://github.com/gogs/gogs/issues/5733)
- New API endpoint `PUT /repos/:owner/:repo/contents/:path` for creating and update repository contents. [#5967](https://github.com/gogs/gogs/issues/5967)
- New configuration option `[git.timeout] DIFF` for customizing operation timeout of `git diff`. [#6315](https://github.com/gogs/gogs/issues/6315)
- New configuration option `[server] SSH_SERVER_MACS` for setting list of accepted MACs for connections to builtin SSH server. [#6434](https://github.com/gogs/gogs/issues/6434)
- New configuration option `[repository] DEFAULT_BRANCH` for setting default branch name for new repositories. [#7291](https://github.com/gogs/gogs/issues/7291)
- New configuration option `[server] SSH_SERVER_ALGORITHMS` for specifying the list of accepted key exchange algorithms for connections to builtin SSH server. [#7345](https://github.com/gogs/gogs/pull/7345)
- Support specifying custom schema for PostgreSQL. [#6695](https://github.com/gogs/gogs/pull/6695)
- Support rendering Mermaid diagrams in Markdown. [#6776](https://github.com/gogs/gogs/pull/6776)
- Docker: Allow passing extra arguments to the `backup` command. [#7060](https://github.com/gogs/gogs/pull/7060)
- New languages support: Mongolian, Romanian. [#6510](https://github.com/gogs/gogs/pull/6510) [#7082](https://github.com/gogs/gogs/pull/7082)

### Changed

- The default branch has been changed to `main`. [#6285](https://github.com/gogs/gogs/pull/6285)
- MSSQL as database backend is deprecated, installation page no longer shows it as an option. Existing installations and manually craft configuration file continue to work. [#6295](https://github.com/gogs/gogs/pull/6295)
- Use [Task](https://github.com/go-task/task) as the build tool. [#6297](https://github.com/gogs/gogs/pull/6297)
- The required Go version to compile source code changed to 1.18.
- Access tokens are now stored using their SHA256 hashes instead of raw values. [#7008](https://github.com/gogs/gogs/pull/7008)

### Fixed

- Unable to use LDAP authentication on ARM machines. [#6761](https://github.com/gogs/gogs/issues/6761)
- Unable to choose "Lookup Avatar by mail" in user settings without deleting custom avatar. [#7267](https://github.com/gogs/gogs/pull/7267)
- Mistakenly include the "data" directory under the custom directory in the Docker setup. [#7343](https://github.com/gogs/gogs/pull/7343)
- Unable to start after data recovery with an outdated migration version. [#7125](https://github.com/gogs/gogs/issues/7125)

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
- Remove option to use Makefile as the build tool. [#6980](https://github.com/gogs/gogs/pull/6980)

## 0.12.11

### Fixed

- _Security:_ Stored XSS for issue assignees. [#7145](https://github.com/gogs/gogs/issues/7145)
- _Security:_ OS Command Injection in repo editor on case-insensitive file systems. [#7030](https://github.com/gogs/gogs/issues/7030)
- Unable to render repository pages with implicit submodules (e.g. `get submodule "REDACTED": revision does not exist`). [#6436](https://github.com/gogs/gogs/issues/6436)

## 0.12.10

### Changed

- Support using `[security] LOCAL_NETWORK_ALLOWLIST = *` to allow all hostnames. [#7111](https://github.com/gogs/gogs/pull/7111)

### Fixed

- Unable to send webhooks to local network addresses after configured `[security] LOCAL_NETWORK_ALLOWLIST`. [#7074](https://github.com/gogs/gogs/issues/7074)

## 0.12.9

### Fixed

- _Security:_ OS Command Injection in file editor. [#7000](https://github.com/gogs/gogs/issues/7000)
- _Security:_ Sanitize `DisplayName` in repository issue list. [#7009](https://github.com/gogs/gogs/pull/7009)
- _Security:_ Path Traversal in file editor on Windows. [#7001](https://github.com/gogs/gogs/issues/7001)
- _Security:_ Path Traversal in Git HTTP endpoints. [#7002](https://github.com/gogs/gogs/issues/7002)
- Unable to init repository during creation on Windows. [#6967](https://github.com/gogs/gogs/issues/6967)
- Mysterious panic on `Value not found for type *repo.HTTPContext`. [#6963](https://github.com/gogs/gogs/issues/6963)

## 0.12.8

### Changed

- All users (including admins) need to use the configuration option `[security] LOCAL_NETWORK_ALLOWLIST` to allow repository migration and webhooks to be able to access local network addresses, which is a comma separated list of hostnames. [#6988](https://github.com/gogs/gogs/pull/6988)

### Fixed

- _Security:_ SSRF in webhook. [#6901](https://github.com/gogs/gogs/issues/6901)
- _Security:_ XSS in cookies. [#6953](https://github.com/gogs/gogs/issues/6953)
- _Security:_ OS Command Injection in file uploading. [#6968](https://github.com/gogs/gogs/issues/6968)
- _Security:_ Remote Command Execution in file editing. [#6555](https://github.com/gogs/gogs/issues/6555)

## 0.12.7

### Fixed

- _Security:_ Stored XSS in issues. [#6919](https://github.com/gogs/gogs/issues/6919)
- Invalid character in `Access-Control-Allow-Credentials` response header. [#4983](https://github.com/gogs/gogs/issues/4983)
- Mysterious `ssh: overflow reading version string` errors from builtin SSH server. [#6882](https://github.com/gogs/gogs/issues/6882)

## 0.12.6

### Fixed

- _Security:_ Remote command execution in file uploading. [#6833](https://github.com/gogs/gogs/issues/6833)
- _Regression:_ Unable to migrate repository from other local Git hosting. Added a new configuration option `[security] LOCAL_NETWORK_ALLOWLIST`, which is a comma separated list of hostnames that are explicitly allowed to be accessed within the local network. [#6841](https://github.com/gogs/gogs/issues/6841)
- Slow start of Docker containers using NAS devices. [#6554](https://github.com/gogs/gogs/issues/6554)

## 0.12.5

### Fixed

- _Security:_ Potential SSRF in repository migration. [#6754](https://github.com/gogs/gogs/issues/6754)
- _Security:_ Improper PAM authorization handling. [#6810](https://github.com/gogs/gogs/issues/6810)

## 0.12.4

### Fixed

- _Security:_ Potential SSRF attack by CRLF injection via repository migration. [#6413](https://github.com/gogs/gogs/issues/6413)
- _Regression:_ Fixed smart links for issues stops rendering. [#6506](https://github.com/gogs/gogs/issues/6506)
- Added `X-Frame-Options` header to prevent Clickjacking. [#6409](https://github.com/gogs/gogs/issues/6409)

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
