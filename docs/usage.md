gopm
====

gopm(Go Package Manager) is a Go package manage tool for search, install, update, share packages in Go.

usage:

gopm help        show this document
gopm sources     list all package source servers or add or rm a source
gopm list        list all packages local or list all versions of a package
gopm get         get a package or according to a gopmfile
gopm upgrade     upgrade a package or all packages and gopm self
gopm rm          remove a package
gopm search      search a package according keywords
gopm doc         show a package's document on console or web browser
gopm serve       run as a package source server
gopm sync        sync all packages from first avilable source server to local
gopm import      import a package into local
gopm gen         generate a .gopmspec file according current dir's source codes
gopm build       build project according to gopmfile
gopm run         build project according to gopmfile and run
gopm test        test project like go test


.gopmspec file format:
[production]
beego = tag:0.1
xorm >= branch:0.2

[test]
