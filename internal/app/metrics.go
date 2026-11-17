package app

import (
	"net/http"

	"gogs.io/gogs/internal/authx"
	"gogs.io/gogs/internal/conf"
)

// MetricsFilter turns away requests for metrics when they are disabled, or
// when the basic authentication they require does not match.
func MetricsFilter(w http.ResponseWriter, r *http.Request) {
	if !conf.Prometheus.Enabled {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	if !conf.Prometheus.EnableBasicAuth {
		return
	}

	username, password := authx.DecodeBasic(r.Header)
	if username != conf.Prometheus.BasicAuthUsername || password != conf.Prometheus.BasicAuthPassword {
		w.WriteHeader(http.StatusForbidden)
		return
	}
}
