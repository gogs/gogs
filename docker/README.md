# Docker for gogs

## Usage
```
docker pull codeskyblue/docker-gogs

mkdir -p /var/gogs
docker run --name=gogs -d -p 10022:22 -p 10080:3000 -v /var/gogs:/data codeskyblue/docker-gogs
```

File will store in local path: `/var/gogs`

Directory `/var/gogs` keeps git repos and gogs data

    /var/gogs
    ├── git
    │   └── gogs-repositories
    |-- ssh
    |    `-- # ssh pub-pri keys for gogs
    └── gogs
        ├── conf
        ├── data
        ├── log
        └── templates

