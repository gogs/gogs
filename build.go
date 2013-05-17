// Copyright (c) 2013 GPMGo Members. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"runtime"
	"strings"

	"github.com/GPMGo/gpm/utils"
)

var cmdBuild = &Command{
	UsageLine: "build [-o output] [build flags] [packages]",
	Short:     "compile and install packages and dependencies",
	Long: `
Build compiles the packages named by the import paths,
along with their dependencies, but it does not install the results.

If the arguments are a list of .go files, build treats them as a list
of source files specifying a single package.

When the command line specifies a single main package,
build writes the resulting executable to output.
Otherwise build compiles the packages but discards the results,
serving only as a check that the packages can be built.

The -o flag specifies the output file name. If not specified, the
output file name depends on the arguments and derives from the name
of the package, such as p.a for package p, unless p is 'main'. If
the package is main and file names are provided, the file name
derives from the first file name mentioned, such as f1 for 'go build
f1.go f2.go'; with no files provided ('go build'), the output file7
name is the base name of the containing directory.

The build flags are shared by the build, install, run, and test commands:

	-a
		force rebuilding of packages that are already up-to-date.
	-n
		print the commands but do not run them.
	-p n
		the number of builds that can be run in parallel.
		The default is the number of CPUs available.
	-race
		enable data race detection.
		Supported only on linux/amd64, darwin/amd64 and windows/amd64.
	-v
		print the names of packages as they are compiled.
	-work
		print the name of the temporary work directory and
		do not delete it when exiting.
	-x
		print the commands.

	-ccflags 'arg list'
		arguments to pass on each 5c, 6c, or 8c compiler invocation.
	-compiler name
		name of compiler to use, as in runtime.Compiler (gccgo or gc).
	-gccgoflags 'arg list'
		arguments to pass on each gccgo compiler/linker invocation.
	-gcflags 'arg list'
		arguments to pass on each 5g, 6g, or 8g compiler invocation.
	-installsuffix suffix
		a suffix to use in the name of the package installation directory,
		in order to keep output separate from default builds.
		If using the -race flag, the install suffix is automatically set to race
		or, if set explicitly, has _race appended to it.
	-ldflags 'flag list'
		arguments to pass on each 5l, 6l, or 8l linker invocation.
	-tags 'tag list'
		a list of build tags to consider satisfied during the build.
		See the documentation for the go/build package for
		more information about build tags.

The list flags accept a space-separated list of strings. To embed spaces
in an element in the list, surround it with either single or double quotes.

For more about specifying packages, see 'go help packages'.
For more about where packages and binaries are installed,
see 'go help gopath'.

See also: go install, go get, go clean.
	`,
}

func init() {
	// break init cycle
	cmdBuild.Run = runBuild
	//cmdInstall.Run = runInstall

	addBuildFlags(cmdBuild)
	//addBuildFlags(cmdInstall)
}

// Flags set by multiple commands.
var buildV bool // -v flag.

// addBuildFlags adds the flags common to the build and install commands.
func addBuildFlags(cmd *Command) {
	// NOTE: If you add flags here, also add them to testflag.go.
	cmd.Flag.BoolVar(&buildV, "v", false, "")
}

func runBuild(cmd *Command, args []string) {
	var cmdArgs []string
	cmdArgs = append(cmdArgs, "install")
	cmdArgs = append(cmdArgs, args...)

	wd, _ := os.Getwd()
	wd = strings.Replace(wd, "\\", "/", -1)
	proName := path.Base(wd)
	if runtime.GOOS == "windows" {
		proName += ".exe"
	}

	cmdExec := exec.Command("go", cmdArgs...)
	stdout, err := cmdExec.StdoutPipe()
	if err != nil {
		fmt.Println(err)
	}
	stderr, err := cmdExec.StderrPipe()
	if err != nil {
		fmt.Println(err)
	}
	err = cmdExec.Start()
	if err != nil {
		fmt.Println(err)
	}
	go io.Copy(os.Stdout, stdout)
	go io.Copy(os.Stderr, stderr)
	cmdExec.Wait()

	// Find executable in GOPATH and copy to current directory.
	gopath := strings.Replace(os.Getenv("GOPATH"), ";", ":", -1)
	gopath = strings.Replace(gopath, "\\", "/", -1)
	paths := strings.Split(gopath, ":")
	for _, v := range paths {
		if utils.IsExist(v + "/bin/" + proName) {
			err = os.Remove(wd + "/" + proName)
			if err != nil {
				fmt.Println("Fail to remove file in current directory :", err)
				return
			}
			err = os.Rename(v+"/bin/"+proName, wd+"/"+proName)
			if err == nil {
				fmt.Println("Moved file from $GOPATH to current directory.")
				return
			} else {
				fmt.Println("Fail to move file from $GOPATH to current directory :", err)
			}
			break
		}
	}
}
