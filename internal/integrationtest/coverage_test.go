package integrationtest

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
// A Flamego route is covered by a span with its method and pattern. A Macaron
// route is covered by a span whose path it matches, and when several routes
// match, the span goes to the one Macaron prefers, see macaronPreferred. Routes
// that render pages need no test, but still take the spans of their pages.
func checkRouteCoverage(spanNames map[string]bool) error {
	routes, err := listRoutes()
	if err != nil {
		return errors.Wrap(err, "list routes")
	}

	type candidate struct {
		route *route
		path  *regexp.Regexp
	}
	covered := make(map[*route]bool)
	var macaron []candidate
	for i := range routes {
		r := &routes[i]
		switch {
		case r.Bridge:
		case r.Macaron:
			re, err := macaronPathRegexp(r.Pattern)
			if err != nil {
				return errors.Wrapf(err, "route %s %s", r.Method, r.Pattern)
			}
			macaron = append(macaron, candidate{route: r, path: re})
		case r.PathRegexp != "":
			re := regexp.MustCompile(r.PathRegexp)
			for name := range spanNames {
				method, path, ok := strings.Cut(name, " ")
				if ok && (r.Method == "*" || r.Method == method) && re.MatchString(path) {
					covered[r] = true
				}
			}
		default:
			covered[r] = spanNames[r.Method+" "+r.Pattern]
		}
	}

	for name := range spanNames {
		method, path, ok := strings.Cut(name, " ")
		// Spans of Flamego routes are named after patterns, e.g., "{owner}".
		if !ok || strings.Contains(path, "{") {
			continue
		}
		var best *candidate
		for i, c := range macaron {
			if (c.route.Method == "*" || c.route.Method == method) && c.path.MatchString(path) {
				if best == nil || macaronPreferred(c.route.Pattern, best.route.Pattern) {
					best = &macaron[i]
				}
			}
		}
		if best != nil {
			covered[best.route] = true
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

// macaronPreferred returns true if Macaron matches a path with pattern a
// rather than pattern b when both match it. Macaron tries the segments of
// patterns from the left, a static segment before a parameter with a regular
// expression, before a "*.*", before a plain parameter, before a "*".
func macaronPreferred(a, b string) bool {
	as, bs := strings.Split(a, "/"), strings.Split(b, "/")
	for i := 0; i < len(as) && i < len(bs); i++ {
		if ra, rb := macaronSegmentRank(as[i]), macaronSegmentRank(bs[i]); ra != rb {
			return ra < rb
		}
	}
	return literalLength(a) > literalLength(b)
}

var macaronHolder = regexp.MustCompile(`^:[A-Za-z0-9_]+$`)

func macaronSegmentRank(segment string) int {
	segment = strings.TrimLeft(segment, "?")
	switch {
	case segment == "*":
		return 4
	case macaronHolder.MatchString(segment):
		return 3
	case segment == "*.*":
		return 2
	case strings.Contains(segment, ":"):
		return 1
	}
	return 0
}

// literalLength returns the number of characters of a Macaron route pattern
// outside of parameters and wildcards.
func literalLength(pattern string) int {
	n := 0
	for _, segment := range strings.Split(pattern, "/") {
		if !strings.ContainsAny(segment, ":*") {
			n += len(segment) + 1
		}
	}
	return n
}
