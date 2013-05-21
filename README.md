gpm - Go Package Manager
===

![GPMGo_Logo](https://raw.github.com/GPMGo/gpm-site/master/static/img/gpmgo.png?raw=true)

gpm(Go Package Manager) is a Go package manage tool for search, install, update and share packages in Go.

## Todo

- Command `install` add support for downloading code from git.oschina.net, gitcafe.com, *.codeplex.com;
- Add support for downloading tarballs from user sources.
- After downloaded all packages in bundles or snapshots, need to check if all dependencies have been downloaded as well.
- Develop user source API server template application to support user sources in bundles.
- Add bundle and snapshot parser code for downloading by bundle or snapshot id.
- Add user system to create, edit, upload, and download bundles or snapshots through gpm client program.
- Add gpm working principle design.
- Download package from code.google.com only support hg as version control system, probably support git and svn.
- All errors should have specific title for exactly where were created.
- Add i18n support for all strings.
- Add feature for downloading through version control tools, and use `checkout` to switch to specific revision; this feature only be enabled when users use bundle or snapshot id.
- Add support for downloading by tag for packages in github.com, bitbucket.org.
- Get author commit time and save in node.
- Save node information after downloaded, and check for next time, reduce download times.
- Collect download and installation results and report to users in the end.