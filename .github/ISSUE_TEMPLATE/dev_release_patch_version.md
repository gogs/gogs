---
name: "Dev: Release a patch version"
about: ONLY USED BY MAINTAINERS.
assignees: "unknwon"
title: "Release [VERSION]"
labels: 📸 release
---

_This is generated from the [patch release template](https://github.com/gogs/gogs/blob/main/.github/ISSUE_TEMPLATE/dev_release_patch_version.md)._

## Before release

On the release branch:

- [ ] Make sure all commits are cherry-picked from the `main` branch by checking the patch milestone.
	- Run `task build` for every cherry-picked commit to make sure there is no compilation error.
- [ ] [Update CHANGELOG on the `main` branch](https://github.com/gogs/gogs/commit/f1102a7a7c545ec221d2906f02fa19170d96f96d) to include entries for the current patch release.

## During release

On the release branch:

- [ ] [Update the hard-coded version](https://github.com/gogs/gogs/commit/f0e3cd90f8d7695960eeef2e4e54b2e717302f6c) to the current release, e.g. `0.12.0` -> `0.12.1`.
- [ ] Wait for GitHub Actions to complete and no failed jobs.
- [ ] Publish new RC releases in [GitHub release](https://github.com/gogs/gogs/releases) (e.g. `v0.12.0-rc.1`, `v0.12.0-rc.2`) ⚠️ **on the release branch** ⚠️ and ensure Docker workflow succeeds.
	- [ ] Pull down the Docker image and [run through application setup](https://github.com/gogs/gogs/blob/main/docker/README.md) to make sure nothing blows up.
	- [ ] Download one of the release archives and run through application setup to make sure nothing blows up.
- [ ] Publish a new [GitHub release](https://github.com/gogs/gogs/releases) ⚠️ **on the release branch** ⚠️ with entries from [CHANGELOG](https://github.com/gogs/gogs/blob/main/CHANGELOG.md) for the current patch release and all previous releases with same minor version.
- [ ] Update all previous GitHub releases with same minor version with the warning:
	```
	**ℹ️ Heads up! There is a new patch release [0.12.1](https://github.com/gogs/gogs/releases/tag/v0.12.1) available, we recommend directly installing or upgrading to that version.**
	```
- [ ] [Wait for new image tags for the current release](https://github.com/gogs/gogs/actions/workflows/docker.yml?query=event%3Arelease) to be created automatically on both [Docker Hub](https://hub.docker.com/r/gogs/gogs/tags) and [GitHub Container registry](https://github.com/gogs/gogs/pkgs/container/gogs).
	- Pull down the Docker image and [run through application setup](https://github.com/gogs/gogs/blob/main/docker/README.md) to make sure nothing blows up.
- [ ] Download all release archives and [generate SHA256 checksum](https://github.com/gogs/gogs/blob/main/docs/dev/release/sha256.sh) for all binaries to the file `checksum_sha256.txt`.
- [ ] Upload all archives and `checksum_sha256.txt` to https://dl.gogs.io.

## After release

On the `main` branch:

- [ ] Post the following message on issues that are included in the patch milestone:
    ```
    The <MAJOR>.<MINOR>.<PATCH> has been released that includes the patch of the reported issue.
    ```
- [ ] Create a new release announcement in [Discussions](https://github.com/gogs/gogs/discussions/categories/announcements).
- [ ] Send a tweet on the [official Twitter account](https://twitter.com/GogsHQ) for the patch release.
- [ ] Close the milestone for the patch release.
- [ ] **After 14 days**, publish [GitHub security advisories](https://github.com/gogs/gogs/security) for security patches included in the release.
