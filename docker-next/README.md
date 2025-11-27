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
- **Supports restrictive security contexts**:
  - `runAsNonRoot: true`
  - `allowPrivilegeEscalation: false`
  - `capabilities: { drop: [ALL] }`
- **No OpenSSH** - use Gogs' built-in SSH server instead (configured in `app.ini`)
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
# Pull image from Docker Hub.
$ docker pull gogs/gogs

# Create local directory for volume.
$ mkdir -p /var/gogs

# Use `docker run` for the first time.
$ docker run --name=gogs -p 10880:3000 -v /var/gogs:/data gogs/gogs

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

# Use `docker run` for the first time.
$ docker run --name=gogs -p 10880:3000 -v gogs-data:/data gogs/gogs
```

## SSH Access

This modern image does **not** include OpenSSH. Use Gogs' built-in SSH server instead.

Enable it in your `app.ini`:

```ini
[server]
START_SSH_SERVER = true
SSH_PORT = 2222
```

Then expose port 2222 from your container:

```sh
docker run --name=gogs -p 10022:2222 -p 10880:3000 -v /var/gogs:/data gogs/gogs
```

To clone repositories: `git clone ssh://git@hostname:10022/username/myrepo.git`

## Settings

### Application

- **Repository Root Path**: keep it as default value `/home/git/gogs-repositories` because `start.sh` already made a symbolic link for you.
- **Run User**: keep it as default value `git` because `build/finalize.sh` already setup a user with name `git`.
- **Domain**: fill in with Docker container IP or hostname of the Docker host machine.
- **SSH Port**: If using built-in SSH, use the exposed port (e.g., `10022` if you expose `10022:2222`).
- **HTTP Port**: Use port Gogs listens on inside Docker (default `3000`).
- **Application URL**: Use combination of **Domain** and **exposed HTTP Port** values (e.g. `http://192.168.99.100:10880/`).

Full documentation of application settings can be found [here](https://github.com/gogs/gogs/blob/main/conf/app.ini).

## Upgrade

:exclamation::exclamation::exclamation:<span style="color: red">**Make sure you have volumed data to somewhere outside Docker container**</span>:exclamation::exclamation::exclamation:

Steps to upgrade Gogs with Docker:

- `docker pull gogs/gogs`
- `docker stop gogs`
- `docker rm gogs`
- Finally, create a container for the first time and don't forget to do the same for the volume and port mapping.

## Known issues

- The docker container cannot currently be built on Raspberry 1 (armv6l) as our base image `alpine` does not have a `go` package available for this platform.
