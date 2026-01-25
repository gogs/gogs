package templates

import (
	"embed"
	"io/fs"
	"os"
	"path"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/flamego/template"

	"gogs.io/gogs/internal/osutil"
)

//go:embed *.tmpl **/*
var files embed.FS

// templateFile implements the template.File interface.
type templateFile struct {
	name string
	data []byte
	ext  string
}

func (tf *templateFile) Name() string {
	return tf.name
}

func (tf *templateFile) Data() ([]byte, error) {
	return tf.data, nil
}

func (tf *templateFile) Ext() string {
	return tf.ext
}

// fileSystem implements the template.FileSystem interface.
type fileSystem struct {
	files []template.File
}

func (fs *fileSystem) Files() []template.File {
	return fs.files
}

func mustNames(fsys fs.FS) []string {
	var names []string
	walkDirFunc := func(path string, d fs.DirEntry, err error) error {
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

// NewTemplateFileSystem returns a template.FileSystem instance for embedded assets.
// The argument "dir" can be used to serve subset of embedded assets. Template file
// found under the "customDir" on disk has higher precedence over embedded assets.
func NewTemplateFileSystem(dir, customDir string) template.FileSystem {
	if dir != "" && !strings.HasSuffix(dir, "/") {
		dir += "/"
	}

	var err error
	var tmplFiles []template.File
	names := mustNames(files)
	for _, name := range names {
		if !strings.HasPrefix(name, dir) {
			continue
		}
		// Check if corresponding custom file exists
		var data []byte
		fpath := path.Join(customDir, name)
		if osutil.IsFile(fpath) {
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
		tmplFiles = append(tmplFiles, &templateFile{
			name: name,
			data: data,
			ext:  ext,
		})
	}
	return &fileSystem{files: tmplFiles}
}
