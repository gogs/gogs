# Release a new version

- To release a new minor version, use the GitHub issue template [Dev: Release a minor version](https://github.com/gogs/gogs/issues/new?title=Release+<MAJOR>.<MINOR>.0&labels=%F0%9F%93%B8%20release&template=dev_release_minor_version.md).
- To release a new patch version, use the GitHub issue template [Dev: Release a patch version](https://github.com/gogs/gogs/issues/new?title=Release+<MAJOR>.<MINOR>.<PATCH>&labels=%F0%9F%93%B8%20release&template=dev_release_patch_version.md).

## Playbooks

### Update Docker image tag

1. Pull down images and create a manifest:
	```sh
	$ export VERSION=0.12.4
	$ export MINOR_RELEASE=0.12

	$ docker pull --platform linux/amd64 gogs/gogs:${VERSION}
	$ docker tag gogs/gogs:${VERSION} gogs/gogs:${MINOR_RELEASE}-amd64
	$ docker push gogs/gogs:${MINOR_RELEASE}-amd64
	$ docker pull --platform linux/arm64 gogs/gogs:${VERSION}
	$ docker tag gogs/gogs:${VERSION} gogs/gogs:${MINOR_RELEASE}-arm64
	$ docker push gogs/gogs:${MINOR_RELEASE}-arm64
	$ docker pull --platform linux/arm/v7 gogs/gogs:${VERSION}
	$ docker tag gogs/gogs:${VERSION} gogs/gogs:${MINOR_RELEASE}-armv7
	$ docker push gogs/gogs:${MINOR_RELEASE}-armv7

	$ docker manifest rm gogs/gogs:${MINOR_RELEASE}
	$ docker manifest create \
		gogs/gogs:${MINOR_RELEASE} \
		gogs/gogs:${MINOR_RELEASE}-amd64 \
		gogs/gogs:${MINOR_RELEASE}-arm64 \
		gogs/gogs:${MINOR_RELEASE}-armv7
	$ docker manifest push gogs/gogs:${MINOR_RELEASE}

	# Only push "linux/amd64" for now
	$ echo ${GITHUB_CR_PAT} | docker login ghcr.io -u <USERNAME> --password-stdin
	$ docker tag gogs/gogs:${MINOR_RELEASE}-amd64 ghcr.io/gogs/gogs:${MINOR_RELEASE}
	$ docker push ghcr.io/gogs/gogs:${MINOR_RELEASE}
	```
2. Delete ephemeral tags from the [Docker Hub](https://hub.docker.com/repository/docker/gogs/gogs/tags).

### Compile and pack binaries

All commands are starting at the repository root.

- macOS:
	```sh
	# Produce the ZIP archive
	$ TAGS=cert task release
	```
- Linux:
	```sh
	# Produce the ZIP archive
	$ TAGS="cert pam" task release

	# Produce the Tarball
	$ export VERSION=0.12.4
	$ cd release && tar czf gogs_${VERSION}_linux_$(go env GOARCH).tar.gz gogs
	```
- ARMv7:
	```sh
	# Produce the ZIP archive
	$ TAGS="cert pam" task release

	# Produce the Tarball
	$ export VERSION=0.12.4
	$ cd release && tar czf gogs_${VERSION}_linux_armv7.tar.gz gogs
	```
- ARMv8:
	```sh
	# Produce the ZIP archive
	$ TAGS="cert pam" task release

	# Produce the Tarball
	$ export VERSION=0.12.4
	$ cd release && tar czf gogs_${VERSION}_linux_armv8.tar.gz gogs
	```
- Windows:
	```sh
	$ TAGS=cert task release
	$ TAGS="cert minwinsvc" task release --force
	```
