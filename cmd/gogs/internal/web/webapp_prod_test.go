//go:build prod

package web

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"path"
	"strings"
	"testing"

	"github.com/flamego/flamego"
	"github.com/stretchr/testify/require"

	"gogs.io/gogs/public"
)

func TestMountWebAppRoutes_ServesAssetWithoutSubpathPrefix(t *testing.T) {
	entries, err := fs.ReadDir(public.WebAssets, "dist/assets")
	require.NoError(t, err)

	var assetName string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".js") {
			assetName = entry.Name()
			break
		}
	}
	require.NotEmpty(t, assetName)

	f := flamego.New()
	require.NoError(t, mountWebAppRoutes(f))

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, path.Join("/assets", assetName), nil)
	f.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Header().Get("Content-Type"), "javascript")
}

func TestProductionBundle_UsesRelativeAssetReferences(t *testing.T) {
	entries, err := fs.ReadDir(public.WebAssets, "dist/assets")
	require.NoError(t, err)

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".css") && !strings.HasSuffix(entry.Name(), ".js") {
			continue
		}

		contents, err := public.WebAssets.ReadFile(path.Join("dist/assets", entry.Name()))
		require.NoError(t, err)
		require.NotContains(t, string(contents), "url(/assets/", entry.Name())
		require.NotContains(t, string(contents), "/assets/worker-", entry.Name())
	}
}
