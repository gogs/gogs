package web

import (
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/flamego/flamego"
	"unknwon.dev/x/logx"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/lazyregexp"
	"gogs.io/gogs/public"
)

// maxCustomFileSize is the largest static file that admins can upload, e.g., a
// logo for a component to show.
const maxCustomFileSize = 10 << 20

// customFileSegment matches each directory and file name in the path of an
// uploaded file. Names start with a letter or digit, so none is hidden or walks
// up the directory tree.
var customFileSegment = lazyregexp.New(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

// customPublicDir returns the directory of static files that are served ahead
// of the built-in ones, which the Files page of the customization manages.
func customPublicDir() string {
	return filepath.Join(conf.CustomDir(), "public")
}

// customFilePath returns the path on disk of the static file at the slash
// separated path, e.g., "img/logo.png", or false when no uploaded file may have
// the path. A file named index.html is refused, since the static handler
// serves it in place of the page at its directory.
func customFilePath(name string) (string, bool) {
	segments := strings.Split(name, "/")
	if len(segments) > 8 || segments[len(segments)-1] == "index.html" {
		return "", false
	}
	for _, s := range segments {
		if !customFileSegment.MatchString(s) || strings.Contains(s, "..") {
			return "", false
		}
	}
	return filepath.Join(customPublicDir(), filepath.FromSlash(name)), true
}

// isBuiltinFile reports whether a built-in static file has the path, which a
// custom file with the same path replaces.
func isBuiltinFile(name string) bool {
	if _, err := fs.Stat(public.Files, name); err == nil {
		return true
	}
	_, err := fs.Stat(public.WebAssets, "dist/"+name)
	return err == nil
}

type adminCustomFile struct {
	// Path is relative to the custom public directory, e.g., "img/logo.png".
	Path       string    `json:"path"`
	Size       int64     `json:"size"`
	ModifiedAt time.Time `json:"modifiedAt"`
	URL        string    `json:"url"`
	// ReplacesBuiltin is whether the file is served in place of a built-in
	// one, e.g., "img/favicon.png".
	ReplacesBuiltin bool `json:"replacesBuiltin"`
	// Editable is whether the admin panel can replace and delete the file,
	// which a file put there by hand under another name is not.
	Editable bool `json:"editable"`
}

func newAdminCustomFile(name string, fi os.FileInfo) adminCustomFile {
	_, editable := customFilePath(name)
	return adminCustomFile{
		Path:            name,
		Size:            fi.Size(),
		ModifiedAt:      fi.ModTime().UTC(),
		URL:             conf.Server.Subpath + "/" + name,
		ReplacesBuiltin: isBuiltinFile(name),
		Editable:        editable,
	}
}

// getAdminCustomFiles lists the files in the custom public directory, including
// the ones put there by hand.
func getAdminCustomFiles(logger *logx.Logger) (statusCode int, resp []adminCustomFile, err error) {
	resp = []adminCustomFile{}
	root := customPublicDir()
	err = filepath.WalkDir(root, func(fpath string, d fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) && fpath == root {
				return filepath.SkipDir
			}
			return err
		}
		if strings.HasPrefix(d.Name(), ".") && fpath != root {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(root, fpath)
		if err != nil {
			return err
		}
		fi, err := d.Info()
		if err != nil {
			return err
		}
		resp = append(resp, newAdminCustomFile(filepath.ToSlash(rel), fi))
		return nil
	})
	if err != nil {
		logger.Error("Failed to list custom public files", "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "list files")
	}
	slices.SortFunc(resp, func(a, b adminCustomFile) int { return strings.Compare(a.Path, b.Path) })
	return http.StatusOK, resp, nil
}

// limitCustomFileUpload allows a file as large as it may be, with room for the
// multipart framing.
func limitCustomFileUpload(c flamego.Context) {
	r := c.Request().Request
	r.Body = http.MaxBytesReader(c.ResponseWriter(), r.Body, maxCustomFileSize+64<<10)
}

// postAdminCustomFile stores the file uploaded as the "file" form field at the
// path in the "path" form field, replacing a file at the same path.
func postAdminCustomFile(c flamego.Context, logger *logx.Logger) (statusCode int, resp *adminCustomFile, err error) {
	r := c.Request().Request
	file, header, err := r.FormFile("file")
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
	name := path.Clean(strings.Trim(r.FormValue("path"), "/"))
	fpath, ok := customFilePath(name)
	if !ok {
		return http.StatusBadRequest, nil, errors.New("each name in the path may only have letters, digits, dots, dashes and underscores, and must start with a letter or digit, and the file may not be named index.html")
	}

	dir := filepath.Dir(fpath)
	if err = os.MkdirAll(dir, os.ModePerm); err != nil {
		logger.Error("Failed to create custom public directory", "path", dir, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "create directory")
	}
	// Write to a temporary file of its own and rename it, so pages never load
	// a file that is only partly written. Its name starts with a dot, so the
	// list skips it.
	tmp, err := os.CreateTemp(dir, ".upload-*.tmp")
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
		logger.Error("Failed to write custom public file", "path", name, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "write file")
	}
	fi, err := os.Stat(fpath)
	if err != nil {
		logger.Error("Failed to stat custom public file", "path", name, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "stat file")
	}
	f := newAdminCustomFile(name, fi)
	return http.StatusCreated, &f, nil
}

func deleteAdminCustomFile(c flamego.Context, logger *logx.Logger) (statusCode int, resp any, err error) {
	name := c.Param("path")
	fpath, ok := customFilePath(name)
	if !ok {
		return http.StatusNotFound, nil, errors.New("file not found")
	}
	if err = os.Remove(fpath); err != nil {
		if os.IsNotExist(err) {
			return http.StatusNotFound, nil, errors.New("file not found")
		}
		logger.Error("Failed to delete custom public file", "path", name, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "delete file")
	}
	return http.StatusNoContent, nil, nil
}
