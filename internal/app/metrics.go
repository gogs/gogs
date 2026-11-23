package app

import (
	"net/http"

	"gogs.io/gogs/internal/authx"
	"gogs.io/gogs/internal/conf"
)

// MetricsFilter returns a handler that turns away requests for metrics when
// they are disabled, or when the basic authentication they require does not
// match.
func MetricsFilter(opts conf.PrometheusOpts) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		if !opts.Enabled {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		if !opts.EnableBasicAuth {
			return
		}

		username, password := authx.DecodeBasic(r.Header)
		if username != opts.BasicAuthUsername || password != opts.BasicAuthPassword {
			w.WriteHeader(http.StatusForbidden)
			return
		}
	}
}
