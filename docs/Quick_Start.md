# Quick Start

Full documentation please visit [GPMGo Documentation]()(Haven't done yet!).

## Index

- [When and why](#when-and-why)
- [Installation](#installation)
- [ **Install** package, or packages](#install-package-or-packages)
- [ **Build** and run it](#build-and-run-it)
- [ **Remove** package, or packages](#remove-package-or-packages)
- [ Use **check** to check dependencies](#use-check-to-check-dependencies)
- [ **Search** and find more](#search-and-find-more)

## When and why

### Lightweight version control

Unlike large version control system like git, hg, or svn, you don't have to install any version control tool for using gpm; you are still able to download and install packages that you prefer to.

### Not only project, but dependencies!
	
With gpm, it's much easier to control dependencies version of your packages specifically. All you need to do is that indicate version either by tag, branch or commit of your dependencies, and leave rest of work to gpm!

### Killer feature over `go get`? 

- `go get` gives great advantages of package installation in Go, but the only thing it's missing is version control of dependencies.
- Every time you use `go get`, you may download unstable version of your package dependencies, and you may waste your time to find last version in almost unreadable commit history.
- Not only main package, dependencies also have their dependencies, in a big project, small things like this should not waste your attention for building awesome applications.

### How's configuration file looks like?

In gpm, we call `bundle` for this kind of files, here is an example of a [bundle](https://github.com/GPMGo/gpm/blob/master/repo/bundles/test_bundle.json), don't get it? It's fine, we'll talk about it more just one second.

## Installation

You can install gpm either from source or download binary. 

### Install from source

- gopm is a `go get` able project: execute command `go get github.com/GPMGo/gopm` to download and install.
- Run test: switch work directory to gopm project, and execute command `go test` to build and test commands automatically(for now, tested commands are `gopm install`, `gopm remove`).
- Add gopm project path to your environment variable `PATH` in order to execute it in other directories.

**Attention** You can actually put binary in any path that has already existed in $PATH, so you don't need to add a new path to $PATH again.

### Download binary

At this time, we recommend you install from source.

Because we don't have all kinds of operating systems, we need your help to complete following download list!(I'm just too lazy to cross compiling -_-|||)

- darwin-386:
- darwin-amd64:
- freebsd-386:
- freebsd-amd64:
- linux-386:
- linux-amd64:
- windows_386:
- windows_amd64: 

**Attention** Because we use API to get information of packages that are hosted on github.com, but it limits 60 requests per hour, so you may get errors if you download too much(more than 50 packages per hour). We do not provider access token for security reason, but we do have configure option `github_access_token` in configuration file `conf/gopm.toml`, so you can go to [here](https://github.com/settings/applications) and create your personal access token(up to 5000 request per hour), and set it in `gopm.toml`.

## Install package, or packages

Command `install` downloads and installs packages along with all dependencies(except when you use bundle or snapshot).

Suppose you want to install package `github.com/GPMGoTest/install_test`, here two ways to do it:

### Install like `go get`

- Execute command `gpm install github.com/GPMGoTest/install_test`, and you do not need to install version control tool. In case you want to, `gpm install -v github.com/GPMGoTest/install_test` calls `go get` in underlying.

### Install through bundle

- It's still not cool enough to download and install packages with import path, let's try execute command `gopm install test.b`, see what happens? 
- Where is the `test.b` comes from? We actually created a bundle for you in directory `repo/bundles/`, and all bundles should be put there. 
- This is how bundle works, you can open it and see what's inside, it includes import path, type, value and dependencies.
- The `test.b` means the bundle whose name is `test`, if you want to use bundle, you have to add suffix `.b`. You may notice that our file name is `install_test.json`, why is `test`? Because we use `bundle_name` inside file, file name doesn't mean anything unless you leave `bundle_name` blank, then the file name becomes bundle name automatically, but be sure that all bundle file name should use JSON and suffix `.json`.
- For `code.google.com`, `launchpad.net`, type is **ALWAYS** `commit`, and you can leave value blank which means up-to-date, or give it a certain value and you will download the same version of the package no matter how many times.
- For `github.com`, `bitbucket.org`, type can be either `commit`, `branch` or `tag`, and give it corresponding value.
- Now, you should have two packages which are `github.com/GPMGoTest/install_test` and `github.com/GPMGoTest/install_test2` in your computer.

### Share?

Copy and paste your bundle files to anyone else, nothing much!

## Build and run it

Command `build` compiles and installs packages along with all dependencies.

Let's switch work directory to package `github.com/GPMGoTest/install_test`.

- Execute command `gopm build -r`.
- After built, you should see string `Welcome to use gopm(Go Package Manager)!` was printed on the screen.
- Then, gpm calls `go install` in underlying, so you should have binary `$GOPATH/bin/install_test`, then gpm moves it to current directory.
- Flag `-r` means run after built, so you saw the string was printed.

### Why we do this?

In some cases like building web applications, we use relative path to access static files, and `go build` compiles packages without saving, so it's a shortcut for `go install` + `go build` + `go run`, and you don't need to compile packages again for those have not changed.

## Remove package, or packages

Command `remove` removes packages from your local file system.

Suppose you want to remove package `github.com/GPMGoTest/install_test2/subpkg`.

- Execute command `gopm remove github.com/GPMGoTest/install_test2/subpkg`, gopm finds this project in all paths in your GOPATH environment.
- You may notice this is not project path, it's OK because gpm knows it, and deletes directory `$GOPATH/src/github.com/GPMGoTest/install_test2/`, this command delete files in `$GOPATH/bin` and `$GOPATH/pkg` as well.
- You can also use `gopm remove test.b` to remove all packages are included in bundle, but we don't need here because we have one more cool stuff to try.

## Use check to check dependencies

Command `check` checks package dependencies and installs missing ones.

Suppose you want to check package `github.com/GPMGoTest/install_test`.

- Switch work directory to package path.
- Execute command `gopm check`.
- That's it!

## Search and find more

Command `search` is for searching packages in [Go Walker](http://gowalker.org) database.

- Execute command `gopm search mysql`.
- Try it by yourself.

## Go further

- Online full documentation is still working, I'm sorry about that. 
- Give us your feedback, these things matters.
- Join us and get better together.
- Contact: [gpmgo.com@gmail.com](mailto:gpmgo.com@gmail.com).