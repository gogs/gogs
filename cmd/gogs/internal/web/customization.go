package web

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/cockroachdb/errors"
	"github.com/flamego/flamego"
	"unknwon.dev/x/logx"

	"gogs.io/gogs/internal/conf"
)

// customParts are the parts of the site that admins can replace, each with a
// script of its own.
var customParts = []string{"landing", "navbar", "footer"}

// maxCustomPartSize is the largest script of a part that is served, so a large
// file cannot slow down every page.
const maxCustomPartSize = 256 << 10

// customScriptURLPath is where the script that combines the parts is served.
const customScriptURLPath = "/-/custom.js"

// customPreviewURLPath is the page the customization editor frames to preview
// a draft.
const customPreviewURLPath = "/-/customize/preview"

func customPartPath(name string) string {
	return filepath.Join(conf.CustomDir(), "web", name+".js")
}

// customBundle is the module that combines the scripts of the customized
// parts. It exports each script as a data URL under the name of its part, so
// the browser gets every part with one request and still imports each one as a
// module of its own.
type customBundle struct {
	// key tells the files the bundle was made from apart from their later
	// versions.
	key     string
	script  []byte
	version string
}

var customBundleCache struct {
	sync.Mutex
	bundle *customBundle
}

// currentCustomBundle returns the bundle of the scripts of the customized
// parts, or nil when no part is customized. It only stats the files unless one
// of them changed since the last call, so edits on disk apply on the next page
// load without a restart.
func currentCustomBundle() (*customBundle, error) {
	var key strings.Builder
	var names []string
	for _, name := range customParts {
		fi, err := os.Stat(customPartPath(name))
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, errors.Wrapf(err, "stat %q", name)
		}
		if fi.Size() > maxCustomPartSize {
			return nil, errors.Newf("script of %q has %d bytes, more than the limit of %d bytes", name, fi.Size(), maxCustomPartSize)
		}
		fmt.Fprintf(&key, "%s:%d:%d;", name, fi.Size(), fi.ModTime().UnixNano())
		names = append(names, name)
	}
	if len(names) == 0 {
		return nil, nil
	}

	customBundleCache.Lock()
	defer customBundleCache.Unlock()
	if b := customBundleCache.bundle; b != nil && b.key == key.String() {
		return b, nil
	}
	var script strings.Builder
	for _, name := range names {
		source, err := os.ReadFile(customPartPath(name))
		if err != nil {
			return nil, errors.Wrapf(err, "read %q", name)
		}
		fmt.Fprintf(&script, "export const %s = \"data:text/javascript;base64,%s\";\n", name, base64.StdEncoding.EncodeToString(source))
	}
	sum := sha256.Sum256([]byte(script.String()))
	b := &customBundle{key: key.String(), script: []byte(script.String()), version: hex.EncodeToString(sum[:6])}
	customBundleCache.bundle = b
	return b, nil
}

// customScriptURL returns the URL of the current customization bundle, which
// changes with its content, or an empty string when no part is customized.
func customScriptURL(logger *logx.Logger) string {
	b, err := currentCustomBundle()
	if err != nil {
		logger.Warn("Failed to load customized parts", "error", err)
		return ""
	}
	if b == nil {
		return ""
	}
	return conf.Server.Subpath + customScriptURLPath + "?v=" + b.version
}

func getCustomScript(logger *logx.Logger, c flamego.Context) {
	w := c.ResponseWriter()
	b, err := currentCustomBundle()
	if err != nil {
		logger.Error("Failed to load customized parts", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	if b == nil {
		http.NotFound(w, c.Request().Request)
		return
	}
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	// A URL with the version of the current content never changes, while one
	// without it, or with an old one, must be asked for again.
	if c.Query("v") == b.version {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "no-cache")
	}
	_, _ = w.Write(b.script)
}

// allowSameOriginFraming lets the customization editor frame the page that
// previews a draft, which every other page of the site refuses.
func allowSameOriginFraming(w http.ResponseWriter) {
	w.Header().Set("X-Frame-Options", "sameorigin")
}

type adminCustomPart struct {
	Name string `json:"name"`
	// Source is the published script of the part, empty when the part is
	// built-in.
	Source string `json:"source"`
}

type adminCustomization struct {
	Parts []adminCustomPart `json:"parts"`
}

func getAdminCustomization(logger *logx.Logger) (statusCode int, resp *adminCustomization, err error) {
	resp = &adminCustomization{Parts: make([]adminCustomPart, 0, len(customParts))}
	for _, name := range customParts {
		source, err := os.ReadFile(customPartPath(name))
		if err != nil && !os.IsNotExist(err) {
			logger.Error("Failed to read customized part", "part", name, "error", err)
			return http.StatusInternalServerError, nil, errors.Wrapf(err, "read %q", name)
		}
		resp.Parts = append(resp.Parts, adminCustomPart{Name: name, Source: string(source)})
	}
	return http.StatusOK, resp, nil
}

// limitCustomizationContent allows the script of a part as large as it may be,
// with room for escaping it in JSON.
func limitCustomizationContent(c flamego.Context) {
	r := c.Request().Request
	r.Body = http.MaxBytesReader(c.ResponseWriter(), r.Body, 2*maxCustomPartSize)
}

type adminCustomPartRequest struct {
	Source string `json:"source"`
}

func putAdminCustomPart(logger *logx.Logger, c flamego.Context, req adminCustomPartRequest) (statusCode int, resp *adminCustomPart, err error) {
	name := c.Param("part")
	if !slices.Contains(customParts, name) {
		return http.StatusNotFound, nil, errors.New("part not found")
	}
	if len(req.Source) > maxCustomPartSize {
		return http.StatusRequestEntityTooLarge, nil, errors.Newf("script exceeds the limit of %d bytes", maxCustomPartSize)
	}

	fpath := customPartPath(name)
	// An empty script brings back the built-in part.
	if strings.TrimSpace(req.Source) == "" {
		if err = os.Remove(fpath); err != nil && !os.IsNotExist(err) {
			logger.Error("Failed to remove customized part", "part", name, "error", err)
			return http.StatusInternalServerError, nil, errors.Wrap(err, "remove script")
		}
		return http.StatusOK, &adminCustomPart{Name: name}, nil
	}

	dir := filepath.Dir(fpath)
	if err = os.MkdirAll(dir, os.ModePerm); err != nil {
		logger.Error("Failed to create customization directory", "path", dir, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "create directory")
	}
	// Write to a temporary file of its own and rename it, so pages never load
	// a script that is only partly written, even while two admins publish.
	tmp, err := os.CreateTemp(dir, name+"-*.tmp")
	if err != nil {
		logger.Error("Failed to create temporary script", "part", name, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "create temporary file")
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	_, err = tmp.WriteString(req.Source)
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
		logger.Error("Failed to write customized part", "part", name, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "write script")
	}
	return http.StatusOK, &adminCustomPart{Name: name, Source: req.Source}, nil
}
