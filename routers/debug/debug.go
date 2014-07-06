// +build pprof

package debug

import (
	"net/http/pprof"

	"github.com/go-martini/martini"
)

func RegisterRoutes(r martini.Router) {
	r.Any("/debug/pprof/cmdline", pprof.Cmdline)
	r.Any("/debug/pprof/profile", pprof.Profile)
	r.Any("/debug/pprof/symbol", pprof.Symbol)
	r.Any("/debug/pprof/**", pprof.Index)
}
