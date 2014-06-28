Gogs - Go Git Service [![wercker status](https://app.wercker.com/status/ad0bdb0bc450ac6f09bc56b9640a50aa/s/ "wercker status")](https://app.wercker.com/project/bykey/ad0bdb0bc450ac6f09bc56b9640a50aa) [![Build Status](https://drone.io/github.com/gogits/gogs/status.png)](https://drone.io/github.com/gogits/gogs/latest)
=====================

Gogs(Go Git Service) 是一个由 Go 语言编写的自助 Git 托管服务。

![Demo](http://gowalker.org/public/gogs_demo.gif)

##### 当前版本：0.4.5 Alpha

## 开发目的

Gogs 完全使用 Go 语言来实现对 Git 数据的操作，实现 **零** 依赖，并且支持 Go 语言所支持的 **所有平台**，包括 Linux、Mac OS X 以及 Windows。

更重要的是，您只需要一个可执行文件就能借助 Gogs 快速搭建属于您自己的代码托管服务！

## 项目概览

- 有关项目设计、已知问题和变更日志，请通过  [使用手册](http://gogs.io/docs/intro/) 查看。
- 您可以到 [Trello Board](https://trello.com/b/uxAoeLUl/gogs-go-git-service) 跟随开发团队的脚步。
- 想要先睹为快？通过 [在线体验](http://try.gogits.org/Unknown/gogs) 或查看 **安装部署 -> 二进制安装** 小节。
- 使用过程中遇到问题？尝试从 [故障排查](http://gogs.io/docs/intro/troubleshooting.md) 页面获取帮助。

## 功能特性

- 活动时间线
- 支持 SSH/HTTP(S) 协议
- 支持 SMTP/LDAP/反向代理 用户认证
- 注册/删除/重命名用户
- 创建/迁移/镜像/删除/关注/重命名/转移 公开/私有 仓库
- 仓库 浏览器/发布/缺陷管理/Web 钩子
- 添加/删除 仓库协作者
- Gravatar 以及缓存支持
- 邮件服务（注册、Issue）
- 管理员面板
- 支持 MySQL、PostgreSQL 以及 SQLite3 数据库
- 社交帐号登录（GitHub、Google、QQ、微博）

## 系统要求

- 最低的系统硬件要求为一个廉价的树莓派
- 如果用于团队项目，建议使用 4 核 CPU 及 1GB 内存

## 安装部署

在安装 Gogs 之前，您需要先安装 [基本环境](http://gogs.io/docs/installation/)。

然后，您可以通过以下 5 种方式来安装 Gogs：

- [二进制安装](http://gogs.io/docs/installation/install_from_binary.md): **强烈推荐**
- [源码安装](http://gogs.io/docs/installation/install_from_source.md)
- [包管理安装](http://gogs.io/docs/installation/install_from_packages.md)
- [采用 Docker 部署](https://github.com/gogits/gogs/tree/master/dockerfiles)
- [通过 Vagrant 安装](https://github.com/geerlingguy/ansible-vagrant-examples/tree/master/gogs)

## 特别鸣谢

- 基于 [WeTalk](https://github.com/beego/wetalk) 修改的邮件服务和模块设计。
- 基于 [GoBlog](https://github.com/fuxiaohei/goblog) 修改的系统监视状态。
- [beego](http://beego.me) 模块的使用与修改。
- [martini](http://martini.codegangsta.io/) 的路由与中间件机制。
- 感谢 [gobuild.io](http://gobuild.io) 提供二进制编译与下载服务。
- 感谢 [lavachen](http://www.lavachen.cn/) 和 [Rocker](http://weibo.com/rocker1989) 设计的 Logo。
- 感谢 [Docker 中文社区](http://www.dockboard.org/) 提供的 [dockerfiles](https://github.com/gogits/gogs/tree/master/dockerfiles)。

## 贡献成员

本项目的 [开发团队](http://gogs.io/team)。您可以通过查看 [贡献者页面](https://github.com/gogits/gogs/graphs/contributors) 获取完整的贡献者列表。

## 授权许可

本项目采用 MIT 开源授权许可证，完整的授权说明已放置在 [LICENSE](https://github.com/gogits/gogs/blob/master/LICENSE) 文件中。