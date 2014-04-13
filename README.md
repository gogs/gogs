Gogs - Go Git Service [![wercker status](https://app.wercker.com/status/ad0bdb0bc450ac6f09bc56b9640a50aa/s/ "wercker status")](https://app.wercker.com/project/bykey/ad0bdb0bc450ac6f09bc56b9640a50aa) [![Build Status](https://drone.io/github.com/gogits/gogs/status.png)](https://drone.io/github.com/gogits/gogs/latest)
=====================

Gogs(Go Git Service) is a Self Hosted Git Service in the Go Programming Language.

![Demo](http://gowalker.org/public/gogs_demo.gif)

##### Current version: 0.2.8 Alpha

### NOTICES

- Due to testing purpose, data of [try.gogits.org](http://try.gogits.org) has been reset in April 6, 2014 and will reset multiple times after. Please do NOT put your important data on the site.
- Demo site [try.gogits.org](http://try.gogits.org) is running under `dev` branch.
- Checkout the `dev` branch code of Gogs should checkout `dev` branch code of `gogits/git` as well.

#### Other language version

- [简体中文](README_ZH.md)

## Purpose

Since we choose to use pure Go implementation of Git manipulation, Gogs certainly supports **ALL platforms**  that Go supports, including Linux, Mac OS X, and Windows with **ZERO** dependency. 

More importantly, Gogs only needs one binary to setup your own project hosting on the fly!

## Overview

- Please see [Wiki](https://github.com/gogits/gogs/wiki) for project design, known issues, change log and road map.
- See [Trello Board](https://trello.com/b/uxAoeLUl/gogs-go-git-service) to follow the develop team.
- Try it before anything? Do it [online](http://try.gogits.org/Unknown/gogs) or go down to **Installation -> Install from binary** section!
- Having troubles? Get help from [Troubleshooting](https://github.com/gogits/gogs/wiki/Troubleshooting).

## Features

- Activity timeline
- SSH/HTTP(S) protocol support.
- Register/delete/rename account.
- Create/migrate/mirror/delete/watch/rename/transfer public/private repository.
- Repository viewer/issue tracker.
- Gravatar and cache support.
- Mail service(register, issue).
- Administration panel.
- Supports MySQL, PostgreSQL and SQLite3.

## Installation

Make sure you install [Prerequirements](https://github.com/gogits/gogs/wiki/Prerequirements) first.

There are 3 ways to install Gogs:

- [Install from binary](https://github.com/gogits/gogs/wiki/Install-from-binary): **STRONGLY RECOMMENDED**
- [Install from source](https://github.com/gogits/gogs/wiki/Install-from-source)
- [Ship with Docker](https://github.com/gogits/gogs/tree/master/dockerfiles)

## Acknowledgments

- Logo is inspired by [martini-contrib](https://github.com/martini-contrib).
- Router and middleware mechanism of [martini](http://martini.codegangsta.io/).
- Mail Service, modules design is inspired by [WeTalk](https://github.com/beego/wetalk).
- System Monitor Status is inspired by [GoBlog](https://github.com/fuxiaohei/goblog).
- Usage and modification from [beego](http://beego.me) modules.
- Thanks [gobuild.io](http://gobuild.io) for providing binary compile and download service.
- Great thanks to [Docker China](http://www.dockboard.org/) for providing [dockerfiles](https://github.com/gogits/gogs/tree/master/dockerfiles).

## Contributors

This project was launched by [Unknown](https://github.com/Unknwon) and [lunny](https://github.com/lunny); [fuxiaohei](https://github.com/fuxiaohei), [slene](https://github.com/slene) and [skyblue](https://github.com/shxsun) joined the team soon after. See [contributors page](https://github.com/gogits/gogs/graphs/contributors) for full list of contributors.

## License

Gogs is under the MIT License. See the [LICENSE](https://github.com/gogits/gogs/blob/master/LICENSE) file for the full license text.
