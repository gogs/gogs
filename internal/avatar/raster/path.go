package raster

import (
	"math"
	"strconv"

	"github.com/cockroachdb/errors"
)

// pathBuilder accumulates flattened subpaths in device space.
type pathBuilder struct {
	m    Matrix
	tol  float64
	path Path
	cur  []Point
}

func (b *pathBuilder) moveTo(p Point) {
	b.flush(false)
	b.cur = append(b.cur[:0:0], p)
}

func (b *pathBuilder) lineTo(p Point) {
	b.cur = append(b.cur, p)
}

// flush ends the current subpath. Subpaths with a single point draw nothing,
// so they are dropped.
func (b *pathBuilder) flush(closed bool) {
	if len(b.cur) >= 2 {
		b.path.Subpaths = append(b.path.Subpaths, b.cur)
		b.path.Closed = append(b.path.Closed, closed)
	}
	b.cur = nil
}

// matrixNorm returns an upper bound of how much m stretches lengths.
func matrixNorm(m Matrix) float64 {
	return math.Sqrt(m[0]*m[0] + m[1]*m[1] + m[2]*m[2] + m[3]*m[3])
}

func (b *pathBuilder) quadTo(p0, p1, p2 Point) {
	ddx, ddy := p0.X-2*p1.X+p2.X, p0.Y-2*p1.Y+p2.Y
	// Uniform subdivision into n pieces deviates at most |p''|/(8n²), where
	// |p''| = 2*|p0-2p1+p2|.
	n := int(math.Ceil(math.Sqrt(math.Hypot(ddx, ddy) / (4 * b.tol))))
	n = max(1, min(n, 1000))
	for i := 1; i < n; i++ {
		t := float64(i) / float64(n)
		mt := 1 - t
		b.lineTo(Point{
			mt*mt*p0.X + 2*mt*t*p1.X + t*t*p2.X,
			mt*mt*p0.Y + 2*mt*t*p1.Y + t*t*p2.Y,
		})
	}
	b.lineTo(p2)
}

func (b *pathBuilder) cubicTo(p0, p1, p2, p3 Point) {
	d1 := math.Hypot(p0.X-2*p1.X+p2.X, p0.Y-2*p1.Y+p2.Y)
	d2 := math.Hypot(p1.X-2*p2.X+p3.X, p1.Y-2*p2.Y+p3.Y)
	// The second derivative is bounded by 6*max(d1, d2), and uniform
	// subdivision into n pieces deviates at most that over 8n².
	n := int(math.Ceil(math.Sqrt(0.75 * max(d1, d2) / b.tol)))
	n = max(1, min(n, 1000))
	for i := 1; i < n; i++ {
		t := float64(i) / float64(n)
		mt := 1 - t
		a, bb, c, d := mt*mt*mt, 3*mt*mt*t, 3*mt*t*t, t*t*t
		b.lineTo(Point{
			a*p0.X + bb*p1.X + c*p2.X + d*p3.X,
			a*p0.Y + bb*p1.Y + c*p2.Y + d*p3.Y,
		})
	}
	b.lineTo(p3)
}

// arcSegments returns how many lines approximate an arc of sweep radians on a
// circle of radius r device pixels within tol.
func arcSegments(r, sweep, tol float64) int {
	step := math.Pi / 2
	if r > tol {
		step = min(step, 2*math.Acos(1-tol/r))
	}
	return max(1, min(int(math.Ceil(math.Abs(sweep)/step)), 10000))
}

// ellipseArc appends points of the ellipse centered at (cx, cy) in user space
// with radii (rx, ry) rotated by phi, from angle theta over sweep, excluding the
// starting point.
func (b *pathBuilder) ellipseArc(cx, cy, rx, ry, phi, theta, sweep float64) {
	sinPhi, cosPhi := math.Sincos(phi)
	n := arcSegments(max(rx, ry)*matrixNorm(b.m), sweep, b.tol)
	for i := 1; i <= n; i++ {
		s, c := math.Sincos(theta + sweep*float64(i)/float64(n))
		b.lineTo(b.m.Apply(
			cx+rx*cosPhi*c-ry*sinPhi*s,
			cy+rx*sinPhi*c+ry*cosPhi*s,
		))
	}
}

// arcTo appends the SVG elliptical arc from (x1, y1) to (x2, y2) in user space,
// using the endpoint to center conversion of SVG 1.1 appendix F.6.5.
func (b *pathBuilder) arcTo(x1, y1, rx, ry, angle float64, large, sweep bool, x2, y2 float64) {
	end := b.m.Apply(x2, y2)
	if x1 == x2 && y1 == y2 {
		return
	}
	rx, ry = math.Abs(rx), math.Abs(ry)
	if rx == 0 || ry == 0 {
		b.lineTo(end)
		return
	}
	phi := angle * math.Pi / 180
	sinPhi, cosPhi := math.Sincos(phi)
	hx, hy := (x1-x2)/2, (y1-y2)/2
	x1p := cosPhi*hx + sinPhi*hy
	y1p := -sinPhi*hx + cosPhi*hy

	if lambda := x1p*x1p/(rx*rx) + y1p*y1p/(ry*ry); lambda > 1 {
		s := math.Sqrt(lambda)
		rx *= s
		ry *= s
	}
	num := rx*rx*ry*ry - rx*rx*y1p*y1p - ry*ry*x1p*x1p
	den := rx*rx*y1p*y1p + ry*ry*x1p*x1p
	coef := math.Sqrt(max(0, num/den))
	if large == sweep {
		coef = -coef
	}
	cxp := coef * rx * y1p / ry
	cyp := -coef * ry * x1p / rx
	cx := cosPhi*cxp - sinPhi*cyp + (x1+x2)/2
	cy := sinPhi*cxp + cosPhi*cyp + (y1+y2)/2

	theta1 := math.Atan2((y1p-cyp)/ry, (x1p-cxp)/rx)
	theta2 := math.Atan2((-y1p-cyp)/ry, (-x1p-cxp)/rx)
	delta := theta2 - theta1
	if sweep && delta < 0 {
		delta += 2 * math.Pi
	} else if !sweep && delta > 0 {
		delta -= 2 * math.Pi
	}
	b.ellipseArc(cx, cy, rx, ry, phi, theta1, delta)
	// Land exactly on the endpoint so that rounding does not leave a sliver.
	b.cur[len(b.cur)-1] = end
}

// pathScanner tokenizes SVG path data.
type pathScanner struct {
	d   string
	pos int
}

func (s *pathScanner) skipSeparators() {
	for s.pos < len(s.d) {
		switch s.d[s.pos] {
		case ' ', '\t', '\n', '\r', '\f', ',':
			s.pos++
		default:
			return
		}
	}
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func (s *pathScanner) number() (float64, error) {
	s.skipSeparators()
	start := s.pos
	i := s.pos
	if i < len(s.d) && (s.d[i] == '+' || s.d[i] == '-') {
		i++
	}
	digits := 0
	for i < len(s.d) && isDigit(s.d[i]) {
		i++
		digits++
	}
	if i < len(s.d) && s.d[i] == '.' {
		i++
		for i < len(s.d) && isDigit(s.d[i]) {
			i++
			digits++
		}
	}
	if digits == 0 {
		return 0, errors.Newf("expected a number at offset %d", start)
	}
	if i < len(s.d) && (s.d[i] == 'e' || s.d[i] == 'E') {
		j := i + 1
		if j < len(s.d) && (s.d[j] == '+' || s.d[j] == '-') {
			j++
		}
		if j < len(s.d) && isDigit(s.d[j]) {
			for j < len(s.d) && isDigit(s.d[j]) {
				j++
			}
			i = j
		}
	}
	v, err := strconv.ParseFloat(s.d[start:i], 64)
	if err != nil {
		return 0, errors.Wrapf(err, "parse number at offset %d", start)
	}
	s.pos = i
	return v, nil
}

// flag reads an arc flag, which is a single digit that needs no separator from
// what follows.
func (s *pathScanner) flag() (bool, error) {
	s.skipSeparators()
	if s.pos < len(s.d) {
		switch s.d[s.pos] {
		case '0':
			s.pos++
			return false, nil
		case '1':
			s.pos++
			return true, nil
		}
	}
	return false, errors.Newf("expected an arc flag at offset %d", s.pos)
}

func (s *pathScanner) numbers(dst []float64) error {
	for i := range dst {
		v, err := s.number()
		if err != nil {
			return err
		}
		dst[i] = v
	}
	return nil
}

func parsePath(d string, m Matrix, tolerance float64) (*Path, error) {
	if !(tolerance > 0) {
		return nil, errors.Newf("tolerance must be positive, got %v", tolerance)
	}
	b := &pathBuilder{m: m, tol: tolerance}
	s := &pathScanner{d: d}
	// Current point, subpath start and the last control point, in user space.
	var cx, cy, sx, sy, ctrlX, ctrlY float64
	var cmd, prev byte
	var args [7]float64

	ensureSubpath := func() {
		if b.cur == nil {
			b.cur = append(b.cur, m.Apply(cx, cy))
		}
	}

	for {
		s.skipSeparators()
		if s.pos >= len(s.d) {
			break
		}
		c := s.d[s.pos]
		switch {
		case (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z'):
			cmd = c
			s.pos++
		case cmd == 0:
			return nil, errors.Newf("path data must start with a command, got %q", c)
		case cmd == 'Z' || cmd == 'z':
			return nil, errors.Newf("unexpected number after close path at offset %d", s.pos)
		}
		if prev == 0 && cmd != 'M' && cmd != 'm' {
			return nil, errors.Newf("path data must start with a moveto, got %q", cmd)
		}

		rel := cmd >= 'a'
		var ox, oy float64
		if rel {
			ox, oy = cx, cy
		}
		switch cmd {
		case 'M', 'm':
			if err := s.numbers(args[:2]); err != nil {
				return nil, err
			}
			cx, cy = ox+args[0], oy+args[1]
			sx, sy = cx, cy
			b.moveTo(m.Apply(cx, cy))
			// Further coordinate pairs are implicit linetos.
			if rel {
				cmd = 'l'
			} else {
				cmd = 'L'
			}
			prev = 'M'
			continue
		case 'Z', 'z':
			b.flush(true)
			cx, cy = sx, sy
		case 'L', 'l':
			if err := s.numbers(args[:2]); err != nil {
				return nil, err
			}
			ensureSubpath()
			cx, cy = ox+args[0], oy+args[1]
			b.lineTo(m.Apply(cx, cy))
		case 'H', 'h':
			if err := s.numbers(args[:1]); err != nil {
				return nil, err
			}
			ensureSubpath()
			cx = ox + args[0]
			b.lineTo(m.Apply(cx, cy))
		case 'V', 'v':
			if err := s.numbers(args[:1]); err != nil {
				return nil, err
			}
			ensureSubpath()
			cy = oy + args[0]
			b.lineTo(m.Apply(cx, cy))
		case 'C', 'c', 'S', 's':
			x1, y1 := cx, cy
			var x2, y2, x, y float64
			if cmd == 'C' || cmd == 'c' {
				if err := s.numbers(args[:6]); err != nil {
					return nil, err
				}
				x1, y1 = ox+args[0], oy+args[1]
				x2, y2, x, y = ox+args[2], oy+args[3], ox+args[4], oy+args[5]
			} else {
				if err := s.numbers(args[:4]); err != nil {
					return nil, err
				}
				if prev == 'C' {
					x1, y1 = 2*cx-ctrlX, 2*cy-ctrlY
				}
				x2, y2, x, y = ox+args[0], oy+args[1], ox+args[2], oy+args[3]
			}
			ensureSubpath()
			b.cubicTo(m.Apply(cx, cy), m.Apply(x1, y1), m.Apply(x2, y2), m.Apply(x, y))
			ctrlX, ctrlY = x2, y2
			cx, cy = x, y
			prev = 'C'
			continue
		case 'Q', 'q', 'T', 't':
			x1, y1 := cx, cy
			var x, y float64
			if cmd == 'Q' || cmd == 'q' {
				if err := s.numbers(args[:4]); err != nil {
					return nil, err
				}
				x1, y1 = ox+args[0], oy+args[1]
				x, y = ox+args[2], oy+args[3]
			} else {
				if err := s.numbers(args[:2]); err != nil {
					return nil, err
				}
				if prev == 'Q' {
					x1, y1 = 2*cx-ctrlX, 2*cy-ctrlY
				}
				x, y = ox+args[0], oy+args[1]
			}
			ensureSubpath()
			b.quadTo(m.Apply(cx, cy), m.Apply(x1, y1), m.Apply(x, y))
			ctrlX, ctrlY = x1, y1
			cx, cy = x, y
			prev = 'Q'
			continue
		case 'A', 'a':
			if err := s.numbers(args[:3]); err != nil {
				return nil, err
			}
			large, err := s.flag()
			if err != nil {
				return nil, err
			}
			sweep, err := s.flag()
			if err != nil {
				return nil, err
			}
			if err := s.numbers(args[3:5]); err != nil {
				return nil, err
			}
			ensureSubpath()
			x, y := ox+args[3], oy+args[4]
			b.arcTo(cx, cy, args[0], args[1], args[2], large, sweep, x, y)
			cx, cy = x, y
		default:
			return nil, errors.Newf("unknown path command %q", cmd)
		}
		prev = 'L'
	}
	b.flush(false)
	return &b.path, nil
}
