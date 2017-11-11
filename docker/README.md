# Docker for Gogs

Visit [Docker Hub](https://hub.docker.com/r/gogs/) / [Docker Store](https://store.docker.com/community/images/gogs/gogs) see all available images and tags.

## Usage

To keep your data out of Docker container, we do a volume (`/var/gogs` -> `/data`) here, and you can change it based on your situation.

```sh
# Pull image from Docker Hub.
$ docker pull gogs/gogs

# Create local directory for volume.
$ mkdir -p /var/gogs

# Use `docker run` for the first time.
$ docker run --name=gogs -p 10022:22 -p 10080:3000 -v /var/gogs:/data gogs/gogs

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

### Volume With Data Container

If you're more comfortable with mounting data to a data container, the commands you execute at the first time will look like as follows:

```sh
# Create data container
docker run --name=gogs-data --entrypoint /bin/true gogs/gogs

# Use `docker run` for the first time.
docker run --name=gogs --volumes-from gogs-data -p 10022:22 -p 10080:3000 gogs/gogs
```

#### Using Docker 1.9 Volume Command

```sh
# Create docker volume.
$ docker volume create --name gogs-data

# Use `docker run` for the first time.
$ docker run --name=gogs -p 10022:22 -p 10080:3000 -v gogs-data:/data gogs/gogs
```

## Settings

### Application

Most of settings are obvious and easy to understand, but there are some settings can be confusing by running Gogs inside Docker:

- **Repository Root Path**: keep it as default value `/home/git/gogs-repositories` because `start.sh` already made a symbolic link for you.
- **Run User**: keep it as default value `git` because `build.sh` already setup a user with name `git`.
- **Domain**: fill in with Docker container IP (e.g. `192.168.99.100`). But if you want to access your Gogs instance from a different physical machine, please fill in with the hostname or IP address of the Docker host machine.
- **SSH Port**: Use the exposed port from Docker container. For example, your SSH server listens on `22` inside Docker, **but** you expose it by `10022:22`, then use `10022` for this value. **Builtin SSH server is not recommended inside Docker Container**
- **HTTP Port**: Use port you want Gogs to listen on inside Docker container. For example, your Gogs listens on `3000` inside Docker, **and** you expose it by `10080:3000`, but you still use `3000` for this value.
- **Application URL**: Use combination of **Domain** and **exposed HTTP Port** values (e.g. `http://192.168.99.100:10080/`).

Full documentation of application settings can be found [here](https://gogs.io/docs/advanced/configuration_cheat_sheet.html).

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

## Upgrade

:exclamation::exclamation::exclamation:<span style="color: red">**Make sure you have volumed data to somewhere outside Docker container**</span>:exclamation::exclamation::exclamation:

Steps to upgrade Gogs with Docker:

- `docker pull gogs/gogs`
- `docker stop gogs`
- `docker rm gogs`
- Finally, create container as the first time and don't forget to do same volume and port mapping.

## Known Issues

- The docker container can not currently be build on Raspberry 1 (armv6l) as our base image `alpine` does not have a `go` package available for this platform.

## Useful Links

- [Share port 22 between Gogs inside Docker & the local system](http://www.ateijelo.com/blog/2016/07/09/share-port-22-between-docker-gogs-ssh-and-local-system)
