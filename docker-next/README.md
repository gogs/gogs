# Docker for Gogs (Next Generation)

![Docker pulls](https://img.shields.io/docker/pulls/gogs/gogs?logo=docker&style=for-the-badge)

Visit [Docker Hub](https://hub.docker.com/u/gogs) or [GitHub Container registry](https://github.com/gogs/gogs/pkgs/container/gogs) to see all available images and tags.

> **Note:** This is the next-generation, security-focused Docker image. For the legacy image with OpenSSH support, see [docker/README.md](../docker/README.md).

## Build

```sh
docker build -f Dockerfile.next -t gogs-next .
```

## Security-first Design

This Docker image is designed with Kubernetes security best practices in mind:

- **Runs as non-root by default** (UID 1000, GID 1000)
- **Minimal image** - only essential packages (git, ca-certificates, tzdata)
- **Direct execution** - no process supervisor, just runs `gogs web`
- **Supports restrictive security contexts**:
  - `runAsNonRoot: true`
  - `allowPrivilegeEscalation: false`
  - `capabilities: { drop: [ALL] }`
- **Built-in SSH** - use Gogs' built-in SSH server (configured in `app.ini`)
- **Fixed UID/GID** at build time for predictable permissions

### Kubernetes Security Context Example

```yaml
securityContext:
  runAsNonRoot: true
  runAsUser: 1000
  runAsGroup: 1000
  allowPrivilegeEscalation: false
  seccompProfile:
    type: RuntimeDefault
  capabilities:
    drop:
      - ALL
```

### Custom UID/GID at Build Time

If you need a different UID/GID, build the image with custom arguments:

```sh
docker build -f Dockerfile.next --build-arg GOGS_UID=1001 --build-arg GOGS_GID=1001 -t my-gogs .
```

## Usage

```sh
# Build the next-gen image
$ docker build -f Dockerfile.next -t gogs-next .

# Create local directory for volume.
$ mkdir -p /var/gogs
$ chown 1000:1000 /var/gogs

# Run the container
$ docker run --name=gogs -p 10022:22 -p 10880:3000 -v /var/gogs:/data gogs-next

# Use `docker start` if you have stopped it.
$ docker start gogs
```

Files will be stored in local path `/var/gogs`.

Directory `/var/gogs` keeps Git repositories and Gogs data:

    /var/gogs
    |-- git
    |   |-- gogs-repositories
    |-- gogs
        |-- conf
        |-- data
        |-- log

### Using Docker volumes

```sh
# Create docker volume.
$ docker volume create --name gogs-data

# Run the container
$ docker run --name=gogs -p 10022:22 -p 10880:3000 -v gogs-data:/data gogs-next
```

## SSH Access

Use Gogs' built-in SSH server. Enable it in your `app.ini`:

```ini
[server]
START_SSH_SERVER = true
SSH_PORT = 22
```

The container exposes port 22 for SSH access. To clone repositories:
`git clone ssh://git@hostname:10022/username/myrepo.git`

## Settings

### Application

- **Repository Root Path**: default `/home/git/gogs-repositories`
- **Run User**: default `git` (UID 1000)
- **Domain**: Docker container IP or hostname of the Docker host machine
- **SSH Port**: If using built-in SSH, use the exposed port (e.g., `10022` if you expose `10022:22`)
- **HTTP Port**: Port Gogs listens on inside Docker (default `3000`)
- **Application URL**: Combination of **Domain** and **exposed HTTP Port** (e.g. `http://192.168.99.100:10880/`)

Full documentation of application settings can be found [here](https://github.com/gogs/gogs/blob/main/conf/app.ini).

## Upgrade

:exclamation::exclamation::exclamation:<span style="color: red">**Make sure you have volumed data to somewhere outside Docker container**</span>:exclamation::exclamation::exclamation:

Steps to upgrade Gogs with Docker:

- Rebuild the image: `docker build -f Dockerfile.next -t gogs-next .`
- `docker stop gogs`
- `docker rm gogs`
- Create a new container with the same volume and port mapping

## Known issues

- The docker container cannot currently be built on Raspberry 1 (armv6l) as our base image `alpine` does not have a `go` package available for this platform.
