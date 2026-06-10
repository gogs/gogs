# Docker for Gogs

> [!WARNING]
> This is now the legacy Docker image that lacks modern security best practices. It will be published as `gogs/gogs:legacy-latest` starting 0.16.0, and be completely removed no earlier than 0.17.0.
>
> To use the next-generation, security-focused Docker image, see [docker-next/README.md](../docker-next/README.md).

> [!IMPORTANT]
> Image versions:
>  - Every released version has its own tag , e.g., `gogs/gogs:0.13.4`, and a tag points to the latest patch of the minor version, e.g., `gogs/gogs:0.13`.
>  - The `latest` tag is the image version built from the latest `main` branch.

![Docker pulls](https://img.shields.io/docker/pulls/gogs/gogs?logo=docker&style=for-the-badge)

Visit [Docker Hub](https://hub.docker.com/u/gogs) or [GitHub Container registry](https://github.com/gogs/gogs/pkgs/container/gogs) to see all available images and tags.

## Configuration

Gogs requires an `app.ini` before it can start, which overlays the shipped defaults, so it only needs the keys you actually want to change. For example, serving Gogs at `https://gogs.example.com` with the container's HTTP port mapped to host port `10880` and SSH port mapped to `10022`, backed by a Postgres on the Docker host:

```ini
RUN_MODE = prod
; USE AS-IS because the image already created this user.
RUN_USER = git

[server]
EXTERNAL_URL = https://gogs.example.com/
DOMAIN       = gogs.example.com
; The port exposed by `docker run -p 10022:22`. Shown in clone URLs.
SSH_PORT     = 10022
; The builtin SSH server is not supported inside legacy Docker.
START_SSH_SERVER = false

[repository]
; USE AS-IS to match the data volume layout shipped in the image.
ROOT = /home/git/gogs-repositories

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

### Bind mount

Write `app.ini` to the host directory, then `docker run` with `-v`:

```zsh
$ mkdir -p /var/gogs/gogs/conf
$ vi /var/gogs/gogs/conf/app.ini   # Paste the example above and edit
$ docker run --name=gogs -p 10022:22 -p 10880:3000 -v /var/gogs:/data gogs/gogs
```

### Named volume

A named volume lives inside Docker's storage, so the file has to be written through a container. Gogs refuses to start without an `app.ini`, so the container must be **created** (not run) first:

```zsh
$ docker volume create --name gogs-data
$ docker create --name=gogs -p 10022:22 -p 10880:3000 -v gogs-data:/data gogs/gogs
$ docker cp app.ini gogs:/data/gogs/conf/app.ini
$ docker start gogs
```

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

### Container options

This container has some options available via environment variables, these options are opt-in features that can help the administration of this container:

- **SOCAT_LINK**:
  - <u>Possible value:</u>
      `true`, `false`, `1`, `0`
  - <u>Default:</u>
      `true`
  - <u>Action:</u>
      Bind linked docker container to localhost socket using socat.
      Any exported port from a linked container will be binded to the matching port on localhost.
  - <u>Disclaimer:</u>
      As this option rely on the environment variable created by docker when a container is linked, this option should be deactivated in managed environment such as Rancher or Kubernetes (set to `0` or `false`)
- **RUN_CROND**:
  - <u>Possible value:</u>
      `true`, `false`, `1`, `0`
  - <u>Default:</u>
      `false`
  - <u>Action:</u>
      Request crond to be run inside the container. Its default configuration will periodically run all scripts from `/etc/periodic/${period}` but custom crontabs can be added to `/var/spool/cron/crontabs/`.
- **BACKUP_INTERVAL**:
  - <u>Possible value:</u>
      `3h`, `7d`, `3M`
  - <u>Default:</u>
      `null`
  - <u>Action:</u>
      In combination with `RUN_CROND` set to `true`, enables backup system.\
      See: [Backup System](#backup-system)
- **BACKUP_RETENTION**:
  - <u>Possible value:</u>
      `360m`, `7d`, `...m/d`
  - <u>Default:</u>
      `7d`
  - <u>Action:</u>
      Used by backup system. Backups older than specified in expression are deleted periodically.\
      See: [Backup System](#backup-system)
- **BACKUP_ARG_CONFIG**:
  - <u>Possible value:</u>
      `/app/gogs/example/custom/config`
  - <u>Default:</u>
      `null`
  - <u>Action:</u>
      Used by backup system. If defined, supplies `--config` argument to `gogs backup`.\
      See: [Backup System](#backup-system)
- **BACKUP_ARG_EXCLUDE_REPOS**:
  - <u>Possible value:</u>
      `test-repo1`, `test-repo2`
  - <u>Default:</u>
      `null`
  - <u>Action:</u>
      Used by backup system. If defined, supplies `--exclude-repos` argument to `gogs backup`.\
      See: [Backup System](#backup-system)
- **BACKUP_EXTRA_ARGS**:
  - <u>Possible value:</u>
      `--verbose --exclude-mirror-repos`
  - <u>Default:</u>
      `null`
  - <u>Action:</u>
      Used by backup system. If defined, append content to arguments to `gogs backup`.\
      See: [Backup System](#backup-system)

## Backup system

Automated backups with retention policy:

- `BACKUP_INTERVAL` controls how often the backup job runs and supports interval in hours (h), days (d), and months (M), eg. `3h`, `7d`, `3M`. The lowest possible value is one hour (`1h`).
- `BACKUP_RETENTION` supports expressions in minutes (m) and days (d), eg. `360m`, `2d`. The lowest possible value is 60 minutes (`60m`).

## Upgrade

> [!CAUTION]
> Make sure you have volumed data to somewhere outside Docker container!

Steps to upgrade Gogs with Docker:

- `docker pull gogs/gogs`
- `docker stop gogs`
- `docker rm gogs`
- Create a container for the first time and don't forget to do the same for the volume and port mapping.

## Known issues

- The docker container cannot currently be built on Raspberry 1 (armv6l) as our base image `alpine` does not have a `go` package available for this platform.

## Useful links

- [Share port 22 between Gogs inside Docker & the local system](http://www.ateijelo.com/blog/2016/07/09/share-port-22-between-docker-gogs-ssh-and-local-system)
