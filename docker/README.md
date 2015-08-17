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

## Todo
Install page need support set `SSH_PORT`

## Problems

If meet error

```
checkVersion()] [E] Binary and template file version does not match
```

Run `rm -fr /var/gogs/gogs/templates/` will fix. Remember to backup templates file, If you have some modification.
