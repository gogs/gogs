package integrationtests

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/cockroachdb/errors"
)

// checkRouteCoverage returns an error that lists the routes no test sent a
// request to, judged by the names of the request spans of the server under
// test.
//
// A route is covered by a span with its method and pattern. Routes that render
// pages need no test.
func checkRouteCoverage(spanNames map[string]bool) error {
	routes, err := listRoutes()
	if err != nil {
		return errors.Wrap(err, "list routes")
	}

	covered := make(map[*route]bool)
	for i := range routes {
		r := &routes[i]
		switch {
		case r.Bridge:
		case r.PathRegexp != "":
			re := regexp.MustCompile(r.PathRegexp)
			for name := range spanNames {
				method, path, ok := strings.Cut(name, " ")
				if ok && (r.Method == "*" || r.Method == method) && re.MatchString(path) {
					covered[r] = true
				}
			}
		case r.Method == "*":
			for name := range spanNames {
				if _, pattern, ok := strings.Cut(name, " "); ok && pattern == r.Pattern {
					covered[r] = true
				}
			}
		default:
			covered[r] = spanNames[r.Method+" "+r.Pattern]
		}
	}

	var missing []string
	total := 0
	for i := range routes {
		r := &routes[i]
		if r.Bridge || r.Page {
			continue
		}
		total++
		if !covered[r] {
			pattern := r.Pattern
			if r.PathRegexp != "" {
				pattern = r.PathRegexp
			}
			missing = append(missing, fmt.Sprintf("%s %s (%s)", r.Method, pattern, r.Pos))
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return errors.Newf("%d of %d routes have no test that sends a request to them:\n%s", len(missing), total, strings.Join(missing, "\n"))
	}
	return nil
}
