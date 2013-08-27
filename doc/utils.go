// Copyright 2013 gopm authors.
//
// Licensed under the Apache License, Version 2.0 (the "License"): you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
// WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
// License for the specific language governing permissions and limitations
// under the License.

package doc

import (
	"fmt"
	"os"
	"path"
	"regexp"
	"runtime"
	"strings"
	"syscall"
)

// IsExist returns if a file or directory exists
func IsExist(path string) bool {
	_, err := os.Stat(path)
	return err == nil || os.IsExist(err)
}

// Non-Windows.
const (
	Gray = uint8(iota + 90)
	Red
	Green
	Yellow
	Blue
	Magenta
	//NRed      = uint8(31) // Normal
	EndColor = "\033[0m"
)

// Windows.
const (
	WDefault = uintptr(iota)
	WBlue
	WGreen
	WCyan
	WRed
	WPurple
	WYellow
	WGray
	WSilver
	WLightBlue
	WLime
	WLightCyan
	WLightRed
	WLightPurple
	WLightYellow
	WWhite
)

// ColorLog colors log and print to stdout.
// Log format: <level> <content [highlight][path]> [ error ].
// Level: TRAC -> blue; ERRO -> red; WARN -> Magenta; SUCC -> green; others -> default.
// Content: default; path: yellow; error -> red.
// Level has to be surrounded by "[" and "]".
// Highlights have to be surrounded by "# " and " #"(space).
// Paths have to be surrounded by "( " and " )"(sapce).
// Errors have to be surrounded by "[ " and " ]"(space).
func ColorLog(format string, a ...interface{}) {
	log := fmt.Sprintf(format, a...)
	if runtime.GOOS != "windows" {
		var clog string

		// Level.
		i := strings.Index(log, "]")
		if log[0] == '[' && i > -1 {
			clog += "[" + getColorLevel(log[1:i]) + "]"
		}

		log = log[i+1:]

		// Error.
		log = strings.Replace(log, "[ ", fmt.Sprintf("[\033[%dm", Red), -1)
		log = strings.Replace(log, " ]", EndColor+"]", -1)

		// Path.
		log = strings.Replace(log, "( ", fmt.Sprintf("(\033[%dm", Yellow), -1)
		log = strings.Replace(log, " )", EndColor+")", -1)

		// Highlights.
		log = strings.Replace(log, "# ", fmt.Sprintf("\033[%dm", Gray), -1)
		log = strings.Replace(log, " #", EndColor, -1)

		log = clog + log
	} else {
		// Level.
		i := strings.Index(log, "]")
		if log[0] == '[' && i > -1 {
			fmt.Print("[")
			printColorLevel(log[1:i])
			fmt.Print("]")
		}

		log = log[i+1:]
	}

	fmt.Print(log)
}

// getColorLevel returns colored level string by given level.
func getColorLevel(level string) string {
	level = strings.ToUpper(level)
	switch level {
	case "TRAC":
		return fmt.Sprintf("\033[%dm%s\033[0m", Blue, level)
	case "ERRO":
		return fmt.Sprintf("\033[%dm%s\033[0m", Red, level)
	case "WARN":
		return fmt.Sprintf("\033[%dm%s\033[0m", Magenta, level)
	case "SUCC":
		return fmt.Sprintf("\033[%dm%s\033[0m", Green, level)
	default:
		return level
	}
}

// printColorLevel prints color level prompt, this is only for Windows.
func printColorLevel(level string) {
	cc := WDefault
	level = strings.ToUpper(level)
	switch level {
	case "TRAC":
		cc = WBlue
	case "ERRO":
		cc = WRed
	case "WARN":
		cc = WPurple
	case "SUCC":
		cc = WGreen
	default:
		cc = WWhite
	}

	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	proc := kernel32.NewProc("SetConsoleTextAttribute")
	handle, _, _ := proc.Call(uintptr(syscall.Stdout), uintptr(cc))
	fmt.Print(level)
	handle, _, _ = proc.Call(uintptr(syscall.Stdout), uintptr(WSilver))
	CloseHandle := kernel32.NewProc("CloseHandle")
	CloseHandle.Call(handle)
}

// GetGOPATH returns all paths in GOPATH variable.
func GetGOPATH() []string {
	gopath := os.Getenv("GOPATH")
	var paths []string
	if runtime.GOOS == "windows" {
		gopath = strings.Replace(gopath, "\\", "/", -1)
		paths = strings.Split(gopath, ";")
	} else {
		paths = strings.Split(gopath, ":")
	}
	return paths
}

// GetGOPATH returns best matched GOPATH.
func GetBestMatchGOPATH(appPath string) string {
	paths := GetGOPATH()
	for _, p := range paths {
		if strings.HasPrefix(p, appPath) {
			return strings.Replace(p, "\\", "/", -1)
		}
	}
	return paths[0]
}

// GetDirsInfo returns os.FileInfo of all sub-directories in root path.
func GetDirsInfo(rootPath string) ([]os.FileInfo, error) {
	rootDir, err := os.Open(rootPath)
	if err != nil {
		return nil, err
	}
	defer rootDir.Close()

	dirs, err := rootDir.Readdir(0)
	if err != nil {
		return nil, err
	}

	return dirs, err
}

// CheckIsExistWithVCS returns false if directory only has VCS folder,
// or doesn't exist.
func CheckIsExistWithVCS(path string) bool {
	// Check if directory exist.
	if !IsExist(path) {
		return false
	}

	// Check if only has VCS folder.
	dirs, err := GetDirsInfo(path)
	if err != nil {
		ColorLog("[ERRO] CheckIsExistWithVCS -> [ %s ]\n", err)
		return false
	}

	if len(dirs) > 1 {
		return true
	} else if len(dirs) == 0 {
		return false
	}

	switch dirs[0].Name() {
	case ".git", ".hg", ".svn":
		return false
	}

	return true
}

// CheckIsExistInGOPATH checks if given package import path exists in any path in GOPATH/src,
// and returns corresponding GOPATH.
func CheckIsExistInGOPATH(importPath string) (string, bool) {
	paths := GetGOPATH()
	for _, p := range paths {
		if CheckIsExistWithVCS(p + "/src/" + importPath + "/") {
			return p, true
		}
	}
	return "", false
}

// GetProjectPath returns project path of import path.
func GetProjectPath(importPath string) (projectPath string) {
	projectPath = importPath

	// Check project hosting.
	switch {
	case strings.HasPrefix(importPath, "github.com"):
		projectPath = joinPath(importPath, 3)
	case strings.HasPrefix(importPath, "code.google.com"):
		projectPath = joinPath(importPath, 3)
	case strings.HasPrefix(importPath, "bitbucket.org"):
		projectPath = joinPath(importPath, 3)
	case strings.HasPrefix(importPath, "launchpad.net"):
		projectPath = joinPath(importPath, 2)
	}

	return projectPath
}

func joinPath(importPath string, num int) string {
	subdirs := strings.Split(importPath, "/")
	if len(subdirs) > num {
		return strings.Join(subdirs[:num], "/")
	}
	return importPath
}

var validTLD = map[string]bool{
	// curl http://data.iana.org/TLD/tlds-alpha-by-domain.txt | sed  -e '/#/ d' -e 's/.*/"&": true,/' | tr [:upper:] [:lower:]
	".ac":                     true,
	".ad":                     true,
	".ae":                     true,
	".aero":                   true,
	".af":                     true,
	".ag":                     true,
	".ai":                     true,
	".al":                     true,
	".am":                     true,
	".an":                     true,
	".ao":                     true,
	".aq":                     true,
	".ar":                     true,
	".arpa":                   true,
	".as":                     true,
	".asia":                   true,
	".at":                     true,
	".au":                     true,
	".aw":                     true,
	".ax":                     true,
	".az":                     true,
	".ba":                     true,
	".bb":                     true,
	".bd":                     true,
	".be":                     true,
	".bf":                     true,
	".bg":                     true,
	".bh":                     true,
	".bi":                     true,
	".biz":                    true,
	".bj":                     true,
	".bm":                     true,
	".bn":                     true,
	".bo":                     true,
	".br":                     true,
	".bs":                     true,
	".bt":                     true,
	".bv":                     true,
	".bw":                     true,
	".by":                     true,
	".bz":                     true,
	".ca":                     true,
	".cat":                    true,
	".cc":                     true,
	".cd":                     true,
	".cf":                     true,
	".cg":                     true,
	".ch":                     true,
	".ci":                     true,
	".ck":                     true,
	".cl":                     true,
	".cm":                     true,
	".cn":                     true,
	".co":                     true,
	".com":                    true,
	".coop":                   true,
	".cr":                     true,
	".cu":                     true,
	".cv":                     true,
	".cw":                     true,
	".cx":                     true,
	".cy":                     true,
	".cz":                     true,
	".de":                     true,
	".dj":                     true,
	".dk":                     true,
	".dm":                     true,
	".do":                     true,
	".dz":                     true,
	".ec":                     true,
	".edu":                    true,
	".ee":                     true,
	".eg":                     true,
	".er":                     true,
	".es":                     true,
	".et":                     true,
	".eu":                     true,
	".fi":                     true,
	".fj":                     true,
	".fk":                     true,
	".fm":                     true,
	".fo":                     true,
	".fr":                     true,
	".ga":                     true,
	".gb":                     true,
	".gd":                     true,
	".ge":                     true,
	".gf":                     true,
	".gg":                     true,
	".gh":                     true,
	".gi":                     true,
	".gl":                     true,
	".gm":                     true,
	".gn":                     true,
	".gov":                    true,
	".gp":                     true,
	".gq":                     true,
	".gr":                     true,
	".gs":                     true,
	".gt":                     true,
	".gu":                     true,
	".gw":                     true,
	".gy":                     true,
	".hk":                     true,
	".hm":                     true,
	".hn":                     true,
	".hr":                     true,
	".ht":                     true,
	".hu":                     true,
	".id":                     true,
	".ie":                     true,
	".il":                     true,
	".im":                     true,
	".in":                     true,
	".info":                   true,
	".int":                    true,
	".io":                     true,
	".iq":                     true,
	".ir":                     true,
	".is":                     true,
	".it":                     true,
	".je":                     true,
	".jm":                     true,
	".jo":                     true,
	".jobs":                   true,
	".jp":                     true,
	".ke":                     true,
	".kg":                     true,
	".kh":                     true,
	".ki":                     true,
	".km":                     true,
	".kn":                     true,
	".kp":                     true,
	".kr":                     true,
	".kw":                     true,
	".ky":                     true,
	".kz":                     true,
	".la":                     true,
	".lb":                     true,
	".lc":                     true,
	".li":                     true,
	".lk":                     true,
	".lr":                     true,
	".ls":                     true,
	".lt":                     true,
	".lu":                     true,
	".lv":                     true,
	".ly":                     true,
	".ma":                     true,
	".mc":                     true,
	".md":                     true,
	".me":                     true,
	".mg":                     true,
	".mh":                     true,
	".mil":                    true,
	".mk":                     true,
	".ml":                     true,
	".mm":                     true,
	".mn":                     true,
	".mo":                     true,
	".mobi":                   true,
	".mp":                     true,
	".mq":                     true,
	".mr":                     true,
	".ms":                     true,
	".mt":                     true,
	".mu":                     true,
	".museum":                 true,
	".mv":                     true,
	".mw":                     true,
	".mx":                     true,
	".my":                     true,
	".mz":                     true,
	".na":                     true,
	".name":                   true,
	".nc":                     true,
	".ne":                     true,
	".net":                    true,
	".nf":                     true,
	".ng":                     true,
	".ni":                     true,
	".nl":                     true,
	".no":                     true,
	".np":                     true,
	".nr":                     true,
	".nu":                     true,
	".nz":                     true,
	".om":                     true,
	".org":                    true,
	".pa":                     true,
	".pe":                     true,
	".pf":                     true,
	".pg":                     true,
	".ph":                     true,
	".pk":                     true,
	".pl":                     true,
	".pm":                     true,
	".pn":                     true,
	".post":                   true,
	".pr":                     true,
	".pro":                    true,
	".ps":                     true,
	".pt":                     true,
	".pw":                     true,
	".py":                     true,
	".qa":                     true,
	".re":                     true,
	".ro":                     true,
	".rs":                     true,
	".ru":                     true,
	".rw":                     true,
	".sa":                     true,
	".sb":                     true,
	".sc":                     true,
	".sd":                     true,
	".se":                     true,
	".sg":                     true,
	".sh":                     true,
	".si":                     true,
	".sj":                     true,
	".sk":                     true,
	".sl":                     true,
	".sm":                     true,
	".sn":                     true,
	".so":                     true,
	".sr":                     true,
	".st":                     true,
	".su":                     true,
	".sv":                     true,
	".sx":                     true,
	".sy":                     true,
	".sz":                     true,
	".tc":                     true,
	".td":                     true,
	".tel":                    true,
	".tf":                     true,
	".tg":                     true,
	".th":                     true,
	".tj":                     true,
	".tk":                     true,
	".tl":                     true,
	".tm":                     true,
	".tn":                     true,
	".to":                     true,
	".tp":                     true,
	".tr":                     true,
	".travel":                 true,
	".tt":                     true,
	".tv":                     true,
	".tw":                     true,
	".tz":                     true,
	".ua":                     true,
	".ug":                     true,
	".uk":                     true,
	".us":                     true,
	".uy":                     true,
	".uz":                     true,
	".va":                     true,
	".vc":                     true,
	".ve":                     true,
	".vg":                     true,
	".vi":                     true,
	".vn":                     true,
	".vu":                     true,
	".wf":                     true,
	".ws":                     true,
	".xn--0zwm56d":            true,
	".xn--11b5bs3a9aj6g":      true,
	".xn--3e0b707e":           true,
	".xn--45brj9c":            true,
	".xn--80akhbyknj4f":       true,
	".xn--80ao21a":            true,
	".xn--90a3ac":             true,
	".xn--9t4b11yi5a":         true,
	".xn--clchc0ea0b2g2a9gcd": true,
	".xn--deba0ad":            true,
	".xn--fiqs8s":             true,
	".xn--fiqz9s":             true,
	".xn--fpcrj9c3d":          true,
	".xn--fzc2c9e2c":          true,
	".xn--g6w251d":            true,
	".xn--gecrj9c":            true,
	".xn--h2brj9c":            true,
	".xn--hgbk6aj7f53bba":     true,
	".xn--hlcj6aya9esc7a":     true,
	".xn--j6w193g":            true,
	".xn--jxalpdlp":           true,
	".xn--kgbechtv":           true,
	".xn--kprw13d":            true,
	".xn--kpry57d":            true,
	".xn--lgbbat1ad8j":        true,
	".xn--mgb9awbf":           true,
	".xn--mgbaam7a8h":         true,
	".xn--mgbayh7gpa":         true,
	".xn--mgbbh1a71e":         true,
	".xn--mgbc0a9azcg":        true,
	".xn--mgberp4a5d4ar":      true,
	".xn--mgbx4cd0ab":         true,
	".xn--o3cw4h":             true,
	".xn--ogbpf8fl":           true,
	".xn--p1ai":               true,
	".xn--pgbs0dh":            true,
	".xn--s9brj9c":            true,
	".xn--wgbh1c":             true,
	".xn--wgbl6a":             true,
	".xn--xkc2al3hye2a":       true,
	".xn--xkc2dl3a5ee0h":      true,
	".xn--yfro4i67o":          true,
	".xn--ygbi2ammx":          true,
	".xn--zckzah":             true,
	".xxx":                    true,
	".ye":                     true,
	".yt":                     true,
	".za":                     true,
	".zm":                     true,
	".zw":                     true,
}

var (
	validHost        = regexp.MustCompile(`^[-a-z0-9]+(?:\.[-a-z0-9]+)+$`)
	validPathElement = regexp.MustCompile(`^[-A-Za-z0-9~+][-A-Za-z0-9_.]*$`)
)

// IsValidRemotePath returns true if importPath is structurally valid for "go get".
func IsValidRemotePath(importPath string) bool {

	parts := strings.Split(importPath, "/")

	if len(parts) <= 1 {
		// Import path must contain at least one "/".
		return false
	}

	if !validTLD[path.Ext(parts[0])] {
		return false
	}

	if !validHost.MatchString(parts[0]) {
		return false
	}
	for _, part := range parts[1:] {
		if !validPathElement.MatchString(part) || part == "testdata" {
			return false
		}
	}

	return true
}

var standardPath = map[string]bool{
	"builtin": true,

	// go list -f '"{{.ImportPath}}": true,'  std | grep -v 'cmd/|exp/'
	"cmd/api":             true,
	"cmd/cgo":             true,
	"cmd/fix":             true,
	"cmd/go":              true,
	"cmd/godoc":           true,
	"cmd/gofmt":           true,
	"cmd/vet":             true,
	"cmd/yacc":            true,
	"archive/tar":         true,
	"archive/zip":         true,
	"bufio":               true,
	"bytes":               true,
	"compress/bzip2":      true,
	"compress/flate":      true,
	"compress/gzip":       true,
	"compress/lzw":        true,
	"compress/zlib":       true,
	"container/heap":      true,
	"container/list":      true,
	"container/ring":      true,
	"crypto":              true,
	"crypto/aes":          true,
	"crypto/cipher":       true,
	"crypto/des":          true,
	"crypto/dsa":          true,
	"crypto/ecdsa":        true,
	"crypto/elliptic":     true,
	"crypto/hmac":         true,
	"crypto/md5":          true,
	"crypto/rand":         true,
	"crypto/rc4":          true,
	"crypto/rsa":          true,
	"crypto/sha1":         true,
	"crypto/sha256":       true,
	"crypto/sha512":       true,
	"crypto/subtle":       true,
	"crypto/tls":          true,
	"crypto/x509":         true,
	"crypto/x509/pkix":    true,
	"database/sql":        true,
	"database/sql/driver": true,
	"debug/dwarf":         true,
	"debug/elf":           true,
	"debug/gosym":         true,
	"debug/macho":         true,
	"debug/pe":            true,
	"encoding/ascii85":    true,
	"encoding/asn1":       true,
	"encoding/base32":     true,
	"encoding/base64":     true,
	"encoding/binary":     true,
	"encoding/csv":        true,
	"encoding/gob":        true,
	"encoding/hex":        true,
	"encoding/json":       true,
	"encoding/pem":        true,
	"encoding/xml":        true,
	"errors":              true,
	"expvar":              true,
	"flag":                true,
	"fmt":                 true,
	"go/ast":              true,
	"go/build":            true,
	"go/doc":              true,
	"go/format":           true,
	"go/parser":           true,
	"go/printer":          true,
	"go/scanner":          true,
	"go/token":            true,
	"hash":                true,
	"hash/adler32":        true,
	"hash/crc32":          true,
	"hash/crc64":          true,
	"hash/fnv":            true,
	"html":                true,
	"html/template":       true,
	"image":               true,
	"image/color":         true,
	"image/draw":          true,
	"image/gif":           true,
	"image/jpeg":          true,
	"image/png":           true,
	"index/suffixarray":   true,
	"io":                  true,
	"io/ioutil":           true,
	"log":                 true,
	"log/syslog":          true,
	"math":                true,
	"math/big":            true,
	"math/cmplx":          true,
	"math/rand":           true,
	"mime":                true,
	"mime/multipart":      true,
	"net":                 true,
	"net/http":            true,
	"net/http/cgi":        true,
	"net/http/cookiejar":  true,
	"net/http/fcgi":       true,
	"net/http/httptest":   true,
	"net/http/httputil":   true,
	"net/http/pprof":      true,
	"net/mail":            true,
	"net/rpc":             true,
	"net/rpc/jsonrpc":     true,
	"net/smtp":            true,
	"net/textproto":       true,
	"net/url":             true,
	"os":                  true,
	"os/exec":             true,
	"os/signal":           true,
	"os/user":             true,
	"path":                true,
	"path/filepath":       true,
	"reflect":             true,
	"regexp":              true,
	"regexp/syntax":       true,
	"runtime":             true,
	"runtime/cgo":         true,
	"runtime/debug":       true,
	"runtime/pprof":       true,
	"sort":                true,
	"strconv":             true,
	"strings":             true,
	"sync":                true,
	"sync/atomic":         true,
	"syscall":             true,
	"testing":             true,
	"testing/iotest":      true,
	"testing/quick":       true,
	"text/scanner":        true,
	"text/tabwriter":      true,
	"text/template":       true,
	"text/template/parse": true,
	"time":                true,
	"unicode":             true,
	"unicode/utf16":       true,
	"unicode/utf8":        true,
	"unsafe":              true,
}

// IsGoRepoPath returns true if package is from standard library.
func IsGoRepoPath(importPath string) bool {
	return standardPath[importPath]
}
