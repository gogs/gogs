package web

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/flamego/flamego"
	"unknwon.dev/x/logx"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/lazyregexp"
)

// maxCustomFileSize is the largest file that admins can upload for the
// components to show, e.g., a logo.
const maxCustomFileSize = 10 << 20

// customFilesURLPath is where the uploaded files are served.
const customFilesURLPath = "/-/custom/files/"

// customFileName matches the names that uploaded files may have. Names start
// with a letter or digit, so none is hidden or walks up the directory tree.
var customFileName = lazyregexp.New(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

func customFilesDir() string {
	return filepath.Join(conf.CustomDir(), "web", "files")
}

// customFilePath returns the path of the uploaded file with the name, or false
// when the name is not one an uploaded file may have.
func customFilePath(name string) (string, bool) {
	if !customFileName.MatchString(name) || strings.Contains(name, "..") {
		return "", false
	}
	return filepath.Join(customFilesDir(), name), true
}

func getCustomFile(c flamego.Context) {
	w, r := c.ResponseWriter(), c.Request().Request
	fpath, ok := customFilePath(c.Param("name"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	f, err := os.Open(fpath)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer func() { _ = f.Close() }()
	fi, err := f.Stat()
	if err != nil || !fi.Mode().IsRegular() {
		http.NotFound(w, r)
		return
	}
	// A file keeps its URL when it is replaced, so browsers check whether it
	// changed before they use their copy.
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeContent(w, r, fi.Name(), fi.ModTime(), f)
}

type adminCustomFile struct {
	Name       string    `json:"name"`
	Size       int64     `json:"size"`
	ModifiedAt time.Time `json:"modifiedAt"`
	URL        string    `json:"url"`
}

func newAdminCustomFile(fi os.FileInfo) adminCustomFile {
	return adminCustomFile{
		Name:       fi.Name(),
		Size:       fi.Size(),
		ModifiedAt: fi.ModTime().UTC(),
		URL:        conf.Server.Subpath + customFilesURLPath + fi.Name(),
	}
}

func getAdminCustomFiles(logger *logx.Logger) (statusCode int, resp []adminCustomFile, err error) {
	entries, err := os.ReadDir(customFilesDir())
	if err != nil && !os.IsNotExist(err) {
		logger.Error("Failed to list customization files", "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "list files")
	}
	resp = []adminCustomFile{}
	for _, entry := range entries {
		if _, ok := customFilePath(entry.Name()); !ok || !entry.Type().IsRegular() {
			continue
		}
		fi, err := entry.Info()
		if err != nil {
			continue
		}
		resp = append(resp, newAdminCustomFile(fi))
	}
	slices.SortFunc(resp, func(a, b adminCustomFile) int { return strings.Compare(a.Name, b.Name) })
	return http.StatusOK, resp, nil
}

// limitCustomFileUpload allows a file as large as it may be, with room for the
// multipart framing.
func limitCustomFileUpload(c flamego.Context) {
	r := c.Request().Request
	r.Body = http.MaxBytesReader(c.ResponseWriter(), r.Body, maxCustomFileSize+64<<10)
}

// postAdminCustomFile stores the file uploaded as the "file" form field under
// its own name, replacing a file of the same name.
func postAdminCustomFile(c flamego.Context, logger *logx.Logger) (statusCode int, resp *adminCustomFile, err error) {
	file, header, err := c.Request().FormFile("file")
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			return http.StatusRequestEntityTooLarge, nil, errors.Newf("file exceeds the size limit of %d MB", maxCustomFileSize>>20)
		}
		return http.StatusBadRequest, nil, errors.Wrap(err, "read file")
	}
	defer func() { _ = file.Close() }()
	if header.Size > maxCustomFileSize {
		return http.StatusRequestEntityTooLarge, nil, errors.Newf("file exceeds the size limit of %d MB", maxCustomFileSize>>20)
	}
	fpath, ok := customFilePath(header.Filename)
	if !ok {
		return http.StatusBadRequest, nil, errors.New("file name may only have letters, digits, dots, dashes and underscores, and must start with a letter or digit")
	}

	dir := filepath.Dir(fpath)
	if err = os.MkdirAll(dir, os.ModePerm); err != nil {
		logger.Error("Failed to create customization files directory", "path", dir, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "create directory")
	}
	// Write to a temporary file of its own and rename it, so pages never load
	// a file that is only partly written.
	tmp, err := os.CreateTemp(filepath.Dir(dir), "file-*.tmp")
	if err != nil {
		logger.Error("Failed to create temporary file", "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "create temporary file")
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	_, err = io.Copy(tmp, io.LimitReader(file, maxCustomFileSize))
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Chmod(tmp.Name(), 0o644)
	}
	if err == nil {
		err = os.Rename(tmp.Name(), fpath)
	}
	if err != nil {
		logger.Error("Failed to write customization file", "name", header.Filename, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "write file")
	}
	fi, err := os.Stat(fpath)
	if err != nil {
		logger.Error("Failed to stat customization file", "name", header.Filename, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "stat file")
	}
	f := newAdminCustomFile(fi)
	return http.StatusCreated, &f, nil
}

func deleteAdminCustomFile(c flamego.Context, logger *logx.Logger) (statusCode int, resp any, err error) {
	fpath, ok := customFilePath(c.Param("name"))
	if !ok {
		return http.StatusNotFound, nil, errors.New("file not found")
	}
	if err = os.Remove(fpath); err != nil {
		if os.IsNotExist(err) {
			return http.StatusNotFound, nil, errors.New("file not found")
		}
		logger.Error("Failed to delete customization file", "path", fpath, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "delete file")
	}
	return http.StatusNoContent, nil, nil
}
