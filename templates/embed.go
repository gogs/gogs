package templates

import (
	"bytes"
	"embed"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/cockroachdb/errors"
	"gopkg.in/macaron.v1"

	"gogs.io/gogs/internal/osx"
)

//go:embed **/*
var files embed.FS

// fileSystem implements the macaron.TemplateFileSystem interface.
type fileSystem struct {
	files []macaron.TemplateFile
}

func (fs *fileSystem) ListFiles() []macaron.TemplateFile {
	return fs.files
}

func (fs *fileSystem) Get(name string) (io.Reader, error) {
	for i := range fs.files {
		if fs.files[i].Name()+fs.files[i].Ext() == name {
			return bytes.NewReader(fs.files[i].Data()), nil
		}
	}
	return nil, errors.Newf("file %q not found", name)
}

func mustNames(fsys fs.FS) []string {
	var names []string
	walkDirFunc := func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			names = append(names, path)
		}
		return nil
	}
	if err := fs.WalkDir(fsys, ".", walkDirFunc); err != nil {
		panic("assetNames failure: " + err.Error())
	}
	return names
}

// MailFileNames returns the embedded template file paths under "mail/",
// each relative to the "mail" directory (e.g. "auth/activate.tmpl").
func MailFileNames() []string {
	var names []string
	for _, name := range mustNames(files) {
		if rel, ok := strings.CutPrefix(name, "mail/"); ok {
			names = append(names, rel)
		}
	}
	return names
}

// ReadMailFile returns the embedded mail template bytes at the given path
// relative to the "mail" directory.
func ReadMailFile(name string) ([]byte, error) {
	return files.ReadFile(path.Join("mail", name))
}

// ReadInjectFile returns the content of the named injection template.
func ReadInjectFile(customDir, name string) ([]byte, error) {
	fpath := path.Join(customDir, "inject", name)
	if osx.IsFile(fpath) {
		return os.ReadFile(fpath)
	}
	return files.ReadFile(path.Join("inject", name))
}

// NewTemplateFileSystem returns a macaron.TemplateFileSystem instance for embedded assets.
// The argument "dir" can be used to serve subset of embedded assets. Template file
// found under the "customDir" on disk has higher precedence over embedded assets.
func NewTemplateFileSystem(dir, customDir string) macaron.TemplateFileSystem {
	if dir != "" && !strings.HasSuffix(dir, "/") {
		dir += "/"
	}

	var err error
	var tmplFiles []macaron.TemplateFile
	names := mustNames(files)
	for _, name := range names {
		if !strings.HasPrefix(name, dir) || isMail(name) {
			continue
		}
		// Check if corresponding custom file exists
		var data []byte
		fpath := path.Join(customDir, name)
		if osx.IsFile(fpath) {
			data, err = os.ReadFile(fpath)
		} else {
			data, err = files.ReadFile(name)
		}
		if err != nil {
			panic(err)
		}
		name = strings.TrimPrefix(name, dir)
		ext := path.Ext(name)
		name = strings.TrimSuffix(name, ext)
		tmplFiles = append(tmplFiles, macaron.NewTplFile(name, data, ext))
	}
	return &fileSystem{files: tmplFiles}
}

// isMail reports whether the template file is an email. Emails are rendered by
// internal/email with functions of its own, so the page renderer skips them.
func isMail(name string) bool {
	return strings.HasPrefix(filepath.ToSlash(name), "mail/")
}

// diskFileSystem lists the page templates on disk, without the emails.
type diskFileSystem struct {
	opt macaron.RenderOptions
}

// NewDiskTemplateFileSystem returns a macaron.TemplateFileSystem for the
// template files on disk in opt.Directory and opt.AppendDirectories. It reads
// the directories again on every listing, so the renderer picks up edits.
func NewDiskTemplateFileSystem(opt macaron.RenderOptions) macaron.TemplateFileSystem {
	if len(opt.Extensions) == 0 {
		opt.Extensions = []string{".tmpl", ".html"}
	}
	return diskFileSystem{opt: opt}
}

func (fs diskFileSystem) ListFiles() []macaron.TemplateFile {
	return slices.DeleteFunc(macaron.NewTemplateFileSystem(fs.opt, false).ListFiles(), func(f macaron.TemplateFile) bool {
		return isMail(f.Name())
	})
}

func (fs diskFileSystem) Get(name string) (io.Reader, error) {
	return macaron.NewTemplateFileSystem(fs.opt, false).Get(name)
}
