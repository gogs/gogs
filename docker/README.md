# Docker for Gogs

Visit [Docker Hub](https://hub.docker.com/r/gogs/gogs/) see all available tags.

## Usage

To keep your data out of Docker container, we do a volume(`/var/gogs` -> `/data`) here, and you can change it based on your situation.

```
# Pull image from Docker Hub.
$ docker pull gogs/gogs

# Create local directory for volume.
$ mkdir -p /var/gogs

# Use `docker run` for the first time.
$ docker run --name=gogs -p 10022:22 -p 10080:3000 -v /var/gogs:/data gogs/gogs

# Use `docker start` if you have stopped it.
$ docker start gogs
```

Files will be store in local path `/var/gogs` in my case.

Directory `/var/gogs` keeps Git repoistories and Gogs data:

    /var/gogs
    |-- git
    |   |-- gogs-repositories
    |-- ssh
    |   |-- # ssh public/private keys for Gogs
    |-- gogs
        |-- conf
        |-- data
        |-- log
        |-- templates

### Volume with data container

If you're more comfortable with mounting data to a data container, the commands you execute at the first time will look like as follows:

```
# Create data container
docker run --name=gogs-data --entrypoint /bin/true gogs/gogs
# Use `docker run` for the first time.
docker run --name=gogs --volumes-from gogs-data -p 10022:22 -p 10080:3000 gogs/gogs
```

## Settings

Most of settings are obvious and easy to understand, but there are some settings can be confusing by running Gogs inside Docker:

- **Repository Root Path**: keep it as default value `/home/git/gogs-repositories` because `start.sh` already made a symbolic link for you.
- **Run User**: keep it as default value `git` because `start.sh` already setup a user with name `git`.
- **Domain**: fill in with Docker container IP(e.g. `192.168.99.100`). But if you want to access your Gogs instance from a different physical machine, please fill in with the hostname or IP address of the Docker host machine.
- **SSH Port**: Use the exposed port from Docker container. For example, your SSH server listens on `22` inside Docker, but you expose it by `10022:22`, then use `10022` for this value.
- **HTTP Port**: Use port you want Gogs to listen on inside Docker container. For example, your Gogs listens on `3000` inside Docker, and you expose it by `10080:3000`, but you still use `3000` for this value.
- **Application URL**: Use combination of **Domain** and **exposed HTTP Port** values(e.g. `http://192.168.99.100:10080/`).

Full documentation of settings can be found [here](http://gogs.io/docs/advanced/configuration_cheat_sheet.html).

## Upgrade

:exclamation::exclamation::exclamation:<span style="color: red">**Make sure you have volumed data to somewhere outside Docker container**</span>:exclamation::exclamation::exclamation:

Steps to upgrade Gogs with Docker:

- `docker pull gogs/gogs`
- `docker stop gogs`
- `docker rm gogs`
- Finally, create container as the first time and don't forget to do same volume and port mapping.

## Known Issues

- `.dockerignore` seems to be ignored during Docker Hub Automated build
