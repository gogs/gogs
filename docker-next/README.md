# Docker for Gogs (Next Generation)

> [!NOTE]
> This is the next-generation, security-focused Docker image. This will become the default image distribution (`gogs/gogs:latest`) starting 0.15.0.

![Docker pulls](https://img.shields.io/docker/pulls/gogs/gogs?logo=docker&style=for-the-badge)

Visit [Docker Hub](https://hub.docker.com/u/gogs) or [GitHub Container registry](https://github.com/gogs/gogs/pkgs/container/gogs) to see all available images and tags.

## Security-first design

This Docker image is designed with Kubernetes security best practices in mind:

- **Runs as non-root by default** - uses UID 1000 and GID 1000
- **Minimal image** - only have essential packages installed
- **Direct execution** - no process supervisor, just runs `gogs web`
- **Supports restrictive security contexts** - ready for Kubernetes

### Kubernetes Security Context example

In the deployment YAML, make sure the following snippets exist:

```yaml
spec:
  template:
    spec:
      securityContext:
        fsGroup: 1000
        fsGroupChangePolicy: OnRootMismatch
      containers:
      - name: gogs
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

### Custom UID/GID at build time

If you need a different UID/GID, build the image with custom arguments:

```zsh
docker build -f Dockerfile.next --build-arg GOGS_UID=1001 --build-arg GOGS_GID=1001 -t my-gogs .
```

## Usage

```zsh
$ docker pull gogs/gogs:next-latest

# Create local directory for volume.
$ mkdir -p /var/gogs
$ chown 1000:1000 /var/gogs

# Use `docker run` for the first time.
$ docker run --name=gogs -p 10022:2222 -p 10880:3000 -v /var/gogs:/data gogs/gogs:next-latest

# Use `docker start` if you have stopped it.
$ docker start gogs
```

Files will be stored in local path `/var/gogs`.

Directory `/var/gogs` keeps Git repositories and Gogs data:

```zsh
/var/gogs
|-- git
    |-- gogs-repositories
|-- gogs
    |-- conf
    |-- data
    |-- log
|-- ssh
```

### Using Docker volumes

```zsh
$ docker volume create --name gogs-data
$ docker run --name=gogs -p 10022:2222 -p 10880:3000 -v gogs-data:/data gogs/gogs:next-latest
```

## Settings

### Application

Most of the settings are obvious and easy to understand, but there are some settings can be confusing by running Gogs inside Docker:

- **Repository Root Path**: either `/data/git/gogs-repositories` or `/home/git/gogs-repositories` works.
- **Run User**: default `git` (UID 1000)
- **Domain**: fill in with Docker container IP (e.g. `192.168.99.100`). But if you want to access your Gogs instance from a different physical machine, please fill in with the hostname or IP address of the Docker host machine.
- **SSH Port**: Use the exposed port from Docker container. For example, your SSH server listens on `2222` inside Docker, **but** you expose it by `10022:2222`, then use `10022` for this value.
- **HTTP Port**: Use port you want Gogs to listen on inside Docker container. For example, your Gogs listens on `3000` inside Docker, **and** you expose it by `10880:3000`, but you still use `3000` for this value.
- **Application URL**: Use combination of **Domain** and **exposed HTTP Port** values (e.g. `http://192.168.99.100:10880/`).

Full documentation of application settings can be found in the [default `app.ini`](https://github.com/gogs/gogs/blob/main/conf/app.ini).

### Git over SSH

>[!IMPORTANT]
> Enable and disable of the builtin SSH server requires restart of the container to take effect.

To enable Git over SSH access, the use of builtin SSH server is required as follows in your `app.ini`:

```ini
[server]
START_SSH_SERVER = true
SSH_PORT         = 10022 # The port shown in the clone URL
SSH_LISTEN_PORT  = 2222  # The port that builtin server listens on
```

## Upgrade

> [!CAUTION]
> Make sure you have volumed data to somewhere outside Docker container!

Steps to upgrade Gogs with Docker:

- `docker pull gogs/gogs:next-latest`
- `docker stop gogs`
- `docker rm gogs`
- Create a container for the first time and don't forget to do the same for the volume and port mapping.
