gopm（Go 包管理工具） 是一款涵盖搜索、安装、更新、分享功能 Go 包的管理工具。

用法:

	gopm 命令 [参数]

命令列表:
{{range .}}{{if .Runnable}}
    {{.Name | printf "%-11s"}} {{.Short}}{{end}}{{end}}

使用 "gopm help [命令]" 来获取相关命令的更多信息.

其它帮助主题:
{{range .}}{{if not .Runnable}}
    {{.Name | printf "%-11s"}} {{.Short}}{{end}}{{end}}

使用 "gopm help [主题]" 来获取相关主题的更多信息.

