package cmd

var CmdGen = &Command{
	UsageLine: "gen [.gopmfile]",
	Short:     "generate a gopmfile according current go project",
	Long: `
generate a gopmfile according current go project
`,
}

func init() {
	CmdGen.Run = gen
}

// scan a directory and gen a gopm file
func gen(cmd *Command, args []string) {

}
