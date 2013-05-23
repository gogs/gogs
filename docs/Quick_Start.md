# Quick Start

**Attention** Features like bundle and snapshot have NOT been published for users.

Full documentation please visit [GPMGo Documentation]().

## Index

- [When and why](#when-and-why)
- [Installation](#installation)
- [ **Build** your first project](#build-your-first-project)
- [ Download and **install** package, or packages](#download-and-install-package,-or-packages)
- [ **Remove** package, or packages](#remove-package,-or-packages)

## When and why

- No version control tool are installed, too lazy to have it? 
	
	Go get gpm!

- Killer feature over `go get`? 

	There is almost nothing better than `go get` until we make feature bundle and snapshot be available to you.

	

## Installation

You can install either from source or download binary. 

### Install from source

- gpm is a `go get` able project: execute command `go get github.com/GPMGo/gpm` to download and install.
- Run test: switch work directory to gpm project, and execute command `go test` to build and test commands automatically(for now, tested commands are `gpm install`, `gpm remove`).
- Add gpm project path to your environment variable `PATH` in order to execute it from other directories.

### Download binary

Because we don't have all kinds of operating systems, we need your help to complete following download list!(I'm just too lazy to cross compiling -_-|||)

- darwin-386:
- darwin-amd64:
- freebsd-386:
- freebsd-amd64:
- linux-386:
- linux-amd64:
- windows_386:
- windows_amd64: [gpm0.1.5 Build 0523](https://docs.google.com/file/d/0B2GBHFyTK2N8Y241eUlKd01Ia1U/edit?usp=sharing)

**Attention** Because we use API to get information of packages that are hosted on github.com, but it limits 60 requests per hour, so you may get errors if you download too much(more than 50 packages per hour). We do not provider access token for security reason, but we do have configure option `github_access_token` in configuration file `conf/gpm.toml`, so you can go to [here](https://github.com/settings/applications) and create your personal access token(up to 5000 request per hour), and set it in `gpm.toml`.

## Build your first project

Command `build` compiles and installs packages along with all dependencies.

Suppose you have a project called `github.com/GPMGo/gpm`.

- Switch to corresponding directory: `cd $GOPATH/src/github.com/GPMGo/gpm`.
- Execute command `gpm build`.
- Then, gpm calls `go install` in underlying, so you should have binary `$GOPATH/bin/gpm`.
- gpm moves binary from corresponding GOPATH to current which is `$GOPATH/src/github.com/GPMGo/` in this case, now just run your application.

### Why we do this?

In some cases like building web applications, we use relative path to access static files, and `go build` compiles packages without saving, so it's a shortcut for `go install` + `go build`, and you don't need to compile packages which have not changed again.

Also, you can use all flags that are used for `go install`.

## Download and install package, or packages

Command `install` downloads and installs packages along with all dependencies(except when you use bundle or snapshot).

Suppose you want to install package `bitbucket.org/zombiezen/gopdf/pdf`.

- Execute command `gpm install -p bitbucket.org/zombiezen/gopdf/pdf`, flag `-p` means **pure download** (download packages without version control), so you do not need to install version control tool. In case you want to, `gpm install bitbucket.org/zombiezen/gopdf/pdf` calls `go get` in underlying.
- gpm tells your which GOPATH will be used for saving packages, and it checks your current execute path to get best matched path in your GOPATH environment variable.

## Remove package, or packages

Command `remove` removes packages from your local file system(except when you use bundle or snapshot).

Suppose you want to remove package `bitbucket.org/zombiezen/gopdf/pdf`.

- Execute command `gpm remove bitbucket.org/zombiezen/gopdf/pdf`, gpm finds this project in all paths in your GOPATH environment.
- You may notice this is not project path, it's OK because gpm knows it, and deletes directory `$GOPATH/src/bitbucket.org/zombiezen/gopdf/`.