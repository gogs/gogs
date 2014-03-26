Gogs - Go Git Service [![wercker status](https://app.wercker.com/status/ad0bdb0bc450ac6f09bc56b9640a50aa/s/ "wercker status")](https://app.wercker.com/project/bykey/ad0bdb0bc450ac6f09bc56b9640a50aa) [![Build Status](https://drone.io/github.com/gogits/gogs/status.png)](https://drone.io/github.com/gogits/gogs/latest)
=====================

Gogs(Go Git Service) 是一个由 Go 语言编写的自助 Git 托管服务。

![Demo](http://gowalker.org/public/gogs_demo.gif)

##### 当前版本：0.1.8 Alpha

## 开发目的

Gogs 完全使用 Go 语言来实现对 Git 数据的操作，实现 **零** 依赖，并且支持 Go 语言所支持的 **所有平台**，包括 Linux、Mac OS X 以及 Windows。

更重要的是，您只需要一个可执行文件就能借助 Gogs 快速搭建属于您自己的代码托管服务！

## 项目概览

- 有关项目设计、开发说明、变更日志和路线图，请通过  [Wiki](https://github.com/gogits/gogs/wiki) 查看。
- 您可以到 [Trello Board](https://trello.com/b/uxAoeLUl/gogs-go-git-service) 跟随开发团队的脚步。
- 想要先睹为快？通过 [在线体验](http://try.gogits.org/Unknown/gogs) 或查看 **安装部署 -> 二进制安装** 小节。
- 使用过程中遇到问题？尝试从 [故障排查](https://github.com/gogits/gogs/wiki/Troubleshooting) 页面获取帮助。

## 功能特性

- 活动时间线
- SSH/HTTPS 协议支持
- 注册/删除用户
- 创建/删除/关注公开仓库
- 用户个人信息页面
- 仓库浏览器
- Gravatar 以及缓存支持
- 邮件服务（注册、Issue）
- 管理员面板
- 支持 MySQL、PostgreSQL 以及 SQLite3（仅限二进制版本）

## 安装部署

在安装 Gogs 之前，您需要先安装 [基本环境](https://github.com/gogits/gogs/wiki/Prerequirements)。

然后，您可以通过以下两种方式来安装 Gogs：

- [二进制安装](https://github.com/gogits/gogs/wiki/Install-from-binary): **强烈推荐** 适合体验者和实际部署
- [源码安装](https://github.com/gogits/gogs/wiki/Install-from-source)

## 特别鸣谢

- Logo 基于 [martini-contrib](https://github.com/martini-contrib) 修改而来。
- 基于 [WeTalk](https://github.com/beego/wetalk) 修改的邮件服务和模块设计。
- 基于 [GoBlog](https://github.com/fuxiaohei/goblog) 修改的系统监视状态。
- [beego](http://beego.me) 模块的使用与修改。
- [martini](http://martini.codegangsta.io/) 的路由与中间件机制。
- 感谢 [gobuild.io](http://gobuild.io) 提供二进制编译与下载服务。

## 贡献成员

本项目最初由 [Unknown](https://github.com/Unknwon) 和 [lunny](https://github.com/lunny) 发起，随后 [fuxiaohei](https://github.com/fuxiaohei) 与 [slene](https://github.com/slene) 加入到开发团队。您可以通过查看 [贡献者页面](https://github.com/gogits/gogs/graphs/contributors) 获取完整的贡献者列表。