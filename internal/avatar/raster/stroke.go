package raster

import (
	"math"
	"slices"
)

// strokeJoinTolerance is how far in pixels the polygonal round joins may fall
// short of a true circle.
const strokeJoinTolerance = 0.1

// Stroke implementation: every segment becomes a rectangle and every vertex
// gets a pie slice filling the gap on the outer side of the turn. Overlapping
// pieces are fine under NonZero as long as they all wind the same way, so each
// polygon is normalized to a positive signed area.
func stroke(p *Path, width float64) *Path {
	out := &Path{}
	if p == nil || !(width > 0) {
		return out
	}
	hw := width / 2
	emit := func(poly []Point) {
		if signedArea(poly) < 0 {
			slices.Reverse(poly)
		}
		out.Subpaths = append(out.Subpaths, poly)
		out.Closed = append(out.Closed, true)
	}

	var pts []Point
	for si, sp := range p.Subpaths {
		closed := si < len(p.Closed) && p.Closed[si]
		pts = pts[:0]
		for _, pt := range sp {
			if len(pts) == 0 || pt != pts[len(pts)-1] {
				pts = append(pts, pt)
			}
		}
		if closed && len(pts) > 1 && pts[0] == pts[len(pts)-1] {
			pts = pts[:len(pts)-1]
		}
		if len(pts) < 2 {
			continue
		}
		segments := len(pts) - 1
		if closed {
			segments = len(pts)
		}
		for i := 0; i < segments; i++ {
			a, b := pts[i], pts[(i+1)%len(pts)]
			n := normal(a, b, hw)
			emit([]Point{
				{a.X + n.X, a.Y + n.Y},
				{b.X + n.X, b.Y + n.Y},
				{b.X - n.X, b.Y - n.Y},
				{a.X - n.X, a.Y - n.Y},
			})
		}
		for i := range pts {
			if !closed && (i == 0 || i == len(pts)-1) {
				continue
			}
			prev := pts[(i+len(pts)-1)%len(pts)]
			next := pts[(i+1)%len(pts)]
			if poly := joinPie(prev, pts[i], next, hw); poly != nil {
				emit(poly)
			}
		}
	}
	return out
}

// normal returns the left normal of the segment from a to b with length hw.
func normal(a, b Point, hw float64) Point {
	dx, dy := b.X-a.X, b.Y-a.Y
	s := hw / math.Hypot(dx, dy)
	return Point{-dy * s, dx * s}
}

// joinPie returns the round join at v between the segments prev to v and v to
// next, or nil when they are collinear and leave no gap.
func joinPie(prev, v, next Point, hw float64) []Point {
	d0x, d0y := v.X-prev.X, v.Y-prev.Y
	d1x, d1y := next.X-v.X, next.Y-v.Y
	cross := d0x*d1y - d0y*d1x
	dot := d0x*d1x + d0y*d1y
	turn := math.Atan2(cross, dot)
	if math.Abs(turn) < 1e-9 {
		return nil
	}
	n0 := normal(prev, v, hw)
	if cross > 0 {
		// Turning toward the left normal leaves the gap on the right side.
		n0 = Point{-n0.X, -n0.Y}
	}
	steps := arcSegments(hw, turn, strokeJoinTolerance)
	poly := make([]Point, 0, steps+2)
	poly = append(poly, v)
	for k := 0; k <= steps; k++ {
		s, c := math.Sincos(turn * float64(k) / float64(steps))
		poly = append(poly, Point{v.X + n0.X*c - n0.Y*s, v.Y + n0.X*s + n0.Y*c})
	}
	return poly
}

func signedArea(poly []Point) float64 {
	var a float64
	for i := range poly {
		p, q := poly[i], poly[(i+1)%len(poly)]
		a += p.X*q.Y - q.X*p.Y
	}
	return a / 2
}
