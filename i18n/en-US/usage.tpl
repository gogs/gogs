gopm(Go Package Manager) is a Go package manage tool for search, install, update, share packages.

Usage:

	gopm command [arguments]

The commands are:
{{range .}}{{if .Runnable}}
    {{.Name | printf "%-11s"}} {{.Short}}{{end}}{{end}}

Use "gopm help [command]" for more information about a command.

Additional help topics:
{{range .}}{{if not .Runnable}}
    {{.Name | printf "%-11s"}} {{.Short}}{{end}}{{end}}

Use "gopm help [topic]" for more information about that topic.

