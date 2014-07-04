// +build pprof

package debug

import (
	"net/http/pprof"

	"github.com/go-martini/martini"
)

func RegisterRoutes(r martini.Router) {
	r.Get("/debug/pprof/cmdline", pprof.Cmdline)
	r.Get("/debug/pprof/profile", pprof.Profile)
	r.Get("/debug/pprof/symbol", pprof.Symbol)
	r.Get("/debug/pprof/**", pprof.Index)
}
