gpm - Go Package Manager
===

![GPMGo_Logo](https://raw.github.com/GPMGo/gpm-site/master/static/img/gpmgo.png?raw=true)

gpm(Go Package Manager) is a Go package manage tool for search, install, update and share packages in Go.

## Todo

- Command `install` add support for downloading code from launchpad.net, bitbucket.org; hopefully, support user sources for downloading tarballs.
- Command `install` installs all packages after downloaded.
- After downloaded all packages in bundles or snapshots, need to check if all dependencies have been downloaded as well.
- Develop user source API server template application to support user sources in bundles.
- Add bundle and snapshot parser code for downloading by bundle or snapshot id.
- Add user system to create, edit, upload, and download bundles or snapshots through gpm client program.
- Add option for whether download dependencies packages in example code or not.
- Add gpm working principle design.
- Download package from code.google.com only support hg as version control system, probably support git and svn.
- All errors should have specific title for exactly where were created.