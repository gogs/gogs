# Docker for Gogs (Next Generation)

> [!NOTE]
> This is the next-generation, security-focused Docker image. This will become the default image distribution (`gogs/gogs:latest`) starting 0.16.0.

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

## Configuration

Gogs requires an `app.ini` before it can start, which overlays the shipped defaults, so it only needs the keys you actually want to change. For example, serving Gogs at `https://gogs.example.com` with the container's HTTP port mapped to host port `10880` and SSH port mapped to `10022`, backed by a Postgres on the Docker host:

```ini
RUN_MODE = prod
; USE AS-IS because the image already created this user with UID 1000.
RUN_USER = git

[server]
EXTERNAL_URL = https://gogs.example.com/
DOMAIN       = gogs.example.com

[repository]
; USE AS-IS to match the data volume layout shipped in the image.
ROOT = /data/git/gogs-repositories

[database]
TYPE     = postgres
; Use host.docker.internal (or the host's LAN IP) to reach a DB on the host.
HOST     = host.docker.internal:5432
NAME     = gogs
USER     = gogs
PASSWORD = ${GOGS_DATABASE_PASSWORD}

[security]
SECRET_KEY = ${GOGS_SECURITY_SECRET_KEY}
```

`SECRET_KEY` can be any unguessable string, e.g., a UUID from [uuidgenerator.net](https://www.uuidgenerator.net). Gogs refuses to start while it is still using the unsafe default.

> [!NOTE]
> `${...}` references in any value expand from the container's environment at startup, which keeps secrets out of `app.ini` on disk. Pass them with `docker run -e GOGS_DATABASE_PASSWORD=… -e GOGS_SECURITY_SECRET_KEY=…` (or `--env-file secrets.env`). Plain literal values work too, e.g., `PASSWORD = hunter2`.

See [configuration primer](https://gogs.io/fine-tuning/configuration-primer) to learn more about how the configuration system works.

### Git over SSH

>[!IMPORTANT]
> Enable and disable of the builtin SSH server requires restart of the container to take effect.

To enable Git over SSH access, the use of builtin SSH server is required as follows in your `app.ini`:

```ini
[server]
START_SSH_SERVER = true
; The port exposed by `docker run -p 10022:2222`. Shown in clone URLs.
SSH_PORT         = 10022
; The port that the builtin SSH server listens on.
SSH_LISTEN_PORT  = 2222
```

### Bind mount

Write `app.ini` to the host directory, then `docker run` with `-v`:

```zsh
$ mkdir -p /var/gogs/gogs/conf
$ chown -R 1000:1000 /var/gogs
$ vi /var/gogs/gogs/conf/app.ini   # Paste the example above and edit
$ docker run --name=gogs -p 10022:2222 -p 10880:3000 -v /var/gogs:/data gogs/gogs:next-latest
```

### Named volume

A named volume lives inside Docker's storage, so the file has to be written through a container. Gogs refuses to start without an `app.ini`, so the container must be **created** (not run) first:

```zsh
$ docker volume create --name gogs-data
$ docker create --name=gogs -p 10022:2222 -p 10880:3000 -v gogs-data:/data gogs/gogs:next-latest
$ docker cp app.ini gogs:/data/gogs/conf/app.ini
$ docker run --rm -v gogs-data:/data alpine chown -R 1000:1000 /data/gogs/conf
$ docker start gogs
```

The `chown` step exists because the non-root image runs as UID 1000 but `docker cp` writes as root.

### Custom directory

The "custom" directory may not be obvious in Docker environment. The `/data/gogs` (in the container) is already the "custom" directory. You don't need to create another layer, edit files there directly.

Directory layout inside Docker container:

```
/data
|-- git
|   |-- gogs-repositories
|-- ssh
|   |-- # ssh public/private keys for Gogs
|-- gogs
    |-- conf
    |-- data
    |-- log
```

## First start

Open `https://gogs.example.com/` and sign up. Whoever signs up while there are no other users becomes the admin.

Alternatively, the admin user can be created from the command line. The command runs inside the container so it reaches the database through the same configuration:

```zsh
$ docker exec -it gogs gogs admin create-user \
    --name admin \
    --password ${PASSWORD} \
    --email admin@example.com \
    --admin \
    --config /data/gogs/conf/app.ini
```

Once Gogs is running, use `docker start gogs` / `docker stop gogs` for subsequent restarts.

## Upgrade

> [!CAUTION]
> Make sure you have volumed data to somewhere outside Docker container!

Steps to upgrade Gogs with Docker:

- `docker pull gogs/gogs:next-latest`
- `docker stop gogs`
- `docker rm gogs`
- Create a container for the first time and don't forget to do the same for the volume and port mapping.
