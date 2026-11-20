package raster

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func sum(m *Mask) float64 {
	var s float64
	for _, a := range m.A {
		s += float64(a)
	}
	return s
}

func mustParse(t *testing.T, d string) *Path {
	t.Helper()
	p, err := ParseSVGPath(d, Identity, 0.1)
	require.NoError(t, err)
	return p
}

func TestParseSVGPath(t *testing.T) {
	t.Run("compact numbers", func(t *testing.T) {
		p := mustParse(t, "M0 0l10-5.5.5.5")
		require.Len(t, p.Subpaths, 1)
		assert.Equal(t, []Point{{0, 0}, {10, -5.5}, {10.5, -5}}, p.Subpaths[0])
		assert.Equal(t, []bool{false}, p.Closed)
	})

	t.Run("exponents and implicit lineto", func(t *testing.T) {
		p := mustParse(t, "M1e1,2E-1 3 4 m1 1 1 1")
		require.Len(t, p.Subpaths, 2)
		assert.Equal(t, []Point{{10, 0.2}, {3, 4}}, p.Subpaths[0])
		assert.Equal(t, []Point{{4, 5}, {5, 6}}, p.Subpaths[1])
	})

	t.Run("horizontal and vertical", func(t *testing.T) {
		p := mustParse(t, "M1 1H5v3h-2V2z")
		require.Len(t, p.Subpaths, 1)
		assert.Equal(t, []Point{{1, 1}, {5, 1}, {5, 4}, {3, 4}, {3, 2}}, p.Subpaths[0])
		assert.Equal(t, []bool{true}, p.Closed)
	})

	t.Run("close returns to subpath start", func(t *testing.T) {
		p := mustParse(t, "M10 10l5 0 0 5zl-5 0")
		require.Len(t, p.Subpaths, 2)
		assert.Equal(t, []bool{true, false}, p.Closed)
		assert.Equal(t, []Point{{10, 10}, {5, 10}}, p.Subpaths[1])
	})

	t.Run("compact arc flags", func(t *testing.T) {
		for _, d := range []string{"M0 0a5 5 0 015 5", "M0 0a5,5,0,0,1,5,5", "M0 0a5 5 0 0 1 5 5"} {
			p := mustParse(t, d)
			require.Len(t, p.Subpaths, 1, d)
			sp := p.Subpaths[0]
			assert.Equal(t, Point{5, 5}, sp[len(sp)-1], d)
			// A clockwise quarter circle centered at (0, 5) bulges toward +x.
			for _, pt := range sp {
				assert.InDelta(t, 5, math.Hypot(pt.X, pt.Y-5), 1e-9, d)
				assert.GreaterOrEqual(t, pt.X, -1e-9, d)
			}
			assert.Greater(t, len(sp), 3, d)
		}
	})

	t.Run("arc flags followed by a negative number", func(t *testing.T) {
		p := mustParse(t, "M0 0a40 40 0 0 1-46 12")
		sp := p.Subpaths[0]
		assert.Equal(t, Point{-46, 12}, sp[len(sp)-1])
	})

	t.Run("arc radii scaled up when too small", func(t *testing.T) {
		p := mustParse(t, "M0 0A1 1 0 0 1 10 0")
		sp := p.Subpaths[0]
		assert.Equal(t, Point{10, 0}, sp[len(sp)-1])
		for _, pt := range sp {
			assert.InDelta(t, 5, math.Hypot(pt.X-5, pt.Y), 1e-9)
		}
	})

	t.Run("smooth curves reflect the control point", func(t *testing.T) {
		smooth := mustParse(t, "M0 0C0 10 10 10 10 0S20-10 20 0")
		explicit := mustParse(t, "M0 0C0 10 10 10 10 0C10-10 20-10 20 0")
		assert.Equal(t, explicit.Subpaths, smooth.Subpaths)

		smooth = mustParse(t, "M0 0Q5 10 10 0T20 0")
		explicit = mustParse(t, "M0 0Q5 10 10 0Q15-10 20 0")
		assert.Equal(t, explicit.Subpaths, smooth.Subpaths)

		// Without a preceding curve, the first control point is the current point.
		smooth = mustParse(t, "M0 0L1 1S2 2 3 3")
		explicit = mustParse(t, "M0 0L1 1C1 1 2 2 3 3")
		assert.Equal(t, explicit.Subpaths, smooth.Subpaths)
	})

	t.Run("transform", func(t *testing.T) {
		p, err := ParseSVGPath("M1 2L3 4", Identity.Translate(10, 20).Scale(2, 3), 0.1)
		require.NoError(t, err)
		assert.Equal(t, []Point{{12, 26}, {16, 32}}, p.Subpaths[0])
	})

	t.Run("flattening tolerance", func(t *testing.T) {
		const tol = 0.1
		p, err := ParseSVGPath("M0 0C0 100 100 100 100 0", Identity, tol)
		require.NoError(t, err)
		sp := p.Subpaths[0]
		for i := 0; i+1 < len(sp); i++ {
			a, b := sp[i], sp[i+1]
			mid := Point{(a.X + b.X) / 2, (a.Y + b.Y) / 2}
			// Compare the chord midpoint with the curve at the matching parameter.
			tm := (float64(i) + 0.5) / float64(len(sp)-1)
			mt := 1 - tm
			cy := 3*mt*mt*tm*100 + 3*mt*tm*tm*100
			cx := 3*mt*tm*tm*100 + tm*tm*tm*100
			assert.LessOrEqual(t, math.Hypot(mid.X-cx, mid.Y-cy), tol)
		}
	})

	t.Run("errors", func(t *testing.T) {
		for _, d := range []string{"L1 1", "1 1", "M1", "M0 0a5 5 0 2 1 5 5", "M0 0X1", "M0 0z 1"} {
			_, err := ParseSVGPath(d, Identity, 0.1)
			assert.Error(t, err, d)
		}
	})
}

func TestFill_RectCoverage(t *testing.T) {
	for _, r := range [][4]float64{
		{2, 3, 5, 4},
		{2.3, 3.7, 5.2, 4.45},
		{0.1, 0.9, 0.6, 0.3},
		{-3.25, -1.5, 8, 6.125},
		{14.6, 14.2, 5, 5},
	} {
		m := Fill(Rect(r[0], r[1], r[2], r[3], Identity), NonZero, 16, 16)
		require.Len(t, m.A, 16*16)
		for y := 0; y < 16; y++ {
			for x := 0; x < 16; x++ {
				ox := max(0, min(float64(x+1), r[0]+r[2])-max(float64(x), r[0]))
				oy := max(0, min(float64(y+1), r[1]+r[3])-max(float64(y), r[1]))
				assert.InDelta(t, ox*oy, m.A[y*16+x], 1.0/16, "rect %v at (%d, %d)", r, x, y)
			}
		}
	}
}

func TestFill_Holes(t *testing.T) {
	outer := []Point{{2, 2}, {18, 2}, {18, 18}, {2, 18}}
	inner := []Point{{6, 6}, {14, 6}, {14, 14}, {6, 14}}
	reversed := []Point{{6, 6}, {6, 14}, {14, 14}, {14, 6}}
	at := func(m *Mask, x, y int) float32 { return m.A[y*m.W+x] }

	same := &Path{Subpaths: [][]Point{outer, inner}, Closed: []bool{true, true}}
	m := Fill(same, EvenOdd, 20, 20)
	assert.InDelta(t, 0, at(m, 10, 10), 1e-6)
	assert.InDelta(t, 1, at(m, 3, 3), 1e-6)
	assert.InDelta(t, 256-64, sum(m), 0.5)
	m = Fill(same, NonZero, 20, 20)
	assert.InDelta(t, 1, at(m, 10, 10), 1e-6)
	assert.InDelta(t, 256, sum(m), 0.5)

	opposite := &Path{Subpaths: [][]Point{outer, reversed}, Closed: []bool{true, true}}
	for _, rule := range []FillRule{NonZero, EvenOdd} {
		m := Fill(opposite, rule, 20, 20)
		assert.InDelta(t, 0, at(m, 10, 10), 1e-6, "rule %d", rule)
		assert.InDelta(t, 256-64, sum(m), 0.5, "rule %d", rule)
	}
}

func TestFill_Circle(t *testing.T) {
	const r = 20.0
	for _, d := range []string{
		"M10 30a20 20 0 1 1 40 0a20 20 0 1 1-40 0z",
		"M10 30a20 20 0 0 0 40 0a20 20 0 0 0-40 0",
	} {
		p := mustParse(t, d)
		for _, rule := range []FillRule{NonZero, EvenOdd} {
			got := sum(Fill(p, rule, 64, 64))
			assert.InEpsilon(t, math.Pi*r*r, got, 0.01, d)
		}
	}

	got := sum(Fill(Ellipse(30, 30, 20, 10, Identity, 0.1), NonZero, 64, 64))
	assert.InEpsilon(t, math.Pi*20*10, got, 0.01)
}

func TestFill_ClipsAndBounds(t *testing.T) {
	m := Fill(Rect(-100, -100, 1000, 1000, Identity), NonZero, 7, 5)
	for _, a := range m.A {
		assert.InDelta(t, 1, a, 1e-6)
	}

	// Overlapping same-direction shapes stay capped at full coverage.
	p := &Path{
		Subpaths: append(Rect(0, 0, 4, 4, Identity).Subpaths, Rect(1, 1, 2, 2, Identity).Subpaths...),
		Closed:   []bool{true, true},
	}
	for _, a := range Fill(p, NonZero, 4, 4).A {
		assert.LessOrEqual(t, a, float32(1))
		assert.GreaterOrEqual(t, a, float32(0))
	}

	assert.Empty(t, Fill(Rect(0, 0, 1, 1, Identity), NonZero, 0, 3).A)
}

func TestStroke(t *testing.T) {
	area := func(p *Path) float64 { return sum(Fill(p, NonZero, 100, 100)) }

	t.Run("horizontal line", func(t *testing.T) {
		got := area(Stroke(mustParse(t, "M10 50H90"), 4))
		assert.InDelta(t, 4*80, got, 1)
	})

	t.Run("polyline with joins", func(t *testing.T) {
		// Round joins add at most a quarter disk at the turn.
		got := area(Stroke(mustParse(t, "M10 20H60V80"), 4))
		want := 4.0*50 + 4*60
		assert.GreaterOrEqual(t, got, want-1)
		assert.LessOrEqual(t, got, want+math.Pi*4/4+1)
	})

	t.Run("closed square", func(t *testing.T) {
		// The stroke of a closed square covers a ring with rounded outer corners.
		p := Stroke(mustParse(t, "M20 20h40v40h-40z"), 4)
		want := 44.0*44 - 36*36 - (4-math.Pi)*4
		assert.InDelta(t, want, area(p), 1)

		open := Stroke(mustParse(t, "M20 20h40v40h-40"), 4)
		assert.Less(t, area(open), area(p)-30)
	})

	t.Run("pieces share one orientation", func(t *testing.T) {
		for _, d := range []string{"M10 10L90 90L10 90L90 10", "M50 10a40 40 0 1 0 1 0z", "M10 10h50v50h-50z"} {
			p := Stroke(mustParse(t, d), 3)
			require.NotEmpty(t, p.Subpaths)
			for _, sp := range p.Subpaths {
				assert.GreaterOrEqual(t, signedArea(sp), 0.0, d)
			}
		}
		// A self-crossing stroke must not cancel itself where it overlaps.
		m := Fill(Stroke(mustParse(t, "M10 10L90 90L10 90L90 10"), 3), NonZero, 100, 100)
		assert.InDelta(t, 1, m.A[50*100+50], 1e-6)
	})

	t.Run("ring", func(t *testing.T) {
		p := Stroke(Ellipse(50, 50, 30, 30, Identity, 0.1), 4)
		want := math.Pi * (32*32 - 28*28)
		assert.InEpsilon(t, want, area(p), 0.01)
	})
}
