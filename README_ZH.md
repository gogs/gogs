# Gogs

Gogs 是一款极易搭建的自助 Git 服务。

## 项目愿景

Gogs（`/gɑgz/`）项目旨在打造一个以最简便的方式搭建简单、稳定和可扩展的自助 Git 服务。使用 Go 语言开发使得 Gogs 能够通过独立的二进制分发，并且支持 Go 语言支持的 **所有平台**，包括 Linux、macOS、Windows 以及 ARM 平台。

## 概览

- 请移步[官网](https://gogs.io)查看用户使用文档
- 请通过 [CHANGELOG.md](CHANGELOG.md) 文件查看各个版本的变更历史
- 想要先睹为快？直接去[在线体验](https://try.gogs.io/gogs/gogs)吧！
- 使用过程中遇到问题？尝试[故障排查](https://gogs.io/docs/intro/troubleshooting.html)或者前往[用户论坛](https://discuss.gogs.io/)获取帮助
- 希望帮助多国语言的翻译吗？请查看[本地化文档](https://gogs.io/docs/features/i18n.html)
- 准备搞点事情？请阅读[开发指南](docs/local_development.md)配置开发环境
- 想调用 API 吗？请查看[文档](https://github.com/gogs/docs-api)吧

## 主要特性

- 控制面板、用户页面以及活动时间线
- 通过 SSH、HTTP 和 HTTPS 协议操作仓库
- 管理用户、组织和仓库
- 仓库和组织级 Webhook，包括 Slack、Discord 和钉钉
- 仓库 Git 钩子和部署密钥
- 仓库工单（Issue）、合并请求（Pull Request）、Wiki、保护分支和多人协作
- 从其它代码平台迁移和镜像仓库以及 Wiki
- 在线编辑仓库文件和 Wiki
- Jupyter Notebook 和 PDF 的渲染
- 通过 SMTP、LDAP、反向代理、GitHub.com 和 GitHub 企业版进行用户认证
- 开启两步验证（2FA）登录
- 自定义 HTML 模板、静态文件和许多其它组件
- 多样的数据库后端，包括 PostgreSQL、MySQL、SQLite3、MSSQL 和 [TiDB](https://github.com/pingcap/tidb)
- 超过[30 种语言](https://crowdin.com/project/gogs)的本地化

## 硬件要求

- 最低的系统硬件要求为一个廉价的树莓派
- 如果用于团队项目管理，建议使用 2 核 CPU 及 512MB 内存
- 当团队成员大量增加时，可以考虑添加 CPU 核数，内存占用保持不变

## 浏览器支持

- 请根据 [Semantic UI](https://github.com/Semantic-Org/Semantic-UI#browser-support) 查看具体支持的浏览器版本。
- 官方支持的最小 UI 尺寸为 **1024*768**，UI 不一定会在更小尺寸的设备上被破坏，但我们无法保证且不会修复。

## 安装部署

在安装 Gogs 之前，您需要先安装 [基本环境](https://gogs.io/docs/installation)。

然后，您可以通过以下 6 种方式来安装 Gogs：

- [二进制安装](https://gogs.io/docs/installation/install_from_binary.html)
- [源码安装](https://gogs.io/docs/installation/install_from_source.html)
- [包管理安装](https://gogs.io/docs/installation/install_from_packages.html)
- [采用 Docker 部署](https://github.com/gogs/gogs/tree/master/docker)
- [通过 Vagrant 安装](https://github.com/geerlingguy/ansible-vagrant-examples/tree/master/gogs)
- [通过基于 Kubernetes 的 Helm Charts](https://github.com/helm/charts/tree/master/incubator/gogs)

### 云端部署

- [OpenShift](https://github.com/tkisme/gogs-openshift)
- [Cloudron](https://cloudron.io/appstore.html#io.gogs.cloudronapp)
- [Scaleway](https://www.scaleway.com/imagehub/gogs/)
- [Sandstorm](https://github.com/cem/gogs-sandstorm)
- [sloppy.io](https://github.com/sloppyio/quickstarters/tree/master/gogs)
- [YunoHost](https://github.com/mbugeia/gogs_ynh)
- [DPlatform](https://github.com/j8r/DPlatform)
- [LunaNode](https://github.com/LunaNode/launchgogs)

### 使用教程

- [使用 Gogs 搭建自己的 Git 服务器](https://blog.mynook.info/post/host-your-own-git-server-using-gogs/)
- [阿里云上 Ubuntu 14.04 64 位安装 Gogs](http://my.oschina.net/luyao/blog/375654)

## 软件、服务以及产品支持

- [Fabric8](http://fabric8.io/)（DevOps）
- [Jenkins](https://plugins.jenkins.io/gogs-webhook/)（CI）
- [Taiga](https://taiga.io/)（项目管理）
- [Puppet](https://forge.puppet.com/Siteminds/gogs)（IT）
- [Kanboard](https://github.com/kanboard/plugin-gogs-webhook)（项目管理）
- [BearyChat](https://bearychat.com/)（团队交流）
- [GitPitch](https://gitpitch.com/)（Markdown 演示）
- [Synology](https://www.synology.com)（Docker）
- [Syncloud](https://syncloud.org/)（应用商店）

## 特别鸣谢

- 感谢 [Egon Elbre](https://twitter.com/egonelbre) 设计的 Logo。
- 感谢 [Crowdin](https://crowdin.com/project/gogs) 提供免费的开源项目本地化支持。
- 感谢 [DigitalOcean](https://www.digitalocean.com)、[VPSServer](https://www.vpsserver.com/)、[Hosted.nl](https://www.hosted.nl/)、[MonoVM](https://monovm.com) 和 [BitLaunch](https://bitlaunch.io) 提供服务器赞助。
- 感谢 [KeyCDN](https://www.keycdn.com/) 提供 CDN 服务赞助。
- 感谢 [Buildkite](https://buildkite.com) 提供免费的开源项目 CI/CD 支持。

## 贡献成员

- 您可以通过查看 [贡献者页面](https://github.com/gogs/gogs/graphs/contributors) 获取 TOP 100 的贡献者列表。
- 您可以通过查看 [TRANSLATORS](conf/locale/TRANSLATORS) 文件获取公开的翻译人员列表。

## 授权许可

本项目采用 MIT 开源授权许可证，完整的授权说明已放置在 [LICENSE](https://github.com/gogs/gogs/blob/master/LICENSE) 文件中。
