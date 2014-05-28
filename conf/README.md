## NOTICE

This directory only used for development, and us [go-bindata](https://github.com/jteeuwen/go-bindata) to store in memory for releases.

To apply any change in this directory, install [go-bindata](https://github.com/jteeuwen/go-bindata), and then execute following command in root of source directory:

```
$ go-bindata -ignore="\\.DS_Store|README.md" -o modules/bin/conf.go -pkg="bin" conf/...
```