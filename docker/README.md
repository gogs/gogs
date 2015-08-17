# Docker for Gogs

## Usage

```
docker pull gogs/gogs

mkdir -p /var/gogs
docker run --name=gogs -p 10022:22 -p 10080:3000 -v /var/gogs:/data gogs/gogs
```

File will store in local path: `/var/gogs`.

Directory `/var/gogs` keeps Git repoistories and Gogs data:

    /var/gogs
    |-- git
    |   `-- gogs-repositories
    |-- ssh
    |    `-- # ssh pub-pri keys for gogs
    `---- gogs
        |-- conf
        |-- data
        |-- log
        `-- templates

## SSH Support

In order to support SSH, You need to change `HTTP_PORT` and `SSH_PORT` in `/var/gogs/gogs/conf/app.ini`:

```
[server]
HTTP_PORT = 3000
SSH_PORT = 10022
```

Full documentation of settings can be found [here](http://gogs.io/docs/advanced/configuration_cheat_sheet.html).
