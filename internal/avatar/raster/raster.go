// Package raster turns vector paths into anti-aliased coverage masks. It
// implements only what the generated avatars need: SVG path data, rectangles,
// ellipses, the nonzero and evenodd fill rules, and strokes.
package raster

import "math"

// Point is a point in device pixels.
type Point struct {
	X, Y float64
}

// Matrix is the affine transform [a b c d e f] of SVG, mapping (x, y) to
// (a*x + c*y + e, b*x + d*y + f).
type Matrix [6]float64

// Identity is the transform that leaves points unchanged.
var Identity = Matrix{1, 0, 0, 1, 0, 0}

// Apply returns the point transformed by m.
func (m Matrix) Apply(x, y float64) Point {
	return Point{m[0]*x + m[2]*y + m[4], m[1]*x + m[3]*y + m[5]}
}

// Translate returns m applied after a translation by (x, y) in the user space
// of m, as SVG's transform="translate(x y)" on a child does.
func (m Matrix) Translate(x, y float64) Matrix {
	return Matrix{m[0], m[1], m[2], m[3], m[0]*x + m[2]*y + m[4], m[1]*x + m[3]*y + m[5]}
}

// Scale returns m applied after a scale by (sx, sy) in the user space of m.
func (m Matrix) Scale(sx, sy float64) Matrix {
	return Matrix{m[0] * sx, m[1] * sx, m[2] * sy, m[3] * sy, m[4], m[5]}
}

// Path is a shape flattened into polylines in device pixels.
type Path struct {
	Subpaths [][]Point
	// Closed reports for each subpath whether it ended with Z, which matters
	// only for strokes. Filling always closes subpaths.
	Closed []bool
}

// FillRule decides which points are inside a path.
type FillRule int

const (
	NonZero FillRule = iota
	EvenOdd
)

// Mask is the coverage of a shape on a W×H pixel grid, row by row, each value
// in [0, 1].
type Mask struct {
	W, H int
	A    []float32
}

// ParseSVGPath parses SVG path data, e.g., the "d" attribute of a <path>, and
// flattens it in device pixels after transforming by m. Curves and arcs are
// split into lines no farther than tolerance pixels from the curve.
func ParseSVGPath(d string, m Matrix, tolerance float64) (*Path, error) {
	return parsePath(d, m, tolerance)
}

// Rect returns the rectangle with the corner (x, y) and the given size,
// transformed by m.
func Rect(x, y, w, h float64, m Matrix) *Path {
	if !(w > 0 && h > 0) {
		return &Path{}
	}
	return &Path{
		Subpaths: [][]Point{{m.Apply(x, y), m.Apply(x+w, y), m.Apply(x+w, y+h), m.Apply(x, y+h)}},
		Closed:   []bool{true},
	}
}

// Ellipse returns the closed ellipse centered at (cx, cy), transformed by m and
// flattened within tolerance pixels.
func Ellipse(cx, cy, rx, ry float64, m Matrix, tolerance float64) *Path {
	if !(rx > 0 && ry > 0 && tolerance > 0) {
		return &Path{}
	}
	b := &pathBuilder{m: m, tol: tolerance}
	b.moveTo(m.Apply(cx+rx, cy))
	b.ellipseArc(cx, cy, rx, ry, 0, 0, 2*math.Pi)
	// The last point repeats the first, which a closed polygon does not need.
	b.cur = b.cur[:len(b.cur)-1]
	b.flush(true)
	return &b.path
}

// Stroke returns the outline of stroking p with lines width pixels wide, to be
// filled with NonZero.
func Stroke(p *Path, width float64) *Path {
	return stroke(p, width)
}

// Fill returns the anti-aliased coverage of p on a w×h grid under the rule.
func Fill(p *Path, rule FillRule, w, h int) *Mask {
	return fill(p, rule, w, h)
}
