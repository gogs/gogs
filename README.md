Gogs - Go Git Service [![wercker status](https://app.wercker.com/status/ad0bdb0bc450ac6f09bc56b9640a50aa/s/ "wercker status")](https://app.wercker.com/project/bykey/ad0bdb0bc450ac6f09bc56b9640a50aa) [![Build Status](https://drone.io/github.com/gogits/gogs/status.png)](https://drone.io/github.com/gogits/gogs/latest)
=====================

Gogs(Go Git Service) is a Self Hosted Git Service in the Go Programming Language.

![Demo](http://gowalker.org/public/gogs_demo.gif)

##### Current version: 0.4.5 Alpha

### NOTICES

- Due to testing purpose, data of [try.gogits.org](http://try.gogits.org) has been reset in **June 21, 2014** and will reset multiple times after. Please do **NOT** put your important data on the site.
- Demo site [try.gogits.org](http://try.gogits.org) is running under `dev` branch.

#### Other language version

- [简体中文](README_ZH.md)

## Purpose

Since we choose to use pure Go implementation of Git manipulation, Gogs certainly supports **ALL platforms**  that Go supports, including Linux, Mac OS X, and Windows with **ZERO** dependency. 

More importantly, Gogs only needs one binary to setup your own project hosting on the fly!

## Overview

- Please see [Documentation](http://gogs.io/docs/intro/) for project design, known issues, and change log.
- See [Trello Board](https://trello.com/b/uxAoeLUl/gogs-go-git-service) to follow the develop team.
- Try it before anything? Do it [online](http://try.gogits.org/Unknown/gogs) or go down to **Installation -> Install from binary** section!
- Having troubles? Get help from [Troubleshooting](http://gogs.io/docs/intro/troubleshooting.md).

## Features

- Activity timeline
- SSH/HTTP(S) protocol support
- SMTP/LDAP/reverse proxy authentication support
- Register/delete/rename account
- Create/migrate/mirror/delete/watch/rename/transfer public/private repository
- Repository viewer/release/issue tracker/webhooks
- Add/remove repository collaborators
- Gravatar and cache support
- Mail service(register, issue)
- Administration panel
- Supports MySQL, PostgreSQL and SQLite3
- Social account login(GitHub, Google, QQ, Weibo)

## System Requirements

- A cheap Raspberry Pi is powerful enough to match the minimal requirement.
- 4 CPU Cores and 1GB RAM would be the baseline for teamwork.

## Installation

Make sure you install [Prerequirements](http://gogs.io/docs/installation/) first.

There are 5 ways to install Gogs:

- [Install from binary](http://gogs.io/docs/installation/install_from_binary.md): **STRONGLY RECOMMENDED**
- [Install from source](http://gogs.io/docs/installation/install_from_source.md)
- [Install from packages](http://gogs.io/docs/installation/install_from_packages.md)
- [Ship with Docker](https://github.com/gogits/gogs/tree/master/dockerfiles)
- [Install with Vagrant](https://github.com/geerlingguy/ansible-vagrant-examples/tree/master/gogs)

## Acknowledgments

- Router and middleware mechanism of [martini](http://martini.codegangsta.io/).
- Mail Service, modules design is inspired by [WeTalk](https://github.com/beego/wetalk).
- System Monitor Status is inspired by [GoBlog](https://github.com/fuxiaohei/goblog).
- Usage and modification from [beego](http://beego.me) modules.
- Thanks [lavachen](http://www.lavachen.cn/) and [Rocker](http://weibo.com/rocker1989) for designing Logo.
- Thanks [gobuild.io](http://gobuild.io) for providing binary compile and download service.
- Great thanks to [Docker China](http://www.dockboard.org/) for providing [dockerfiles](https://github.com/gogits/gogs/tree/master/dockerfiles).

## Contributors

The [core team](http://gogs.io/team) of this project. See [contributors page](https://github.com/gogits/gogs/graphs/contributors) for full list of contributors.

## License

This project is under the MIT License. See the [LICENSE](https://github.com/gogits/gogs/blob/master/LICENSE) file for the full license text.
