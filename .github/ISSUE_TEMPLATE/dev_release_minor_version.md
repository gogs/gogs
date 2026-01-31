---
name: "Dev: Release a minor version"
about: ONLY USED BY MAINTAINERS.
assignees: "unknwon"
title: "Release [VERSION]"
labels: 📸 release
---

_This is generated from the [minor release template](https://github.com/gogs/gogs/blob/main/.github/ISSUE_TEMPLATE/dev_release_minor_version.md)._

## Before release

On the `main` branch:

- [ ] Close stale issues with the label [status: needs feedback](https://github.com/gogs/gogs/issues?q=is%3Aissue+is%3Aopen+label%3A%22status%3A+needs+feedback%22).
- [ ] [Sync locales from Crowdin](https://github.com/gogs/gogs/blob/main/docs/dev/import_locale.md).
- [ ] [Update CHANGELOG](https://github.com/gogs/gogs/commit/f1102a7a7c545ec221d2906f02fa19170d96f96d) to include entries for the current minor release.
	- Do not forget adding entries for GHSA patches.
- [ ] Cut a new release branch `release/<MAJOR>.<MINOR>`, e.g. `release/0.14`.

## During release

On the release branch:

- [ ] [Update the hard-coded version](https://github.com/gogs/gogs/commit/f17e7d5a2c36c52a1121d2315f3d75dcd8053b89) to the current release, e.g. `0.14.0+dev` -> `0.14.0`.
- [ ] Wait for GitHub Actions to complete and no failed jobs.
- [ ] Publish new RC releases (e.g. `v0.14.0-rc.1`, `v0.14.0-rc.2`) to ensure Docker and release workflows both succeed.
	- ⚠️ **Make sure the tag is created on the release branch**.
	- [ ] Pull down the Docker image and [run through application setup](https://github.com/gogs/gogs/blob/main/docker/README.md) to make sure nothing blows up.
	- [ ] Download one of the release archives and run through application setup to make sure nothing blows up.
- [ ] Publish a new [GitHub release](https://github.com/gogs/gogs/releases) with entries from [CHANGELOG](https://github.com/gogs/gogs/blob/main/CHANGELOG.md) for the current minor release.
	- ⚠️ **Make sure the tag is created on the release branch**.
- [ ] [Wait for a new image tag for the current release](https://github.com/gogs/gogs/actions/workflows/docker.yml?query=event%3Arelease) to be created automatically on both [Docker Hub](https://hub.docker.com/r/gogs/gogs/tags) and [GitHub Container registry](https://github.com/gogs/gogs/pkgs/container/gogs).
- [ ] [Push a new Docker image tag](https://github.com/gogs/gogs/blob/main/docs/dev/release/release_new_version.md#update-docker-image-tag) as `<MAJOR>.<MINOR>` to both [Docker Hub](https://hub.docker.com/r/gogs/gogs/tags) and [GitHub Container registry](https://github.com/gogs/gogs/pkgs/container/gogs), e.g.:
- [ ] Download all release archives and [generate SHA256 checksum](https://github.com/gogs/gogs/blob/main/docs/dev/release/sha256.sh) for all binaries to the file `checksum_sha256.txt`.
- [ ] Upload all archives and `checksum_sha256.txt` to https://dl.gogs.io.

## After release

On the `main` branch:

- [ ] Publish [GitHub security advisories](https://github.com/gogs/gogs/security) for security patches included in the release.
- [ ] Update the repository mirror on [Gitee](https://gitee.com/unknwon/gogs).
- [ ] Create a new release announcement in [Discussions](https://github.com/gogs/gogs/discussions/categories/announcements).
- [ ] Send a tweet on the [official Twitter account](https://twitter.com/GogsHQ) for the minor release.
- [ ] Close the milestone for the minor release.
- [ ] [Bump the hard-coded version](https://github.com/gogs/gogs/commit/a98968436cd5841cf691bb0b80c54c81470d1676) to the new develop version, e.g. `0.14.0+dev` -> `0.15.0+dev`.
- [ ] Run `task legacy` to identify deprecated code that is aimed to be removed in current develop version.
- [ ] **After 14 days**, publish [GitHub security advisories](https://github.com/gogs/gogs/security) for security patches included in the release.
