gopm - Go Package Manager
===

![GPMGo_Logo](https://raw.github.com/GPMGo/gpm-site/master/static/img/gpmgo2.png?raw=true)

gopm(Go Package Manager) is a Go package manage tool for search, install, update, share and backup packages in Go.

[![Build Status](https://travis-ci.org/GPMGo/gopm.png?branch=master)](https://travis-ci.org/GPMGo/gopm) [![Build Status](https://drone.io/github.com/GPMGo/gpm/status.png)](https://drone.io/github.com/GPMGo/gpm/latest) [![Coverage Status](https://coveralls.io/repos/GPMGo/gpm/badge.png)](https://coveralls.io/r/GPMGo/gpm)

(Travis CI hasn't support Go 1.1 yet)

This application still in experiment, any change could happen, but it doesn't affect download and install packages.

## Main features

- Download packages from popular project hosting with/without version control tools.
- Remove packages from local file system.
- Check package dependencies and download missing ones.
- More specific examples, see [Quick Start](docs/Quick_Start.md).

## Main commands

- `build` compiles and installs packages and dependencies: basically, it calls `go install` and moves executable to current path from `GOPATH` if any, the executable name is the folder name which is default by `go install`.
- `search` searchs packages in [Go Walker](http://gowalker.org) database by keyword.
- `install` downloads and installs packages and dependencies: you can download packages without version control tools like git, hg, svn, etc. It downloads and installs all packages including all dependencies automatically(except when you use bundle or snapshot). For now, this command supports `code.google.com`, `github.com`, `launchpad.net`, `bitbucket.org`. 
- `remove` removes packages and dependencies: it removes all packages including all dependencies(except when you use bundle or snapshot).
- `check` checks dependencies of packages, and install all missing as a choose.

## Known issues

- When you use commands like `gopm install bitbucket.org/zombiezen/gopdf` where is project root path but the directory doesn't contain any source files, you will get error in the installation step, you have to use `gopm install bitbucket.org/zombiezen/gopdf/pdf` in order to go through all steps correctly.
- Cannot remove `.git` with error `access is denied`.

## Todo

### v0.3.*

- Command `install` add flag `-u` for force update pakcages, and need to check if has downloaded same package with same version already.
- Command `install` and `remove` give number to let user choose operate one package.
- Command `check` add feature to update or generate gopack.json.
- Command `install` generates dependencies configuration file.
- Command `install` save tarball add support for packages in code.google.com, bitbucket.org, launchpad.net, git.oschina.net, gitcafe.com, *.codeplex.com.
- Command `build` use dependencies configuration file to build with specific versions of dependencies, if VCS tools are available, simply use `checkout`. 
- Command `clean` is for cleaning empty directories and backup.
- Add gpm working principle design.
- Complete documentation.

### Future

- Command `search` compares version.
- Command `home` and `doc`.
- Command `remove` add flag `-d` for removing dependencies at the same time.
- Command `remove` add feature check for dependencies, make sure other packages don't import this one, and give choose for users.
- Command `update` is for checking updates.
- Command `sync` is for sync packages in two computers, support client and server mode through ssh.
- Command `init` is for auto-configuring Go develop environment.
- Command `install` and `remove` and `update` backup data(up to 50 records) before executing.
- Command `rollback` is for rolling back to certain operation.
- Add feature "struct generator".
- Command `install` add flag `-all` for re-installing everything in GOPATH, usually use this after upgraded Go version.
- Command `daemon` is for auto-compile web applications when debug it locally.
- Collect download and installation results and report to users in the end.
- Add user system to create, edit, upload, and download bundles or snapshots through gpm client program.
- Develop user source API server template application to support user sources in bundles.
- Add support for downloading tarballs from user sources.
- After downloaded all packages in bundles or snapshots, need to check if all dependencies have been downloaded as well.
- Download package from code.google.com only support hg as version control system, probably support git and svn.
- Command `install` add support for downloading code from git.oschina.net, gitcafe.com, *.codeplex.com.
- Command `install` add support for downloading by tag and branch for packages in git.oschina.net, gitcafe.com.
- Command `install` and `remove` add bundle parse code for getting data from server.
- Command `install` and `remove` add snapshot parse code for downloading or removing snapshot.
- Add built-in application version in order to backup data when users update.

## Credits

- Source files that contain code that is from [gopkgdoc](https://github.com/garyburd/gopkgdoc) is honored in specific.
- Idea that support packages build revision is inspired by [gopack](https://github.com/d2fn/gopack).

## License

[MIT-STYLE](LICENSE). 
