package raster

import (
	"math"
	"slices"
)

// subScanlines is how many rows of samples are taken per pixel row. Within a
// sample row the horizontal coverage is computed exactly.
const subScanlines = 16

// edge is a non-horizontal segment, oriented top to bottom.
type edge struct {
	x0, y0  float64 // The top end.
	y1      float64 // The bottom end.
	dxdy    float64
	x       float64 // The crossing with the current sample row.
	winding int
}

func buildEdges(p *Path, h int) []edge {
	n := 0
	for _, sp := range p.Subpaths {
		n += len(sp)
	}
	edges := make([]edge, 0, n)
	fh := float64(h)
	for _, sp := range p.Subpaths {
		for i := range sp {
			a, b := sp[i], sp[(i+1)%len(sp)]
			if a.Y == b.Y || math.IsNaN(a.X+a.Y+b.X+b.Y) {
				continue
			}
			wind := 1
			if a.Y > b.Y {
				a, b = b, a
				wind = -1
			}
			if b.Y <= 0 || a.Y >= fh {
				continue
			}
			edges = append(edges, edge{
				x0: a.X, y0: a.Y, y1: b.Y,
				dxdy:    (b.X - a.X) / (b.Y - a.Y),
				winding: wind,
			})
		}
	}
	return edges
}

// Fill implementation: for every sample row, the active edges are sorted by
// their crossing, the spans inside under the rule are found, and each span adds
// its exact horizontal coverage to the pixel row. Partial pixels at span ends
// go to cover, and runs of whole pixels are recorded as differences in run so
// that a span costs O(1) regardless of its length.
func fill(p *Path, rule FillRule, w, h int) *Mask {
	if w <= 0 || h <= 0 {
		return &Mask{W: max(w, 0), H: max(h, 0)}
	}
	mask := &Mask{W: w, H: h, A: make([]float32, w*h)}
	if p == nil {
		return mask
	}
	edges := buildEdges(p, h)
	if len(edges) == 0 {
		return mask
	}
	slices.SortFunc(edges, func(a, b edge) int {
		switch {
		case a.y0 < b.y0:
			return -1
		case a.y0 > b.y0:
			return 1
		}
		return 0
	})

	ymin, ymax := edges[0].y0, edges[0].y1
	for i := range edges {
		ymax = max(ymax, edges[i].y1)
	}
	rowStart := max(0, int(math.Floor(ymin)))
	rowEnd := min(h, int(math.Ceil(ymax)))

	cover := make([]float32, w+2)
	run := make([]float32, w+2)
	active := make([]*edge, 0, 64)
	next := 0
	fw := float64(w)
	const weight = 1.0 / subScanlines

	for row := rowStart; row < rowEnd; row++ {
		minX, maxX := w, -1
		for sub := 0; sub < subScanlines; sub++ {
			y := float64(row) + (float64(sub)+0.5)/subScanlines
			for next < len(edges) && edges[next].y0 <= y {
				active = append(active, &edges[next])
				next++
			}
			kept := active[:0]
			for _, e := range active {
				if e.y1 > y {
					e.x = e.x0 + (y-e.y0)*e.dxdy
					kept = append(kept, e)
				}
			}
			active = kept
			// The order barely changes between sample rows, so insertion
			// sort runs in close to linear time.
			for i := 1; i < len(active); i++ {
				e := active[i]
				j := i
				for j > 0 && active[j-1].x > e.x {
					active[j] = active[j-1]
					j--
				}
				active[j] = e
			}

			winding := 0
			var spanStart float64
			for _, e := range active {
				wasInside := inside(winding, rule)
				winding += e.winding
				isInside := inside(winding, rule)
				if wasInside == isInside {
					continue
				}
				if isInside {
					spanStart = e.x
					continue
				}
				a := min(max(spanStart, 0), fw)
				b := min(max(e.x, 0), fw)
				if b <= a {
					continue
				}
				ia, ib := int(a), int(b)
				minX = min(minX, ia)
				maxX = max(maxX, ib)
				if ia == ib {
					cover[ia] += float32((b - a) * weight)
					continue
				}
				cover[ia] += float32((float64(ia+1) - a) * weight)
				run[ia+1] += weight
				run[ib] -= weight
				cover[ib] += float32((b - float64(ib)) * weight)
			}
		}
		if maxX < 0 {
			continue
		}
		out := mask.A[row*w : row*w+w]
		var acc float32
		for x := minX; x <= maxX+1 && x <= w; x++ {
			acc += run[x]
			if x < w {
				out[x] = min(max(cover[x]+acc, 0), 1)
			}
			cover[x], run[x] = 0, 0
		}
	}
	return mask
}

func inside(winding int, rule FillRule) bool {
	if rule == EvenOdd {
		return winding&1 != 0
	}
	return winding != 0
}
