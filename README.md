Gogs - Go Git Service [![Build Status](https://travis-ci.org/gogits/gogs.svg?branch=master)](https://travis-ci.org/gogits/gogs)
=====================

[![Gitter](https://badges.gitter.im/Join%20Chat.svg)](https://gitter.im/gogits/gogs?utm_source=badge&utm_medium=badge&utm_campaign=pr-badge&utm_content=badge)

![](public/img/gogs-large-resize.png)

##### Current version: 0.7.22 Beta

<table>
    <tr>
        <td width="33%"><img src="https://gogs.io/img/screenshots/1.png"></td>
        <td width="33%"><img src="https://gogs.io/img/screenshots/2.png"></td>
        <td width="33%"><img src="https://gogs.io/img/screenshots/3.png"></td>
    </tr>
    <tr>
        <td><img src="https://gogs.io/img/screenshots/4.png"></td>
        <td><img src="https://gogs.io/img/screenshots/5.png"></td>
        <td><img src="https://gogs.io/img/screenshots/6.png"></td>
    </tr>
    <tr>
        <td><img src="https://gogs.io/img/screenshots/7.png"></td>
        <td><img src="https://gogs.io/img/screenshots/8.png"></td>
        <td><img src="https://gogs.io/img/screenshots/9.png"></td>
    </tr>
</table>

### NOTICES

- Due to testing purpose, data of [try.gogs.io](https://try.gogs.io) has been reset in **Jan 28, 2015** and will reset multiple times after. Please do **NOT** put your important data on the site.
- The demo site [try.gogs.io](https://try.gogs.io) is running under `develop` branch.
- :bangbang:<span style="color: red">You **MUST** read [CONTRIBUTING.md](CONTRIBUTING.md) before you start filing an issue or making a Pull Request, and **MUST** discuss with us on [Gitter](https://gitter.im/gogits/gogs) for UI changes, otherwise it's high possibilities that we are not going to merge it.</span>:bangbang:
- Please [start discussion](http://forum.gogs.io/category/2/general-discussion) or [ask a question](http://forum.gogs.io/category/4/getting-help) on [the forum](http://forum.gogs.io/). GitHub issue tracker only keeps **bugs** and **feature requests**, all other topics will be closed without reason.
- If you think there are vulnerabilities in the project, please talk privately to **u@gogs.io**. Thanks!
- If you're interested in using APIs, we have experimental support with [documentation](https://github.com/gogits/go-gogs-client/wiki).
- If your team/company is using Gogs and would like to put your logo on [our website](http://gogs.io), contact us by any means.

[简体中文](README_ZH.md)

## Purpose

The goal of this project is to make the easiest, fastest, and most painless way of setting up a self-hosted Git service. With Go, this can be done with an independent binary distribution across **ALL platforms** that Go supports, including Linux, Mac OS X, Windows and ARM.

## Overview

- Please see the [Documentation](http://gogs.io/docs/intro) for common usages and change log.
- See the [Trello Board](https://trello.com/b/uxAoeLUl/gogs-go-git-service) to follow the develop team.
- Want to try it before doing anything else? Do it [online](https://try.gogs.io/gogs/gogs) or go down to the **Installation -> Install from binary** section!
- Having trouble? Get help with [Troubleshooting](http://gogs.io/docs/intro/troubleshooting.html).
- Want to help with localization? Check out the [guide](http://gogs.io/docs/features/i18n.html)!

## Features

- Activity timeline
- SSH and HTTP/HTTPS protocols
- SMTP/LDAP/Reverse proxy authentication
- Reverse proxy with sub-path
- Account/Organization/Repository management
- Repository/Organization webhooks (including Slack)
- Repository Git hooks/deploy keys
- Repository issues and pull requests
- Add/Remove repository collaborators
- Gravatar and custom source
- Mail service
- Administration panel
- CI integration: [Drone](https://github.com/drone/drone)
- Supports MySQL, PostgreSQL, SQLite3 and [TiDB](https://github.com/pingcap/tidb) (experimental)
- Multi-language support ([14 languages](https://crowdin.com/project/gogs))

## System Requirements

- A cheap Raspberry Pi is powerful enough for basic functionality.
- 2 CPU cores and 1GB RAM would be the baseline for teamwork.

## Browser Support

- Please see [Semantic UI](https://github.com/Semantic-Org/Semantic-UI#browser-support) for specific versions of supported browsers.
- The official support minimal size  is **1024*768**, UI may still looks right in smaller size but no promises and fixes.

## Installation

Make sure you install the [prerequisites](http://gogs.io/docs/installation) first.

There are 5 ways to install Gogs:

- [Install from binary](http://gogs.io/docs/installation/install_from_binary.html)
- [Install from source](http://gogs.io/docs/installation/install_from_source.html)
- [Install from packages](http://gogs.io/docs/installation/install_from_packages.html)
- [Ship with Docker](https://github.com/gogits/gogs/tree/master/docker)
- [Install with Vagrant](https://github.com/geerlingguy/ansible-vagrant-examples/tree/master/gogs)

### Tutorials

- [How To Set Up Gogs on Ubuntu 14.04](https://www.digitalocean.com/community/tutorials/how-to-set-up-gogs-on-ubuntu-14-04)
- [Run your own GitHub-like service with the help of Docker](http://blog.hypriot.com/post/run-your-own-github-like-service-with-docker/)
- [使用 Gogs 搭建自己的 Git 服务器](https://mynook.info/blog/post/host-your-own-git-server-using-gogs) (Chinese)
- [阿里云上 Ubuntu 14.04 64 位安装 Gogs](http://my.oschina.net/luyao/blog/375654) (Chinese)
- [Installing Gogs on FreeBSD](https://www.codejam.info/2015/03/installing-gogs-on-freebsd.html)
- [Gogs on Raspberry Pi](http://blog.meinside.pe.kr/Gogs-on-Raspberry-Pi/)

### Screencasts

- [Instalando Gogs no Ubuntu](http://blog.linuxpro.com.br/2015/08/14/instalando-gogs-no-ubuntu/) (Português)

### Deploy to Cloud

- [OpenShift](https://github.com/tkisme/gogs-openshift)
- [Cloudron](https://cloudron.io/appstore.html#io.gogs.cloudronapp)
- [Scaleway](https://www.scaleway.com/imagehub/gogs/)
- [Portal](https://portaldemo.xyz/cloud/)
- [Sandstorm](https://github.com/cem/gogs-sandstorm)

### Product Support

- [Synology](https://www.synology.com) (Docker)
- [One Space](http://www.onespace.cc) (App Store)

## Acknowledgments

- Router and middleware mechanism of [Macaron](https://github.com/go-macaron/macaron).
- Modules design is inspired by [WeTalk](https://github.com/beego/wetalk).
- System Monitor Status is inspired by [GoBlog](https://github.com/fuxiaohei/goblog).
- Thanks [lavachen](http://www.lavachen.cn/) and [Rocker](http://weibo.com/rocker1989) for designing Logo.
- Thanks [Crowdin](https://crowdin.com/project/gogs) for providing open source translation plan.
- Thanks [DigitalOcean](https://www.digitalocean.com) for hosting home and demo sites.

## Contributors

- Ex-team members [@lunny](https://github.com/lunny), [@fuxiaohei](https://github.com/fuxiaohei) and [@slene](https://github.com/slene).
- See [contributors page](https://github.com/gogits/gogs/graphs/contributors) for full list of contributors.
- See [TRANSLATORS](conf/locale/TRANSLATORS) for public list of translators.

## License

This project is under the MIT License. See the [LICENSE](https://github.com/gogits/gogs/blob/master/LICENSE) file for the full license text.
