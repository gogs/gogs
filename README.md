Gogs - Go Git Service [![wercker status](https://app.wercker.com/status/ad0bdb0bc450ac6f09bc56b9640a50aa/s/ "wercker status")](https://app.wercker.com/project/bykey/ad0bdb0bc450ac6f09bc56b9640a50aa) [![Build Status](https://drone.io/github.com/gogits/gogs/status.png)](https://drone.io/github.com/gogits/gogs/latest)
=====================

Gogs(Go Git Service) is a Self Hosted Git Service in the Go Programming Language.

![Demo](http://gowalker.org/public/gogs_demo.gif)

##### Current version: 0.1.6 Alpha

[简体中文](README_ZH.md)

## Purpose

Since we choose to use pure Go implementation of Git manipulation, Gogs certainly supports **ALL platforms**  that Go supports, including Linux, Mac OS X, and Windows with **ZERO** dependency. 

More importantly, Gogs only needs one binary to setup your own project hosting on the fly!

## Overview

- Please see [Wiki](https://github.com/gogits/gogs/wiki) for project design, develop specification, change log and road map.
- See [Trello Broad](https://trello.com/b/uxAoeLUl/gogs-go-git-service) to follow the develop team.
- Try it before anything? Do it [online](http://try.gogits.org/Unknown/gogs) or go down to **Installation -> Install from binary** section!
- Having troubles? Get help from [Troubleshooting](https://github.com/gogits/gogs/wiki/Troubleshooting).

## Features

- Activity timeline
- SSH protocol support.
- Register/delete account.
- Create/delete/watch public repository.
- User profile page.
- Repository viewer.
- Gravatar support.
- Mail service(register).
- Administration panel.
- Supports MySQL, PostgreSQL and SQLite3(binary release only).

## Installation

Make sure you install [Prerequirements](https://github.com/gogits/gogs/wiki/Prerequirements) first.

There are two ways to install Gogs:

- [Install from binary](https://github.com/gogits/gogs/wiki/Install-from-binary): **STRONGLY RECOMMENDED** for just try and deployment!
- [Install from source](https://github.com/gogits/gogs/wiki/Install-from-source)

## Acknowledgments

- Logo is inspired by [martini](https://github.com/martini-contrib).
- Mail Service, modules design is inspired by [WeTalk](https://github.com/beego/wetalk).
- System Monitor Status is inspired by [GoBlog](https://github.com/fuxiaohei/goblog).

## Contributors

This project was launched by [Unknown](https://github.com/Unknwon) and [lunny](https://github.com/lunny); [fuxiaohei](https://github.com/fuxiaohei) and [slene](https://github.com/slene) joined the team soon after. See [contributors page](https://github.com/gogits/gogs/graphs/contributors) for full list of contributors.