## Before release

On release branch:

- [ ] Make sure all commits are cherry-picked from the develop branch by checking the patch milestone.
- [ ] Update [CHANGELOG](https://github.com/gogs/gogs/blob/main/CHANGELOG.md) to include entries for the current patch release, e.g. `git log v0.12.1...HEAD --pretty=format:'- [ ] %H %s' --reverse`:
	- [ ] _link to the commit_

## During release

On release branch:

- [ ] Update the [hard-coded version](https://github.com/gogs/gogs/blob/main/gogs.go#L21) to the current release, e.g. `0.12.0` -> `0.12.1`.
- [ ] Wait for GitHub Actions to complete and no failed jobs.
- [ ] Publish a new [GitHub release](https://github.com/gogs/gogs/releases) with entries from [CHANGELOG](https://github.com/gogs/gogs/blob/main/CHANGELOG.md) for the current patch release and all previous releases with same minor version. **Make sure the tag is created on the release branch**.
- [ ] Update all previous GitHub releases with same minor version with the warning:
    ```
    **ℹ️ Heads up! There is a new patch release [0.12.1](https://github.com/gogs/gogs/releases/tag/v0.12.1) available, we recommend directly installing or upgrading to that version.**
    ```
- [ ] Wait for a new [Docker Hub tag](https://hub.docker.com/r/gogs/gogs/tags) for the current release to be created automatically.
- [ ] Update Docker image tag for the minor release `<MAJOR>.<MINOR>`, e.g. `0.12`.
- [ ] Compile and pack binaries (all prefixed with `gogs_<MAJOR>.<MINOR>.<PATCH>_`, e.g. `gogs_0.12.0_`):
	- [ ] macOS: `darwin_amd64.zip`
	- [ ] Linux: `linux_386.tar.gz`, `linux_386.zip`, `linux_amd64.tar.gz`, `linux_amd64.zip`
	- [ ] ARM: `linux_armv7.tar.gz`, `linux_armv7.zip`, `linux_armv8.tar.gz`, `linux_armv8.zip`
	- [ ] Windows: `windows_amd64.zip`, `windows_amd64_mws.zip`
- [ ] Generate SHA256 checksum for all binaries to the file `checksum_sha256.txt`.
- [ ] Upload all binaries to:
	- [ ] GitHub release
	- [ ] https://dl.gogs.io (also upload `checksum_sha256.txt`)
- [ ] Build, push and tag a new Docker image for ARM to [Docker Hub](https://hub.docker.com/r/gogs/gogs-rpi).

## After release

On develop branch:

- [ ] Post the following message on issues that are included in the patch milestone:
    ```
    The <MAJOR>.<MINOR>.<PATCH> has been released.
    ```
- [ ] Update the repository mirror on [Gitee](https://gitee.com/unknwon/gogs).
- [ ] Reply to the release topic for the minor release on [Gogs Discussion](https://discuss.gogs.io/c/announcements/5).
- [ ] Close the patch milestone.
