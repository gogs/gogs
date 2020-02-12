package templates

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"path/filepath"

	"github.com/unknwon/com"
	"gopkg.in/macaron.v1"
)

//go:generate go-bindata -nomemcopy -ignore="\\.DS_Store" -pkg=templates -prefix=../../../templates -debug=false -o=templates_gen.go ../../../templates/...

// tplFileSystem implements TemplateFileSystem interface.
type tplFileSystem struct {
	files []macaron.TemplateFile
}

func (fs *tplFileSystem) ListFiles() []macaron.TemplateFile {
	return fs.files
}

func (fs *tplFileSystem) Get(name string) (io.Reader, error) {
	for i := range fs.files {
		if fs.files[i].Name()+fs.files[i].Ext() == name {
			return bytes.NewReader(fs.files[i].Data()), nil
		}
	}
	return nil, fmt.Errorf("file '%s' not found", name)
}

// NewTemplateFileSystem creates new template file system with given options.
func NewTemplateFileSystem(appendDirs []string, exts []string, omitData bool) macaron.TemplateFileSystem {
	fs := &tplFileSystem{}
	fs.files = make([]macaron.TemplateFile, 0, 10)

	// Directories are composed in reverse order because later one overwrites previous ones,
	// so once found, we can directly jump out of the loop.
	dirs := make([]string, 0, len(appendDirs))
	for i := len(appendDirs) - 1; i >= 0; i-- {
		dirs = append(dirs, appendDirs[i])
	}

	var err error
	for i := range dirs {
		// Skip ones that does not exists for symlink test,
		// but allow non-symlink ones added after start.
		if !com.IsExist(dirs[i]) {
			continue
		}

		dirs[i], err = filepath.EvalSymlinks(dirs[i])
		if err != nil {
			panic("EvalSymlinks(" + dirs[i] + "): " + err.Error())
		}
	}

	relPaths := AssetNames()
	for _, path := range relPaths {
		ext := macaron.GetExt(path)
		for _, extension := range exts {
			if ext != extension {
				continue
			}
			var data []byte
			if !omitData {
				// Loop over candidates of directory, break out once found.
				// The file always exists because it's inside the walk function,
				// and read original file is the worst case.
				for _, dir := range dirs {
					filePath := filepath.Join(dir, path)
					if !com.IsFile(filePath) {
						continue
					}
					data, err = ioutil.ReadFile(filePath)
					break
				}
				if err == nil && len(data) == 0 {
					data, err = Asset(path)
				}
				if err != nil {
					panic("NewTemplateFileSystem: " + err.Error())
				}
			}
			name := filepath.ToSlash(path[0 : len(path)-len(ext)])
			fs.files = append(fs.files, macaron.NewTplFile(name, data, ext))
		}
	}
	return fs
}
