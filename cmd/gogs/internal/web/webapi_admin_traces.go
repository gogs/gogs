package web

import (
	"net/http"
	"strconv"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/flamego/flamego"

	"gogs.io/gogs/internal/telemetry"
)

func getAdminTraceOverview(c flamego.Context, store *telemetry.Store) (statusCode int, resp *telemetry.Overview, err error) {
	rangeName := c.Query("range", "1h")
	var minutes int
	switch rangeName {
	case "15m":
		minutes = 15
	case "1h":
		minutes = 60
	default:
		return http.StatusBadRequest, nil, errors.New("range must be 15m or 1h")
	}
	overview := store.Overview(minutes, rangeName)
	return http.StatusOK, &overview, nil
}

func getAdminTraces(c flamego.Context, store *telemetry.Store) (statusCode int, resp *telemetry.SearchResult, err error) {
	q := telemetry.SearchQuery{
		Q:       c.Query("q"),
		Route:   c.Query("route"),
		Status:  c.Query("status"),
		Slowest: c.Query("sort") == "slowest",
	}
	if q.Status != "" && q.Status != "5xx" && q.Status != "4xx" {
		return http.StatusBadRequest, nil, errors.New("status must be 5xx or 4xx")
	}
	if v := c.Query("minMs"); v != "" {
		q.MinMs, err = strconv.ParseFloat(v, 64)
		if err != nil {
			return http.StatusBadRequest, nil, errors.New("minMs must be a number")
		}
	}
	for _, p := range []struct {
		name string
		to   *time.Time
	}{{"from", &q.From}, {"to", &q.To}} {
		v := c.Query(p.name)
		if v == "" {
			continue
		}
		unixMilli, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return http.StatusBadRequest, nil, errors.Newf("%s must be Unix milliseconds", p.name)
		}
		*p.to = time.UnixMilli(unixMilli)
	}
	result := store.Search(q)
	return http.StatusOK, &result, nil
}

func getAdminTrace(c flamego.Context, store *telemetry.Store) (statusCode int, resp *telemetry.Trace, err error) {
	t, ok := store.Get(c.Param("id"))
	if !ok {
		return http.StatusNotFound, nil, errors.New("trace not found")
	}
	return http.StatusOK, t, nil
}

func getAdminHealth(c flamego.Context, health *telemetry.Health) (statusCode int, resp *telemetry.HealthReport, err error) {
	rangeName := c.Query("range", "1h")
	if rangeName != "1h" && rangeName != "24h" {
		return http.StatusBadRequest, nil, errors.New("range must be 1h or 24h")
	}
	report := health.Report(time.Now(), rangeName, rangeName == "24h")
	return http.StatusOK, &report, nil
}
