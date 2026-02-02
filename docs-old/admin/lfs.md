# Configuring Git Large File Storage (LFS)

> NOTE: Git LFS is supported in Gogs starting with version 0.12.

Git LFS works out of box with default configuration for any supported versions.

## Known limitations

- Only local storage is supported (i.e. all LFS objects are stored on the same server where Gogs runs), support of Object Storage Service like Amazon S3 is being tracked in [#6065](https://github.com/gogs/gogs/issues/6065).

## Configuration

All configuration options for Git LFS are located in [`[lfs]` section](https://github.com/gogs/gogs/blob/44ea9604ed7440c2cf1105d965c2429ee225e8f6/conf/app.ini#L266-L270):

```ini
[lfs]
; The storage backend for uploading new objects.
STORAGE = local
; The root path to store LFS objects on local file system.
OBJECTS_PATH = data/lfs-objects
```
