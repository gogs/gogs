Gogs - Go Git Service [![Build Status](https://travis-ci.org/gogits/gogs.svg?branch=master)](https://travis-ci.org/gogits/gogs)
=====================

[![Gitter](https://badges.gitter.im/Join%20Chat.svg)](https://gitter.im/gogits/gogs?utm_source=badge&utm_medium=badge&utm_campaign=pr-badge&utm_content=badge)

Gogs(Go Git Service) is a painless self-hosted Git Service written in Go.

![Demo](http://gogs.qiniudn.com/gogs_demo.gif)

##### Current version: 0.5.13 Beta

### NOTICES

- Due to testing purpose, data of [try.gogs.io](https://try.gogs.io) has been reset in **Jan 28, 2015** and will reset multiple times after. Please do **NOT** put your important data on the site.
- Demo site [try.gogs.io](https://try.gogs.io) is running under `dev` branch.
- You **MUST** read [CONTRIBUTING.md](CONTRIBUTING.md) before you start filing a issue or making a Pull Request.
- If you think there are vulnerabilities in the project, please talk private to **u@gogs.io**, thanks!

#### Other language version

- [简体中文](README_ZH.md)

## Purpose

The goal of this project is to make the easiest, fastest and most painless way to set up a self-hosted Git service. With Go, this can be done in independent binary distribution across **ALL platforms** that Go supports, including Linux, Mac OS X, and Windows.

## Overview

- Please see [Documentation](http://gogs.io/docs/intro/) for project design, known issues, and change log.
- See [Trello Board](https://trello.com/b/uxAoeLUl/gogs-go-git-service) to follow the develop team.
- Try it before anything? Do it [online](https://try.gogs.io/Unknown/gogs) or go down to **Installation -> Install from binary** section!
- Having troubles? Get help from [Troubleshooting](http://gogs.io/docs/intro/troubleshooting.md).
- Want to help on localization? Check out [Crowdin](https://crowdin.com/project/gogs)!

## Features

- Activity timeline
- SSH/HTTP(S) protocol support
- SMTP/LDAP/reverse proxy authentication support
- Reverse proxy suburl support
- Register/delete/rename account
- Create/manage/delete organization with team management
- Create/fork/migrate/mirror/delete/watch/rename/transfer public/private repository
- Repository viewer/release/issue tracker
- Repository and Organization level webhooks
- Repository Git hooks
- Add/remove repository collaborators
- Gravatar and cache support
- Mail service(register, issue)
- Administration panel
- Slack webhook integration
- Drone CI integration
- Supports MySQL, PostgreSQL and SQLite3
- Social account login(GitHub, Google, QQ, Weibo)
- Multi-language support([9 languages](https://crowdin.com/project/gogs))

## System Requirements

- A cheap Raspberry Pi is powerful enough to match the minimal requirement.
- 4 CPU Cores and 1GB RAM would be the baseline for teamwork.

## Installation

Make sure you install [Prerequirements](http://gogs.io/docs/installation/) first.

There are 5 ways to install Gogs:

- [Install from binary](http://gogs.io/docs/installation/install_from_binary.md)
- [Install from source](http://gogs.io/docs/installation/install_from_source.md)
- [Install from packages](http://gogs.io/docs/installation/install_from_packages.md)
- [Ship with Docker](https://github.com/gogits/gogs/tree/master/docker)
- [Install with Vagrant](https://github.com/geerlingguy/ansible-vagrant-examples/tree/master/gogs)

## Acknowledgments

- Router and middleware mechanism of [Macaron](https://github.com/Unknwon/macaron).
- Mail Service, modules design is inspired by [WeTalk](https://github.com/beego/wetalk).
- System Monitor Status is inspired by [GoBlog](https://github.com/fuxiaohei/goblog).
- Thanks [lavachen](http://www.lavachen.cn/) and [Rocker](http://weibo.com/rocker1989) for designing Logo.
- Thanks [gobuild.io](http://gobuild.io) for providing binary compile and download service.
- Thanks [Crowdin](https://crowdin.com/project/gogs) for providing open source translation plan.

## Contributors

- The [core team](http://gogs.io/team) of this project.
- See [contributors page](https://github.com/gogits/gogs/graphs/contributors) for full list of contributors.
- See [TRANSLATORS](conf/locale/TRANSLATORS) for full list of translators.

## License

This project is under the MIT License. See the [LICENSE](https://github.com/gogits/gogs/blob/master/LICENSE) file for the full license text.
