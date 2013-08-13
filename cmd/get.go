package cmd

import (
	"archive/zip"
	//"errors"
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

For more about how 'gopm get' finds source code to
download, see 'gopm help'.

See also: gopm build, gopm install, gopm clean.
`,
}

var getD = CmdGet.Flag.Bool("f", false, "")
var getU = CmdGet.Flag.Bool("u", false, "")

func init() {
	CmdGet.Run = runGet
}

func isStandalone() bool {
	return true
}

func runGet(cmd *Command, args []string) {
	if len(args) > 0 {
		var ver string = TRUNK
		if len(args) == 2 {
			ver = args[1]
		}
		pkg := NewPkg(args[0], ver)
		if isStandalone() {
			getDirect(pkg)
		} else {
			fmt.Println("Not implemented.")
			//getSource(pkgName)
		}
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
	fmt.Println("Downloading", url, "...")
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

/*func extractPkg(pkg *Pkg, update bool) error {
	gopath := os.Getenv("GOPATH")
	var childDirs []string = strings.Split(pkg.Name, "/")

	if pkg.Ver != TRUNK {
		childDirs[len(childDirs)-1] = fmt.Sprintf("%v_%v_%v", childDirs[len(childDirs)-1], pkg.Ver, pkg.VerId)
	}
	srcDir = path.Join(gopath, childDir...)

	if !update {
		if dirExists(srcDir) {
			return nil
		}
		err = os.MkdirAll(localdir, 0777)
		if err != nil {
			return err
		}
	} else {
		if dirExists(srcDir) {
			os.Remove(localdir)
		} else {
			err = os.MkdirAll(localdir, 0777)
			if err != nil {
				return err
			}
		}
	}

	// Iterate through the files in the archive,
	// printing some of their contents.
	for _, f := range r.File {
		fmt.Printf("Contents of %s:\n", f.Name)
		rc, err := f.Open()
		if err != nil {
			return err
		}

		_, err = io.Copy(os.Stdout, rc)
		if err != nil {
			return err
		}
		rc.Close()
	}
	return nil
}*/

func getPackage(pkg *Pkg, url string) error {
	curUser, err := user.Current()
	if err != nil {
		return err
	}

	reposDir = strings.Replace(reposDir, "~", curUser.HomeDir, -1)
	localdir := path.Join(reposDir, pkg.Name)
	localdir, err = filepath.Abs(localdir)
	if err != nil {
		return err
	}

	urls := strings.Split(url, ".")

	localfile := path.Join(localdir, fmt.Sprintf("%v.%v", pkg.VerSimpleString(), urls[len(urls)-1]))

	err = download(url, localfile)
	if err != nil {
		return err
	}

	r, err := zip.OpenReader(localfile)
	if err != nil {
		return err
	}
	defer r.Close()

	if pkg.Ver != TRUNK {
		return nil
	}

	//return extractPkg(pkg)
	return nil
}

func getDirect(pkg *Pkg) error {
	return getPackage(pkg, pkg.Source.PkgUrl(pkg.Name, pkg.VerString()))
}

/*func getFromSource(pkgName string, ver string, source string) error {
	urlTempl := "https://%v/%v"
	//urlTempl := "https://%v/archive/master.zip"
	url := fmt.Sprintf(urlTempl, source, pkgName)

	return getPackage(pkgName, ver, url)
}*/
