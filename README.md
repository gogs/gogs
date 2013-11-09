gopm - Go Package Manager
=========================

![GPMGo_Logo](https://raw.github.com/gpmgo/gopmweb/master/static/img/gpmgo.png?raw=true)

Gopm(Go Package Manager) is a Go package manage tool for search, install, update and share packages in Go.

Current Version: **v0.5.1**

# Requirement

- Go Development Environment >= 1.1.
- Command `ln -s` support on Mac OS and Unix-like systems.
- Command `mklink -j` support on Windows( **Windows Vista and later** ).

# Installation

Because we do NOT offer binaries for now, so before you install the gopm, you should have already installed Go Development Environment with version 1.1 and later.

```
go get github.com/gpmgo/gopm
```

The executable will be produced under `$GOPATH/bin` in your file system; for global use purpose, we recommand you to add this path into your `PATH` environment variable.

# Features

- No requirement for installing any version control system tool like `git`, `svn` or `hg` in order to download packages(although you have to install git for installing gopm though `go get` for now).
- Download, install or build your packages with specific revisions.
- When build program with `gopm build` or `gopm install`, everything just happen in its own GOPATH and do not bother anything you've done.
* Put your Go project on anywhere you want.

# Commands

```
NAME:
   gopm - Go Package Manager

USAGE:
   gopm [global options] command [command options] [arguments...]

VERSION:
   0.5.2.1109

COMMANDS:
   get		fetch remote package(s) and dependencies to local repository
   gen		generate a gopmfile according current go project
   help, h	Shows a list of commands or help for one command

GLOBAL OPTIONS:
   --version	print the version
   --help, -h	show help
```


