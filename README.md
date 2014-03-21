Gogs - Go Git Service [![wercker status](https://app.wercker.com/status/ad0bdb0bc450ac6f09bc56b9640a50aa/s/ "wercker status")](https://app.wercker.com/project/bykey/ad0bdb0bc450ac6f09bc56b9640a50aa) [![Go Walker](http://gowalker.org/api/v1/badge)](https://gowalker.org/github.com/gogits/gogs)
=====================

Gogs(Go Git Service) is a GitHub-like clone in the Go Programming Language.

Since we choose to use pure Go implementation of Git manipulation, Gogs certainly supports **ALL platforms**  that Go supports, including Linux, Max OS X, and Windows with **ZERO** dependency.

##### Current version: 0.1.5 Alpha

## Purpose

There are some very good products in this category such as [gitlab](http://gitlab.com), but the environment setup steps often make us crazy. So our goal of Gogs is to build a GitHub-like clone with very easy setup steps, which take advantages of the Go Programming Language.

## Overview

- Please see [Wiki](https://github.com/gogits/gogs/wiki) for project design, develop specification, change log and road map.
- See [Trello Broad](https://trello.com/b/uxAoeLUl/gogs-go-git-service) to follow the develop team.
- Try it before anything? Do it [online](http://try.gogits.org/Unknown/gogs) or go down to **Installation -> Install from binary** section!
- Having troubles? Get help from [Troubleshooting](https://github.com/gogits/gogs/wiki/Troubleshooting).

## Features

- Activity timeline
- SSH protocol support.
- Register/delete account.
- Create/delete public repository.
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

- Mail service is based on [WeTalk](https://github.com/beego/wetalk).
- Logo is inspired by [martini](https://github.com/martini-contrib).

## Contributors

This project was launched by [Unknown](https://github.com/Unknwon) and [lunny](https://github.com/lunny); [fuxiaohei](https://github.com/fuxiaohei) and [slene](https://github.com/slene) joined the team soon after. See [contributors page](https://github.com/gogits/gogs/graphs/contributors) for full list of contributors.