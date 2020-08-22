## Before release

On develop branch:

- [ ] Close stale issues with the label [status: needs feedback](https://github.com/gogs/gogs/issues?q=is%3Aissue+is%3Aopen+label%3A%22status%3A+needs+feedback%22).
- [ ] [Sync locales from Crowdin](https://github.com/gogs/gogs/blob/master/docs/dev/import_locale.md).
- [ ] Update [CHANGELOG](https://github.com/gogs/gogs/blob/master/CHANGELOG.md) to include entries for the current release.
- [ ] Cut a new release branch `release/<MAJOR>.<MINOR>`, e.g. `release/0.12`.

## During release

On release branch:

- [ ] Update the [hard-coded version](https://github.com/gogs/gogs/blob/master/gogs.go#L21) to the current release, e.g. `0.12.0+dev` -> `0.12.0`.
- [ ] Publish a new [GitHub release](https://github.com/gogs/gogs/releases) with entries from [CHANGELOG](https://github.com/gogs/gogs/blob/master/CHANGELOG.md) for the current release.
- [ ] Wait for a new [Docker Hub tag](https://hub.docker.com/r/gogs/gogs/tags) for the current release to be created automatically.
- [ ] Compile and pack binaries (all prefixed with `gogs_<MAJOR>.<MINOR>.<PATCH>_`, e.g. `gogs_0.12.0_`):
	- [ ] macOS: `darwin_amd64.zip`
	- [ ] Linux: `linux_386.tar.gz`, `linux_386.zip`, `linux_amd64.tar.gz`, `linux_amd64.zip`
	- [ ] ARM: `linux_armv7.zip`
	- [ ] Windows: `windows_amd64.zip`, `windows_amd64_mws.zip`
- [ ] Generate SHA256 checksum for all binaries to the file `checksum_sha256.txt`.
- [ ] Upload all binaries to:
	- [ ] GitHub release
	- [ ] KeyCDN
	- [ ] https://dl.gogs.io (also upload `checksum_sha256.txt`)
- [ ] Build, push and tag a new Docker image for ARM to [Docker Hub](https://hub.docker.com/r/gogs/gogs-rpi).

## After release

On develop branch:

- [ ] Update the repository mirror on [Gitee](https://gitee.com/unknwon/gogs).
- [ ] Create a new release topic on [Gogs Discussion](https://discuss.gogs.io/c/announcements/5).
- [ ] Send out release announcement emails via [Mailchimp](https://mailchimp.com/).
- [ ] Publish a new release article on [OSChina](http://my.oschina.net/Obahua/admin/releases).
- [ ] Update the [hard-coded version](https://github.com/gogs/gogs/blob/master/gogs.go#L21) to the new develop version, e.g. `0.12.0+dev` -> `0.13.0+dev`.
- [ ] Run `make legacy` to identify deprecated code that is aimed to be removed in current develop version.
