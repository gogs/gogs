gpm - Go 包管理工具
===

![GPMGo_Logo](https://raw.github.com/GPMGo/gpm-site/master/static/img/gpmgo2.png?raw=true)

gpm（Go 包管理工具） 是一款涵盖搜索、安装、更新、分享以及备份功能 Go 包的管理工具。

[![Build Status](https://travis-ci.org/GPMGo/gpm.png)](https://travis-ci.org/GPMGo/gpm) [![Build Status](https://drone.io/github.com/GPMGo/gpm/status.png)](https://drone.io/github.com/GPMGo/gpm/latest) [![Coverage Status](https://coveralls.io/repos/GPMGo/gpm/badge.png)](https://coveralls.io/r/GPMGo/gpm)

（Travis CI 暂未支持 Go 1.1）

该应用目前扔处于实验阶段，任何改变都可能发生，但这不会影响到您下载和安装 Go 包。

## 主要功能

- 无需安装各类复杂的版本控制工具就可以从源代码托管平台下载并安装 Go 包。
- 从本地文件系统中删除 Go 包。
- 更多示例，参见 [快速入门](docs/Quick_Start_ZH.md)

## 主要命令

- `build` 编译并安装 Go 包以及其依赖包：该命令从底层调用 `go install` 命令，如果为 main 包，则会将可执行文件从 `GOPATH` 中移至当前目录，可执行文件的名称是由 `go install` 默认指定的当前文件夹名称。 
- `install` 下载并安装 Go 包以及其依赖包：您无需安装像 git、hg 或 svn 这类版本控制工具就可以下载您指定的包。该命令也会自动下载相关的依赖包（当您使用集合或快照下载时，不会自动下载依赖包）。目前，该命令支持托管在 `code.google.com`、`github.com`、`launchpad.net` 和 `bitbucket.org` 上的开源项目。 
- `remove` 删除 Go 包及其依赖包：该命令可删除 Go 包及其依赖包（当您使用集合或快照删除时，无法自动删除依赖包）。

## 已知问题

- 当您使用命令例如 `gpm install -p bitbucket.org/zombiezen/gopdf` 时，你会在安装步骤时得到错误，虽然这是项目的根目录，但是并没有包含任何 Go 源代码，因此您必须使用 `gpm install -p bitbucket.org/zombiezen/gopdf/pdf` 才能正确完成安装。

## 授权许可

[MIT-STYLE](LICENSE)