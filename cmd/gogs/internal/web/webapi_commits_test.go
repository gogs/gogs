package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/flamego/flamego"
	"github.com/stretchr/testify/assert"
)

func TestCompareRangeRoute(t *testing.T) {
	f := flamego.New()
	f.Get("/compare/"+compareRangeRoute, func(c flamego.Context) string {
		return c.Param("before") + " " + c.Param("after")
	})

	sha1Before, sha1After := strings.Repeat("a", 40), strings.Repeat("b", 40)
	sha256Before, sha256After := strings.Repeat("c", 64), strings.Repeat("d", 64)
	tests := []struct {
		name     string
		path     string
		wantCode int
		wantBody string
	}{
		{name: "SHA-1", path: sha1Before + "..." + sha1After, wantCode: http.StatusOK, wantBody: sha1Before + " " + sha1After},
		{name: "SHA-256", path: sha256Before + "..." + sha256After, wantCode: http.StatusOK, wantBody: sha256Before + " " + sha256After},
		{name: "abbreviated", path: sha1Before[:39] + "..." + sha1After, wantCode: http.StatusNotFound},
		{name: "between the two lengths", path: strings.Repeat("e", 50) + "..." + sha1After, wantCode: http.StatusNotFound},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resp := httptest.NewRecorder()
			f.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/compare/"+test.path, nil))
			assert.Equal(t, test.wantCode, resp.Code)
			if test.wantBody != "" {
				assert.Equal(t, test.wantBody, resp.Body.String())
			}
		})
	}
}
