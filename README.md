# <img src="https://github.com/gogs/gogs/raw/main/public/img/favicon.png" width="45" align="left">Gogs - A painless self-hosted Git service

[![GitHub Workflow Status](https://img.shields.io/github/workflow/status/gogs/gogs/Go?logo=github&style=for-the-badge)](https://github.com/gogs/gogs/actions?query=workflow%3AGo) [![Discord](https://img.shields.io/discord/382595433060499458.svg?style=for-the-badge&logo=discord)](https://discord.gg/9aqdHU7) [![Sourcegraph](https://img.shields.io/badge/view%20on-Sourcegraph-brightgreen.svg?style=for-the-badge&logo=sourcegraph)](https://sourcegraph.com/github.com/gogs/gogs)

![Repository](https://gogs.io/img/screenshots/2.png)

[简体中文](README_ZH.md)

## 🔮 Vision

The Gogs (`/gɑgz/`) project aims to build a simple, stable and extensible self-hosted Git service that can be setup in the most painless way. With Go, this can be done with an independent binary distribution across **ALL platforms** that Go supports, including Linux, macOS, Windows and ARM.

## 📡 Overview

- Please visit [our home page](https://gogs.io) for user documentation.
- Please refer to [CHANGELOG.md](CHANGELOG.md) for list of changes in each releases.
- Want to try it before doing anything else? Do it [online](https://try.gogs.io/gogs/gogs)!
- Having trouble? Help yourself with [troubleshooting](https://gogs.io/docs/intro/troubleshooting.html) or ask questions on [user forum](https://discuss.gogs.io/).
- Want to help with localization? Check out the [localization documentation](https://gogs.io/docs/features/i18n.html).
- Ready to get hands dirty? Read our guide to [set up your development environment](docs/dev/local_development.md).
- Hmm... What about APIs? We have experimental support with [documentation](https://github.com/gogs/docs-api).

## 💌 Features

- User dashboard, user profile and activity timeline.
- Access repositories via SSH, HTTP and HTTPS protocols.
- User, organization and repository management.
- Repository and organization webhooks, including Slack, Discord and Dingtalk.
- Repository Git hooks, deploy keys and Git LFS.
- Repository issues, pull requests, wiki, protected branches and collaboration.
- Migrate and mirror repositories with wiki from other code hosts.
- Web editor for quick editing repository files and wiki.
- Jupyter Notebook and PDF rendering.
- Authentication via SMTP, LDAP, reverse proxy, GitHub.com and GitHub Enterprise with 2FA.
- Customize HTML templates, static files and many others.
- Rich database backend, including PostgreSQL, MySQL, SQLite3 and [TiDB](https://github.com/pingcap/tidb).
- Have localization over [30 languages](https://crowdin.com/project/gogs).

## 💾 Hardware requirements

- A Raspberry Pi or $5 Digital Ocean Droplet is more than enough to get you started. Some even use 64MB RAM Docker [CaaS](https://blog.docker.com/2016/02/containers-as-a-service-caas/).
- 2 CPU cores and 512MB RAM would be the baseline for teamwork.
- Increase CPU cores when your team size gets significantly larger, memory footprint remains low.

## 💻 Browser support

- Please see [Semantic UI](https://github.com/Semantic-Org/Semantic-UI#browser-support) for specific versions of supported browsers.
- The smallest resolution officially supported is **1024*768**, however the UI may still look right in smaller resolutions, but no promises or fixes.

## 📜 Installation

Make sure you install the [prerequisites](https://gogs.io/docs/installation) first.

There are 6 ways to install Gogs:

- [Install from binary](https://gogs.io/docs/installation/install_from_binary.html)
- [Install from source](https://gogs.io/docs/installation/install_from_source.html)
- [Install from packages](https://gogs.io/docs/installation/install_from_packages.html)
- [Ship with Docker](https://github.com/gogs/gogs/tree/main/docker)
- [Install with Vagrant](https://github.com/geerlingguy/ansible-vagrant-examples/tree/master/gogs)
- [Install with Kubernetes Using Helm Charts](https://github.com/helm/charts/tree/master/incubator/gogs)

### Deploy to cloud

- [Cloudron](https://cloudron.io/appstore.html#io.gogs.cloudronapp)
- [Scaleway](https://www.scaleway.com/imagehub/gogs/)
- [Sandstorm](https://github.com/cem/gogs-sandstorm)
- [sloppy.io](https://github.com/sloppyio/quickstarters/tree/master/gogs)
- [YunoHost](https://github.com/YunoHost-Apps/gogs_ynh)
- [DPlatform](https://github.com/j8r/DPlatform)
- [LunaNode](https://github.com/LunaNode/launchgogs)

### Tutorials

- [How To Set Up Gogs on Ubuntu 14.04](https://www.digitalocean.com/community/tutorials/how-to-set-up-gogs-on-ubuntu-14-04)
- [Run your own GitHub-like service with the help of Docker](http://blog.hypriot.com/post/run-your-own-github-like-service-with-docker/)
- [Dockerized Gogs git server and alpine postgres in 20 minutes or less](http://garthwaite.org/docker-gogs.html)
- [Host Your Own Private GitHub with Gogs](https://eladnava.com/host-your-own-private-github-with-gogs-io/)
- [使用 Gogs 搭建自己的 Git 服务器](https://blog.mynook.info/post/host-your-own-git-server-using-gogs/) (Chinese)
- [阿里云上 Ubuntu 14.04 64 位安装 Gogs](http://my.oschina.net/luyao/blog/375654) (Chinese)
- [Installing Gogs on FreeBSD](https://www.codejam.info/2015/03/installing-gogs-on-freebsd.html)
- [Cloudflare Full SSL with Gogs using NGINX](http://www.listekconsulting.com/articles/cloudflare-full-ssl-with-gogs-go-git-service-using-nginx/)
- [How to install Gogs on a Linux Server (DigitalOcean)](https://www.youtube.com/watch?v=deSfX0gqefE)

## 📦 Software, service and product support

- [Fabric8](http://fabric8.io/) (DevOps)
- [Jenkins](https://plugins.jenkins.io/gogs-webhook/) (CI)
- [Taiga](https://taiga.io/) (Project Management)
- [Puppet](https://forge.puppet.com/Siteminds/gogs) (IT)
- [Kanboard](https://github.com/kanboard/plugin-gogs-webhook) (Project Management)
- [BearyChat](https://bearychat.com/) (Team Communication)
- [GitPitch](https://gitpitch.com/) (Markdown Presentations)
- [Synology](https://www.synology.com) (Docker)
- [Syncloud](https://syncloud.org/) (App Store)

## 🙇‍♂️ Acknowledgments

- Thanks [Egon Elbre](https://twitter.com/egonelbre) for designing the original version of the logo.
- Thanks [Crowdin](https://crowdin.com/project/gogs) for sponsoring open source translation plan.
- Thanks [DigitalOcean](https://www.digitalocean.com), [VPSServer](https://www.vpsserver.com/), [Hosted.nl](https://www.hosted.nl/), [MonoVM](https://monovm.com) and [BitLaunch](https://bitlaunch.io) for sponsoring VPS services.
- Thanks [KeyCDN](https://www.keycdn.com/) for sponsoring CDN service.
- Thanks [Buildkite](https://buildkite.com) for sponsoring open source CI/CD plan.

## 👋 Contributors

- See [contributors page](https://github.com/gogs/gogs/graphs/contributors) for top 100 contributors.
- See [TRANSLATORS](conf/locale/TRANSLATORS) for public list of translators.

## License

This project is under the MIT License. See the [LICENSE](https://github.com/gogs/gogs/blob/main/LICENSE) file for the full license text.
