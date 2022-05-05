---
name: Dev: Release a patch version
about: ONLY USED BY MAINTAINERS.
title: "Release [VERSION]"
labels: 📸 release
---

_This is generated from the [patch release template](https://github.com/gogs/gogs/blob/main/.github/ISSUE_TEMPLATE/dev_release_patch_version.md)._

## Before release

On the release branch:

- [ ] Make sure all commits are cherry-picked from the `main` branch by checking the patch milestone.
- [ ] [Update CHANGELOG on the `main` branch](https://github.com/gogs/gogs/commit/e6c5633f580399c8f4dfc07166a63a01c6c70346) to include entries for the current patch release.

## During release

On the release branch:

- [ ] [Update the hard-coded version](https://github.com/gogs/gogs/commit/f0e3cd90f8d7695960eeef2e4e54b2e717302f6c) to the current release, e.g. `0.12.0` -> `0.12.1`.
- [ ] Wait for GitHub Actions to complete and no failed jobs.
- [ ] Publish new RC releases (e.g. `v0.12.0-rc.1`, `v0.12.0-rc.2`) to ensure Docker workflow succeeds. **Make sure the tag is created on the release branch**.
	- [ ] Pull down the Docker image and [run through application setup](https://github.com/gogs/gogs/blob/main/docker/README.md) to make sure nothing blows up.
- [ ] Publish a new [GitHub release](https://github.com/gogs/gogs/releases) with entries from [CHANGELOG](https://github.com/gogs/gogs/blob/main/CHANGELOG.md) for the current patch release and all previous releases with same minor version. **Make sure the tag is created on the release branch**.
	- [ ] Only mention patch authors for the current release.
- [ ] Update all previous GitHub releases with same minor version with the warning:
	```
	**ℹ️ Heads up! There is a new patch release [0.12.1](https://github.com/gogs/gogs/releases/tag/v0.12.1) available, we recommend directly installing or upgrading to that version.**
	```
- [ ] [Wait for a new image tag for the current release](https://github.com/gogs/gogs/actions/workflows/docker.yml?query=event%3Arelease) to be created automatically on both [Docker Hub](https://hub.docker.com/r/gogs/gogs/tags) and [GitHub Container registry](https://github.com/gogs/gogs/pkgs/container/gogs).
- [ ] [Update Docker image tag](https://github.com/gogs/gogs/blob/main/docs/dev/release/release_new_version.md#update-docker-image-tag) for the minor release `<MAJOR>.<MINOR>` on both [Docker Hub](https://hub.docker.com/r/gogs/gogs/tags) and [GitHub Container registry](https://github.com/gogs/gogs/pkgs/container/gogs).
- [ ] [Compile and pack binaries](https://github.com/gogs/gogs/blob/main/docs/dev/release/release_new_version.md#compile-and-pack-binaries) (all prefixed with `gogs_<MAJOR>.<MINOR>.<PATCH>_`, e.g. `gogs_0.12.0_`):
	- [ ] macOS: `darwin_amd64.zip`, `darwin_arm64.zip`
	- [ ] Linux: `linux_386.tar.gz`, `linux_386.zip`, `linux_amd64.tar.gz`, `linux_amd64.zip`
	- [ ] ARM: `linux_armv7.tar.gz`, `linux_armv7.zip`, `linux_armv8.tar.gz`, `linux_armv8.zip`
	- [ ] Windows: `windows_amd64.zip`, `windows_amd64_mws.zip`
- [ ] [Generate SHA256 checksum](https://github.com/gogs/gogs/blob/main/docs/dev/release/sha256.sh) for all binaries to the file `checksum_sha256.txt`.
- [ ] Upload all binaries to:
	- [ ] GitHub release
	- [ ] https://dl.gogs.io (also upload `checksum_sha256.txt`)
- [ ] Update content of [Install from binary](https://gogs.io/docs/installation/install_from_binary).

## After release

On the `main` branch:

- [ ] Publish [GitHub security advisories](https://github.com/gogs/gogs/security) for security patches included in the release.
- [ ] Post the following message on issues that are included in the patch milestone:
    ```
    The <MAJOR>.<MINOR>.<PATCH> has been released that includes the patch of the reported issue.
    ```
- [ ] Update the repository mirror on [Gitee](https://gitee.com/unknwon/gogs).
- [ ] Create a new release announcement in [Discussions](https://github.com/gogs/gogs/discussions/categories/announcements).
- [ ] Send a tweet on the [official Twitter account](https://twitter.com/GogsHQ) for the patch release.
- [ ] Close the patch milestone.
