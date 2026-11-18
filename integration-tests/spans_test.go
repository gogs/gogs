package integrationtests

import (
	"compress/gzip"
	"io"
	"net"
	"net/http"
	"sync"

	"github.com/cockroachdb/errors"
	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/protobuf/proto"
)

// spanReceiver receives the spans the server under test exports over OTLP. The
// server names the span of a request after the route it matched, e.g.,
// "GET /api/web/{owner}/{repo}/header".
type spanReceiver struct {
	server *http.Server
	url    string

	mu    sync.Mutex
	names map[string]bool
}

func newSpanReceiver() (*spanReceiver, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, errors.Wrap(err, "listen")
	}
	r := &spanReceiver{
		url:   "http://" + l.Addr().String() + "/v1/traces",
		names: make(map[string]bool),
	}
	r.server = &http.Server{Handler: http.HandlerFunc(r.serveHTTP)}
	go func() { _ = r.server.Serve(l) }()
	return r, nil
}

func (r *spanReceiver) serveHTTP(w http.ResponseWriter, req *http.Request) {
	var body io.Reader = req.Body
	if req.Header.Get("Content-Encoding") == "gzip" {
		gz, err := gzip.NewReader(req.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		body = gz
	}
	b, err := io.ReadAll(body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var export coltracepb.ExportTraceServiceRequest
	if err = proto.Unmarshal(b, &export); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	r.mu.Lock()
	for _, rs := range export.ResourceSpans {
		for _, ss := range rs.ScopeSpans {
			for _, s := range ss.Spans {
				// Spans of requests are the ones without a parent in the
				// server, unlike the spans of Git operations.
				if len(s.ParentSpanId) == 0 {
					r.names[s.Name] = true
				}
			}
		}
	}
	r.mu.Unlock()

	w.Header().Set("Content-Type", "application/x-protobuf")
	resp, _ := proto.Marshal(&coltracepb.ExportTraceServiceResponse{})
	_, _ = w.Write(resp)
}

// spanNames returns the names of the request spans received so far.
func (r *spanReceiver) spanNames() map[string]bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	names := make(map[string]bool, len(r.names))
	for name := range r.names {
		names[name] = true
	}
	return names
}

func (r *spanReceiver) close() {
	_ = r.server.Close()
}
