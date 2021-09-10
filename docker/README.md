# Docker for Gogs

![Docker pulls](https://img.shields.io/docker/pulls/gogs/gogs?logo=docker&style=for-the-badge) ![Docker image size](https://img.shields.io/microbadger/image-size/gogs/gogs?logo=docker&style=for-the-badge)

Visit [Docker Hub](https://hub.docker.com/u/gogs) see all available images and tags.

## Usage

ℹ️ Please use `gogs/gogs-rpi` if you're using Raspberry Pis!

To keep your data out of Docker container, we do a volume (`/var/gogs` -> `/data`) here, and you can change it based on your situation.

```sh
# Pull image from Docker Hub.
$ docker pull gogs/gogs

# Create local directory for volume.
$ mkdir -p /var/gogs

# Use `docker run` for the first time.
$ docker run --name=gogs -p 10022:22 -p 10880:3000 -v /var/gogs:/data gogs/gogs

# Use `docker start` if you have stopped it.
$ docker start gogs
```

Note: It is important to map the Gogs ssh service from the container to the host and set the appropriate SSH Port and URI settings when setting up Gogs for the first time. To access and clone Gogs Git repositories with the above configuration you would use: `git clone ssh://git@hostname:10022/username/myrepo.git` for example.

Files will be store in local path `/var/gogs` in my case.

Directory `/var/gogs` keeps Git repositories and Gogs data:

    /var/gogs
    |-- git
    |   |-- gogs-repositories
    |-- ssh
    |   |-- # ssh public/private keys for Gogs
    |-- gogs
        |-- conf
        |-- data
        |-- log

#### Custom Directory

The "custom" directory may not be obvious in Docker environment. The `/var/gogs/gogs` (in the host) and `/data/gogs` (in the container) is already the "custom" directory and you do not need to create another layer but directly edit corresponding files under this directory.

### Volume With Data Container

If you're more comfortable with mounting data to a data container, the commands you execute at the first time will look like as follows:

```sh
# Create data container
docker run --name=gogs-data --entrypoint /bin/true gogs/gogs

# Use `docker run` for the first time.
docker run --name=gogs --volumes-from gogs-data -p 10022:22 -p 10880:3000 gogs/gogs
```

#### Using Docker 1.9 Volume Command

```sh
# Create docker volume.
$ docker volume create --name gogs-data

# Use `docker run` for the first time.
$ docker run --name=gogs -p 10022:22 -p 10880:3000 -v gogs-data:/data gogs/gogs
```

## Settings

### Application

Most of settings are obvious and easy to understand, but there are some settings can be confusing by running Gogs inside Docker:

- **Repository Root Path**: keep it as default value `/home/git/gogs-repositories` because `start.sh` already made a symbolic link for you.
- **Run User**: keep it as default value `git` because `finalize.sh` already setup a user with name `git`.
- **Domain**: fill in with Docker container IP (e.g. `192.168.99.100`). But if you want to access your Gogs instance from a different physical machine, please fill in with the hostname or IP address of the Docker host machine.
- **SSH Port**: Use the exposed port from Docker container. For example, your SSH server listens on `22` inside Docker, **but** you expose it by `10022:22`, then use `10022` for this value. **Builtin SSH server is not recommended inside Docker Container**
- **HTTP Port**: Use port you want Gogs to listen on inside Docker container. For example, your Gogs listens on `3000` inside Docker, **and** you expose it by `10880:3000`, but you still use `3000` for this value.
- **Application URL**: Use combination of **Domain** and **exposed HTTP Port** values (e.g. `http://192.168.99.100:10880/`).

Full documentation of application settings can be found [here](https://github.com/gogs/gogs/blob/main/conf/app.ini).

### Container Options

This container have some options available via environment variables, these options are opt-in features that can help the administration of this container:

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

## Backup System
Automated backups with retention policy:

- `BACKUP_INTERVAL` controls how often the backup job runs and supports interval in hours (h), days (d), and months (M), eg. `3h`, `7d`, `3M`. The lowest possible value is one hour (`1h`).
- `BACKUP_RETENTION` supports expressions in minutes (m) and days (d), eg. `360m`, `2d`. The lowest possible value is 60 minutes (`60m`).

## Upgrade

:exclamation::exclamation::exclamation:<span style="color: red">**Make sure you have volumed data to somewhere outside Docker container**</span>:exclamation::exclamation::exclamation:

Steps to upgrade Gogs with Docker:

- `docker pull gogs/gogs`
- `docker stop gogs`
- `docker rm gogs`
- Finally, create a container for the first time and don't forget to do the same for the volume and port mapping.

## Known Issues

- The docker container cannot currently be built on Raspberry 1 (armv6l) as our base image `alpine` does not have a `go` package available for this platform.

## Useful Links

- [Share port 22 between Gogs inside Docker & the local system](http://www.ateijelo.com/blog/2016/07/09/share-port-22-between-docker-gogs-ssh-and-local-system)
