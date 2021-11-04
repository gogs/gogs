// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package app

import (
	"net/http"

	"gopkg.in/macaron.v1"

	"gogs.io/gogs/internal/authutil"
	"gogs.io/gogs/internal/conf"
)

func MetricsFilter() macaron.Handler {
	return func(w http.ResponseWriter, r *http.Request) {
		if !conf.Prometheus.Enabled {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		if !conf.Prometheus.EnableBasicAuth {
			return
		}

		username, password := authutil.DecodeBasic(r.Header)
		if username != conf.Prometheus.BasicAuthUsername || password != conf.Prometheus.BasicAuthPassword {
			w.WriteHeader(http.StatusForbidden)
			return
		}
	}
}
