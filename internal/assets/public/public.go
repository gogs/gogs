package public

import (
	"bytes"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

//go:generate go-bindata -nomemcopy -pkg=public -ignore="\\.DS_Store|less" -prefix=../../../public -debug=false -o=public_gen.go ../../../public/...

type fakeDirInfo struct {
	name string
	size int64
}

func (fi fakeDirInfo) Name() string {
	return fi.name
}

func (fi fakeDirInfo) Size() int64 {
	return fi.size
}

func (fi fakeDirInfo) Mode() os.FileMode {
	return os.FileMode(2147484068) // equal os.FileMode(0644)|os.ModeDir
}

func (fi fakeDirInfo) ModTime() time.Time {
	return time.Time{}
}

// IsDir return file whether a directory
func (fi *fakeDirInfo) IsDir() bool {
	return true
}

func (fi fakeDirInfo) Sys() interface{} {
	return nil
}

type assetFile struct {
	*bytes.Reader
	name            string
	childInfos      []os.FileInfo
	childInfoOffset int
}

// Close no need do anything
func (f *assetFile) Close() error {
	return nil
}

// Readdir read dir's children file info
func (f *assetFile) Readdir(count int) ([]os.FileInfo, error) {
	if len(f.childInfos) == 0 {
		return nil, os.ErrNotExist
	}

	if count <= 0 {
		return f.childInfos, nil
	}

	if f.childInfoOffset+count > len(f.childInfos) {
		count = len(f.childInfos) - f.childInfoOffset
	}
	offset := f.childInfoOffset
	f.childInfoOffset += count
	return f.childInfos[offset : offset+count], nil
}

// Stat read file info from asset item
func (f *assetFile) Stat() (os.FileInfo, error) {
	childCount := len(f.childInfos)
	if childCount != 0 {
		return &fakeDirInfo{name: f.name, size: int64(childCount)}, nil
	}
	return AssetInfo(f.name)
}

type assetOperator struct{}

// Open implement http.FileSystem interface
func (f *assetOperator) Open(name string) (http.File, error) {
	if len(name) > 0 && name[0] == '/' {
		name = name[1:]
	}

	var err error
	content, err := Asset(name)
	if err == nil {
		return &assetFile{name: name, Reader: bytes.NewReader(content)}, nil
	}

	// maybe it's directory so get children's file info from the path
	children, err := AssetDir(name)
	if err == nil {
		childInfos := make([]os.FileInfo, 0, len(children))
		for _, child := range children {
			childPath := filepath.Join(name, child)
			info, err := AssetInfo(childPath)
			if err == nil {
				childInfos = append(childInfos, info)
			} else { // not find asset info from Assets so child is a directory
				childInfos = append(childInfos, &fakeDirInfo{name: childPath})
			}
		}
		return &assetFile{name: name, childInfos: childInfos}, nil
	} else {
		// If the error is not found, return an error that will
		// result in a 404 error. Otherwise the server returns
		// a 500 error for files not found.
		if strings.Contains(err.Error(), "not found") {
			return nil, os.ErrNotExist
		}
		return nil, err
	}
}

// FileSystem return a http.FileSystem instance that data backend by asset
func FileSystem() http.FileSystem {
	return &assetOperator{}
}
