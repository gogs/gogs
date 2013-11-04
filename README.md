gopm - Go Package Manager
=========================

![GPMGo_Logo](https://raw.github.com/gpmgo/gopmweb/master/static/img/gpmgo.png?raw=true)

Gopm(Go Package Manager) is a Go package manage tool for search, install, update and share packages in Go.

**Attention** This application still in experiment, we'are working on new break version, you may use [old version](https://github.com/gpmgo/gopm/tree/v0.1.0) for now.

# Requirement

Currently, gopm use soft symblink `ln -s` on Unix-like OS and `mklink -j` on Windows.
Make sure that you have the command.

# Install

You should install Go and Go tool before install gopm currently.

```
go get github.com/gpmgo/gopm
```

This will install gopm on $GOPATH$/bin。Before using gopm, you should add this to $PATH.

# Features

* Don't need to install git, svn, hg etc. for installing packages.
* Package has version
* Every project has own GOPATH
* Put your Go project on anywhere you want

# Commands

1. Show the command help
```
gopm help
```

2. Show gopm version
```
gopm version
```

3. Get a package
```
gopm get github.com/gpmgo/gopm
```

4. Search a package
```
gopm search gopm
```

5. Build a project, the build's arguments are the same as go build. But it will check all the dependencies and dowload them.
```
<change to project directory>
gopm build
```

6. Run a go file
```
<change to project directory>
gopm run main.go
```

7. Install 
```
<change to project directory>
gopm install
```


