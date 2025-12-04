---
name: "Dev: Release a minor version"
about: ONLY USED BY MAINTAINERS.
title: "Release [VERSION]"
labels: 📸 release
---

_This is generated from the [minor release template](https://github.com/gogs/gogs/blob/main/.github/ISSUE_TEMPLATE/dev_release_minor_version.md)._

## Before release

On the `main` branch:

- [ ] Close stale issues with the label [status: needs feedback](https://github.com/gogs/gogs/issues?q=is%3Aissue+is%3Aopen+label%3A%22status%3A+needs+feedback%22).
- [ ] [Sync locales from Crowdin](https://github.com/gogs/gogs/blob/main/docs/dev/import_locale.md).
- [ ] [Update CHANGELOG](https://github.com/gogs/gogs/commit/540134d4436d8da82247dd2cabe9312ca2f5b1f1) to include entries for the current minor release.
- [ ] Cut a new release branch `release/<MAJOR>.<MINOR>`, e.g. `release/0.12`.

## During release

On the release branch:

- [ ] [Update the hard-coded version](https://github.com/gogs/gogs/commit/f17e7d5a2c36c52a1121d2315f3d75dcd8053b89) to the current release, e.g. `0.12.0+dev` -> `0.12.0`.
- [ ] Wait for GitHub Actions to complete and no failed jobs.
- [ ] Publish new RC releases (e.g. `v0.12.0-rc.1`, `v0.12.0-rc.2`) to ensure Docker workflow succeeds. **Make sure the tag is created on the release branch**.
	- Pull down the Docker image and [run through application setup](https://github.com/gogs/gogs/blob/main/docker/README.md) to make sure nothing blows up.
- [ ] Publish a new [GitHub release](https://github.com/gogs/gogs/releases) with entries from [CHANGELOG](https://github.com/gogs/gogs/blob/main/CHANGELOG.md) for the current minor release. **Make sure the tag is created on the release branch**.
- [ ] [Wait for a new image tag for the current release](https://github.com/gogs/gogs/actions/workflows/docker.yml?query=event%3Arelease) to be created automatically on both [Docker Hub](https://hub.docker.com/r/gogs/gogs/tags) and [GitHub Container registry](https://github.com/gogs/gogs/pkgs/container/gogs).
- [ ] [Push a new Docker image tag](https://github.com/gogs/gogs/blob/main/docs/dev/release/release_new_version.md#update-docker-image-tag) as `<MAJOR>.<MINOR>` to both [Docker Hub](https://hub.docker.com/r/gogs/gogs/tags) and [GitHub Container registry](https://github.com/gogs/gogs/pkgs/container/gogs), e.g.:
- [ ] [Compile and pack binaries](https://github.com/gogs/gogs/blob/main/docs/dev/release/release_new_version.md#compile-and-pack-binaries) (all prefixed with `gogs_<MAJOR>.<MINOR>.<PATCH>_`, e.g. `gogs_0.12.0_`):
	- [ ] macOS: `darwin_amd64.zip`, `darwin_arm64.zip`
	- [ ] Linux: `linux_386.tar.gz`, `linux_386.zip`, `linux_amd64.tar.gz`, `linux_amd64.zip`
	- [ ] ARM: `linux_armv7.tar.gz`, `linux_armv7.zip`, `linux_armv8.tar.gz`, `linux_armv8.zip`
	- [ ] Windows: `windows_amd64.zip`, `windows_amd64_mws.zip`
- [ ] [Generate SHA256 checksum](https://github.com/gogs/gogs/blob/main/docs/dev/release/sha256.sh) for all binaries to the file `checksum_sha256.txt`.
- [ ] Upload all binaries and `checksum_sha256.txt` to:
	- [ ] GitHub release
	- [ ] https://dl.gogs.io
- [ ] Update content of [Install from binary](https://gogs.io/docs/installation/install_from_binary).

## After release

On the `main` branch:

- [ ] Publish [GitHub security advisories](https://github.com/gogs/gogs/security) for security patches included in the release.
- [ ] Update the repository mirror on [Gitee](https://gitee.com/unknwon/gogs).
- [ ] Create a new release announcement in [Discussions](https://github.com/gogs/gogs/discussions/categories/announcements).
- [ ] Send a tweet on the [official Twitter account](https://twitter.com/GogsHQ) for the minor release.
- [ ] Publish a new release article on [OSChina](http://my.oschina.net/Obahua/admin/releases).
- [ ] Close the minor milestone.
- [ ] [Bump the hard-coded version](https://github.com/gogs/gogs/commit/a98968436cd5841cf691bb0b80c54c81470d1676) to the new develop version, e.g. `0.12.0+dev` -> `0.13.0+dev`.
- [ ] Run `task legacy` to identify deprecated code that is aimed to be removed in current develop version.
