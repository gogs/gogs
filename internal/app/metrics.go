// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package app

import (
	"net/http"

	"gopkg.in/macaron.v1"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/context"
)

func MetricsFilter() macaron.Handler {
	return func(c *context.Context) {
		if !conf.Prometheus.Enabled {
			c.Status(http.StatusNotFound)
			return
		}

		if !conf.Prometheus.EnableBasicAuth {
			return
		}

		c.RequireBasicAuth(conf.Prometheus.BasicAuthUsername, conf.Prometheus.BasicAuthPassword)
	}
}
