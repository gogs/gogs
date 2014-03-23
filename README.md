gopm - Go Package Manager [![wercker status](https://app.wercker.com/status/899e79d6738e606dab98c915a269d531/s/ "wercker status")](https://app.wercker.com/project/bykey/899e79d6738e606dab98c915a269d531)
=========================

![GPMGo_Logo](https://raw.github.com/gpmgo/gopmweb/master/static/img/gpmgo.png?raw=true)

Gopm(Go Package Manager) is a Go package manage tool for search, install, update and share packages in Go.

**News** The best IDE for Go development [LiteIDE](https://github.com/visualfc/liteide)(after X20) now has a simple integration of gopm!

**News** Want online cross-platform compile service? Just try [gobuild](http://build.gopm.io) and it won't let you down!

Please see **[Documentation](https://github.com/gpmgo/docs)** before you ever start.

# Commands

```
NAME:
   gopm - Go Package Manager

USAGE:
   gopm [global options] command [command options] [arguments...]

VERSION:
   0.6.5.0320

COMMANDS:
   get		fetch remote package(s) and dependencies to local repository
   bin		download and link dependencies and build executable binary
   gen		generate a gopmfile according current Go project
   run		link dependencies and go run
   build	link dependencies and go build
   install	link dependencies and go install
   update	check and update gopm resources including itself
   config	configurate gopm global settings
   help, h	Shows a list of commands or help for one command
   
GLOBAL OPTIONS:
   --noterm		disable color output
   --version, -v	print the version
   --help, -h		show help
```


