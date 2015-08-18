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

## SSH Support

In order to support SSH, You need to change `SSH_PORT` in `/var/gogs/gogs/conf/app.ini`:

```
[server]
SSH_PORT = 10022
```

Full documentation of settings can be found [here](http://gogs.io/docs/advanced/configuration_cheat_sheet.html).

## Todo

Install page need support set `SSH_PORT`.

## Troubleshooting

If you see the following error:

```
checkVersion()] [E] Binary and template file version does not match
```

Run `rm -fr /var/gogs/gogs/templates/` should fix this it. Just remember to backup templates file if you have made modifications youself.