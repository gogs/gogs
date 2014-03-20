// Copyright 2013-2014 gopm authors.
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
	"bytes"
	"go/build"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/Unknwon/com"

	"github.com/gpmgo/gopm/log"
)

const VENDOR = ".vendor"

// GetDirsInfo returns os.FileInfo of all sub-directories in root path.
func GetDirsInfo(rootPath string) ([]os.FileInfo, error) {
	if !com.IsDir(rootPath) {
		log.Warn("Directory %s does not exist", rootPath)
		return []os.FileInfo{}, nil
	}

	rootDir, err := os.Open(rootPath)
	if err != nil {
		return nil, err
	}
	defer rootDir.Close()

	dirs, err := rootDir.Readdir(0)
	if err != nil {
		return nil, err
	}

	return dirs, nil
}

// A Source describles a Source code file.
type Source struct {
	SrcName string
	SrcData []byte
}

func (s *Source) Name() string       { return s.SrcName }
func (s *Source) Size() int64        { return int64(len(s.SrcData)) }
func (s *Source) Mode() os.FileMode  { return 0 }
func (s *Source) ModTime() time.Time { return time.Time{} }
func (s *Source) IsDir() bool        { return false }
func (s *Source) Sys() interface{}   { return nil }
func (s *Source) Data() []byte       { return s.SrcData }

type Context struct {
	build.Context
	importPath string
	srcFiles   map[string]*Source
}

func (ctx *Context) readDir(dir string) ([]os.FileInfo, error) {
	fis := make([]os.FileInfo, 0, len(ctx.srcFiles))
	for _, src := range ctx.srcFiles {
		fis = append(fis, src)
	}
	return fis, nil
}

func (ctx *Context) openFile(path string) (r io.ReadCloser, err error) {
	if src, ok := ctx.srcFiles[filepath.Base(path)]; ok {
		return ioutil.NopCloser(bytes.NewReader(src.Data())), nil
	}
	return nil, os.ErrNotExist
}

// GetImports returns package denpendencies.
func GetImports(absPath, importPath string, example, test bool) []string {
	fis, err := GetDirsInfo(absPath)
	if err != nil {
		log.Error("", "Fail to get directory's information")
		log.Fatal("", err.Error())
	}
	absPath += "/"

	ctx := new(Context)
	ctx.importPath = importPath
	ctx.srcFiles = make(map[string]*Source)
	ctx.Context = build.Default
	ctx.JoinPath = path.Join
	ctx.IsAbsPath = path.IsAbs
	ctx.ReadDir = ctx.readDir
	ctx.OpenFile = ctx.openFile

	// TODO: Load too much, need to make sure which is imported which are not.
	dirs := make([]string, 0, 10)
	for _, fi := range fis {
		if strings.Contains(fi.Name(), VENDOR) {
			continue
		}

		if fi.IsDir() {
			dirs = append(dirs, absPath+fi.Name())
			continue
		} else if !test && strings.HasSuffix(fi.Name(), "_test.go") {
			continue
		} else if !strings.HasSuffix(fi.Name(), ".go") || strings.HasPrefix(fi.Name(), ".") ||
			strings.HasPrefix(fi.Name(), "_") {
			continue
		}
		src := &Source{SrcName: fi.Name()}
		src.SrcData, err = ioutil.ReadFile(absPath + fi.Name())
		if err != nil {
			log.Error("", "Fail to read file")
			log.Fatal("", err.Error())
		}
		ctx.srcFiles[fi.Name()] = src
	}

	pkg, err := ctx.ImportDir(absPath, build.AllowBinary)
	if err != nil {
		if _, ok := err.(*build.NoGoError); !ok {
			log.Error("", "Fail to get imports")
			log.Fatal("", err.Error())
		}
	}

	imports := make([]string, 0, len(pkg.Imports))
	for _, p := range pkg.Imports {
		if !IsGoRepoPath(p) && !strings.HasPrefix(p, importPath) {
			imports = append(imports, p)
		}
	}

	if len(dirs) > 0 {
		imports = append(imports, GetAllImports(dirs, importPath, example, test)...)
	}
	return imports
}

// isVcsPath returns true if the directory was created by VCS.
func isVcsPath(dirPath string) bool {
	return strings.Contains(dirPath, "/.git") ||
		strings.Contains(dirPath, "/.hg") ||
		strings.Contains(dirPath, "/.svn")
}

// GetAllImports returns all imports in given directory and all sub-directories.
func GetAllImports(dirs []string, importPath string, example, test bool) (imports []string) {
	for _, d := range dirs {
		if !isVcsPath(d) &&
			!(!example && strings.Contains(d, "example")) {
			imports = append(imports, GetImports(d, importPath, example, test)...)
		}
	}
	return imports
}

// GetGOPATH returns best matched GOPATH.
func GetBestMatchGOPATH(appPath string) string {
	paths := com.GetGOPATHs()
	for _, p := range paths {
		if strings.HasPrefix(p, appPath) {
			return strings.Replace(p, "\\", "/", -1)
		}
	}
	return paths[0]
}

// CheckIsExistWithVCS returns false if directory only has VCS folder,
// or doesn't exist.
func CheckIsExistWithVCS(path string) bool {
	// Check if directory exist.
	if !com.IsExist(path) {
		return false
	}

	// Check if only has VCS folder.
	dirs, err := GetDirsInfo(path)
	if err != nil {
		log.Error("", "Fail to get directory's information")
		log.Fatal("", err.Error())
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
	paths := com.GetGOPATHs()
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
	case strings.HasPrefix(importPath, "github.com") ||
		strings.HasPrefix(importPath, "git.oschina.net"):
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
	"runtime/race":        true,
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

func CheckNodeValue(v string) string {
	if len(v) == 0 {
		return "<UTD>"
	}
	return v
}
