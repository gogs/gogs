package templates

import (
	"embed"
	"io/fs"
	"os"
	"path"
	"strings"

	"gogs.io/gogs/internal/osx"
)

//go:embed **/*
var files embed.FS

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
