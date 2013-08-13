package cmd

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/user"
	"path"
	"path/filepath"
	"strings"
)

var CmdGet = &Command{
	UsageLine: "get [-u] [packages]",
	Short:     "download and install packages and dependencies",
	Long: `
Get downloads and installs the packages named by the import paths,
along with their dependencies.

The -d flag instructs get to stop after downloading the packages; that is,
it instructs get not to install the packages.

The -fix flag instructs get to run the fix tool on the downloaded packages
before resolving dependencies or building the code.

The -u flag instructs get to use the network to update the named packages
and their dependencies. By default, get uses the network to check out
missing packages but does not use it to look for updates to existing packages.

Get also accepts all the flags in the 'go build' and 'go install' commands,
to control the installation. See 'go help build'.

When checking out or updating a package, get looks for a branch or tag
that matches the locally installed version of Go. The most important
rule is that if the local installation is running version "go1", get
searches for a branch or tag named "go1". If no such version exists it
retrieves the most recent version of the package.

For more about specifying packages, see 'go help packages'.

For more about how 'go get' finds source code to
download, see 'go help remote'.

See also: go build, go install, go clean.
`,
}

var getD = CmdGet.Flag.Bool("f", false, "")
var getU = CmdGet.Flag.Bool("u", false, "")

func init() {
	CmdGet.Run = runGet
}

func runGet(cmd *Command, args []string) {
	if len(args) > 0 {
		getDirect(args[0], "trunk")
	}
}

func dirExists(dir string) bool {
	d, e := os.Stat(dir)
	switch {
	case e != nil:
		return false
	case !d.IsDir():
		return false
	}

	return true
}

func fileExists(dir string) bool {
	info, err := os.Stat(dir)
	if err != nil {
		return false
	}

	return !info.IsDir()
}

func download(url string, localfile string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	localdir := filepath.Dir(localfile)
	if !dirExists(localdir) {
		err = os.MkdirAll(localdir, 0777)
		if err != nil {
			return err
		}
	}

	if !fileExists(localfile) {
		f, err := os.Create(localfile)
		if err == nil {
			_, err = io.Copy(f, resp.Body)
		}
		if err != nil {
			return err
		}
	}

	return nil
}

func getPackage(pkgName string, ver string, url string) error {
	curUser, err := user.Current()
	if err != nil {
		return err
	}

	reposDir = strings.Replace(reposDir, "~", curUser.HomeDir, -1)
	localdir := path.Join(reposDir, pkgName)
	localdir, err = filepath.Abs(localdir)
	if err != nil {
		return err
	}

	localfile := path.Join(localdir, "trunk.zip")

	return download(url, localfile)
}

func getDirect(pkgName string, ver string) error {
	urlTempl := "https://codeload.%v/zip/master"
	//urlTempl := "https://%v/archive/master.zip"
	url := fmt.Sprintf(urlTempl, pkgName)

	return getPackage(pkgName, ver, url)
}

func getFromSource(pkgName string, ver string, source string) error {
	urlTempl := "https://%v/%v"
	//urlTempl := "https://%v/archive/master.zip"
	url := fmt.Sprintf(urlTempl, source, pkgName)

	return getPackage(pkgName, ver, url)
}
