# Git Large File Storage (LFS)

> This document is driven from https://docs.gitlab.com/ee/topics/git/lfs/.

Managing large binaries in Git repositories is challenging, that is why Git LFS was developed for, to manage large files.

## How it works

Git LFS client talks with the Gogs server over HTTP/HTTPS. It uses HTTP Basic Authentication to authorize client requests. Once the request is authorized, Git LFS client receives instructions from where to fetch or where to push the large file.

## Server configuration

Please refer to [Configuring Git Large File Storage (LFS)](../admin/lfs.md).

## Requirements

- Git LFS is supported in Gogs starting with version 0.12.
- [Git LFS client](https://git-lfs.github.com/) version 1.0.1 and up.

## Known limitations

- When SSH is set as a remote, Git LFS objects still go through HTTP/HTTPS.
- Any Git LFS request will ask for HTTP/HTTPS credentials to be provided so a good Git credentials store is recommended.
- File locking is not supported, and is being tracked in [#6064](https://github.com/gogs/gogs/issues/6064).

## Using Git LFS

Git LFS endpoints in a Gogs server can be automatically discovered by the Git LFS client, therefore you do not need to configure anything upfront for using it. Please walk through official [Git LFS Tutorial](https://github.com/git-lfs/git-lfs/wiki/Tutorial) to get started.
