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

## Settings

Most of settings are obvious and easy to understand, but there are some settings can be confusing by running Gogs inside Docker:

- **Repository Root Path**: keep it as default value `/home/git/gogs-repositories` because `start.sh` already made a symbolic link for you.
- **Run User**: keep it as default value `git` because `start.sh` already setup a user with name `git`.
- **Domain**: fill in with Docker container IP(e.g. `192.168.99.100`).
- **SSH Port**: Use the exposed port from Docker container. For example, your SSH server listens on `22` inside Docker, but you expose it by `10022:22`, then use `10022` for this value.
- **HTTP Port**: Use port you want Gogs to listen on inside Docker container. For example, your Gogs listens on `3000` inside Docker, and you expose it by `10080:3000`, but you still use `3000` for this value.
- **Application URL**: Use combination of **Domain** and **exposed HTTP Port** values(e.g. `http://192.168.99.100:10080/`). 

Full documentation of settings can be found [here](http://gogs.io/docs/advanced/configuration_cheat_sheet.html).

## Troubleshooting

If you see the following error:

```
checkVersion()] [E] Binary and template file version does not match
```

Run `rm -fr /var/gogs/gogs/templates/` should fix this it. Just remember to backup templates file if you have made modifications youself.

## Known Issues

- [Use ctrl+c when clone through SSH makes Docker exit unexpectedly](https://github.com/gogits/gogs/issues/1499)