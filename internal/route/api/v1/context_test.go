package v1

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/flamego/flamego"
	"github.com/stretchr/testify/assert"

	"gogs.io/gogs/internal/conf"
)

func TestAPIContext_SetLinkHeader(t *testing.T) {
	conf.SetMockServer(t, conf.ServerOpts{ExternalURL: "https://gogs.example.com/"})

	f := flamego.New()
	f.Get("/api/v1/repos/alice/r/commits", func(c flamego.Context) {
		ctx := &apiContext{Context: c, path: "/api/v1/repos/alice/r/commits"}
		ctx.SetLinkHeader(75, 20)
	})
	w := httptest.NewRecorder()
	f.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/repos/alice/r/commits?sha=main&pageSize=20&page=2", nil))

	const base = "https://gogs.example.com/api/v1/repos/alice/r/commits?"
	assert.Equal(t,
		`<`+base+`page=3&pageSize=20&sha=main>; rel="next",`+
			`<`+base+`page=4&pageSize=20&sha=main>; rel="last",`+
			`<`+base+`page=1&pageSize=20&sha=main>; rel="first",`+
			`<`+base+`page=1&pageSize=20&sha=main>; rel="prev"`,
		w.Header().Get("Link"),
	)
}
