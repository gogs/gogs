gopm
====

* [总体设计目标](#10)
* [Go包版本说明](#20)
* [各命令的目标和作用](#30)
	* [gopm help](#31) 
	* [gopm sources](#32)
	* [gopm list](#33)
	* [gopm get](#34)
	* [gopm rm](#35)
	* [gopm search](#36)
	* [gopm doc](#37)
	* [gopm serve](#38)
	* [gopm sync](#39)
	* [gopm import](#40)
	* [gopm gen](#41)
	* [gopm build](#42)
	* [gopm run](#43)
	* [gopm test](#44)
* [gopmspec文件格式](#50)

<a id="10" name="10"></a>
#总体设计目标

1. 支持go语言的版本管理
2. 支持文档管理
3. 支持本地源服务器
4. 本地源服务器同时支持公共包和私有包
5. 支持依赖管理
6. 支持从github, code.google.com, gitLab, 等常见的源码托管服务下载 

<a id="20" name="20"></a>
#Go包版本说明

版本分为四种：

* []:      表示的是当前最新版本即trunk     
* branch:  表示的是某个分支
* tag:     表示的是某个tag
* commit:  表示的是某个reversion

<a id="30" name="30"></a>
#各命令的目标和作用

<a id="31" name="31"></a>
###gopm help        

显示当前可用的命令，以下命令中，[]表示可选，{}表示是参数

<a id="32" name="32"></a>
###gopm sources [add|rm [{url}]]

* []   	   列出当前可用的所有源，默认为http://gopm.io/
* add url 添加一个源到本地
* rm  url 删除一个源到本地，如果没有任何源，则自己成为一个独立的服务器，类似gopm.io

<a id="33" name="33"></a>
###gopm list [{packagename}[:{version}]]

* []   列出所有本地的包
* packagename   显示指定名称的包的详细信息

<a id="34" name="34"></a>
###gopm get [-u] [{packagename}[:{version}]] [-f {gopmfile}]

* [] 查找当前目录下的所有.gopmfile文件，根据文件的描述下载所有的包
* packagename 从源中下载某个包
* -u packagename 从源中更新某个包
* -f gopmfile 根据指定的文件来下载包

<a id="35" name="35"></a>
###gopm rm {packagename}[:{version}]

去除一个包，如果不加版本标示，则删除该包的所有版本

<a id="36" name="36"></a>
###gopm search {keyword}

根据关键词查找包

<a id="37" name="37"></a>
###gopm doc [-b] {packagename}[:{version}]

* []   显示一个包的文档
* -b   在默认浏览器中显示该包的文档

<a id="38" name="38"></a>
###gopm serve [-p {port}]

将本地仓库作为服务对外提供，如果没有-p，则端口为80，如果有，则端口为指定端口，该服务是一个web服务，通过浏览器也可以进行浏览。

<a id="39" name="39"></a>
###gopm sync [-u]

[] 如果当前配置了源，则从可用的源中同步所有的包信息和包内容的最新版本到本地仓库；
    如果当前没有配置任何源，则将所有已有的包从源头进行更新
-u  仅更新本地仓库已经有的包，不包含本地仓库没有的包

<a id="40" name="40"></a>
###gopm import [{url}|{filepath}]

将某个地址或者本地的包导入到本地仓库中，url应为可支持的源码托管站点或者gitLab

<a id="41" name="41"></a>
###gopm gen [{gopmfile}]

扫描当前目录下的go工程，并自动生成一个.gopmspec的文件依赖文档，如果未指定，则文件名为.gopmspec，如果指定了，则为指定的文件名

<a id="42" name="42"></a>
###gopm build [-u]

此命令依赖于go build

1. 如果当前没有.gopmspec文件，则扫描当前的go工程的依赖，自动生成.gopmspec文档
2. 根据.gopmspec文件自动下载所有需要的包，如果加了-u参数，则同时更新所有的包
3. 根据.gopmspec文件自动切换gopath中的相关版本
4. 调用go build对工程进行编译

<a id="43" name="43"></a>
###gopm run [{gofile}]

此命令依赖于go run

调用gopm build在临时文件夹生成可执行文件，并设置程序当前目录为当前目录，并执行

<a id="44" name="44"></a>
###gopm test

此命令依赖于go test

调用gopm build在临时文件夹生成可执行的测试文件，并设置程序当前目录为当前目录，并执行

<a id="50" name="50"></a>
#gopmspec文件格式

.gopmspec文件的格式类似一个ini文件，当前分为两个section。
build段内的依赖保存的是go build所需要依赖的所有包，一行一个，可用 =, >=等等，如果什么符号都没有，就是取最新版本

```
[build]
xweb
beego = tag:0.1
xorm >= branch:0.2

[test]
testing
```
