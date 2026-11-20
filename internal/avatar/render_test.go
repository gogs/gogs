package avatar

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseTranslate(t *testing.T) {
	tests := []struct {
		in     string
		x, y   float64
		hasErr bool
	}{
		{in: "translate(532 491)", x: 532, y: 491},
		{in: "translate(805.2 732.6)", x: 805.2, y: 732.6},
		{in: "translate(424,305.7)", x: 424, y: 305.7},
		{in: "translate( -1.5 , 2 )", x: -1.5, y: 2},
		{in: "translate(7)", x: 7},
		{in: "translate(1e2 -3)", x: 100, y: -3},
		{in: "scale(2)", hasErr: true},
		{in: "translate(1 2) scale(2)", hasErr: true},
		{in: "translate(a b)", hasErr: true},
		{in: "translate()", hasErr: true},
	}
	for _, test := range tests {
		t.Run(test.in, func(t *testing.T) {
			x, y, err := parseTranslate(test.in)
			if test.hasErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, test.x, x)
			assert.Equal(t, test.y, y)
		})
	}
}

func TestParsePaint(t *testing.T) {
	tests := []struct {
		in     string
		want   paint
		hasErr bool
	}{
		{in: "none", want: paint{none: true}},
		{in: "white", want: paint{r: 1, g: 1, b: 1}},
		{in: "#ffffff", want: paint{r: 1, g: 1, b: 1}},
		{in: "#000000", want: paint{}},
		{in: "#FF0080", want: paint{r: 1, g: 0, b: 128.0 / 255}},
		{in: "#fff", hasErr: true},
		{in: "red", hasErr: true},
		{in: "#gggggg", hasErr: true},
		{in: "url(#gradient)", hasErr: true},
	}
	for _, test := range tests {
		t.Run(test.in, func(t *testing.T) {
			got, err := parsePaint(test.in)
			if test.hasErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, test.want, got)
		})
	}
}

func TestParseSVG_generated(t *testing.T) {
	// Seen features prove that the seeds exercise the whole supported subset,
	// so that a DiceBear update cannot silently drop one out of the test.
	seen := make(map[string]bool)
	var walk func(e *element)
	walk = func(e *element) {
		seen["<"+e.name+">"] = true
		if e.maskID != "" {
			seen["mask="] = true
		}
		if e.blendDifference {
			seen["mix-blend-mode"] = true
		}
		if e.stroke != nil && !e.stroke.none {
			seen["stroke"] = true
		}
		if e.fillRule != 0 {
			seen["fill-rule"] = true
		}
		for _, child := range e.children {
			walk(child)
		}
	}

	for i := range 500 {
		seed := strconv.Itoa(i)
		svg, err := Generate(seed)
		require.NoError(t, err)
		doc, err := parseSVG(svg)
		require.NoError(t, err, "seed %q", seed)
		walk(doc.root)
	}

	for _, feature := range []string{"<svg>", "<defs>", "<g>", "<use>", "<path>", "<rect>", "<ellipse>", "<mask>", "mask=", "mix-blend-mode", "stroke", "fill-rule"} {
		assert.True(t, seen[feature], "no seed uses %s", feature)
	}
}

func TestParseSVG_unsupported(t *testing.T) {
	const svg = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10" fill="none">%s</svg>`
	tests := []struct {
		name    string
		content string
	}{
		{name: "element", content: `<circle cx="1" cy="1" r="1"/>`},
		{name: "attribute", content: `<path d="M0 0h1v1z" opacity="0.5"/>`},
		{name: "transform", content: `<g transform="rotate(45)"/>`},
		{name: "style", content: `<g style="mix-blend-mode:multiply"/>`},
		{name: "mask type", content: `<defs><mask id="m"><path d="M0 0h1v1z" fill="white"/></mask></defs>`},
		{name: "unknown reference", content: `<use href="#missing"/>`},
		{name: "mask reference", content: `<g mask="url(#missing)"/>`},
		{name: "rounded rectangle", content: `<rect width="1" height="1" rx="1"/>`},
		{name: "text", content: `<g>hello</g>`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := parseSVG(fmt.Appendf(nil, svg, test.content))
			assert.Error(t, err)
		})
	}

	t.Run("ignorable", func(t *testing.T) {
		content := `<!-- comment --><metadata><anything at="all"/></metadata>` +
			`<defs><clipPath id="c"><rect width="10" height="10"/></clipPath></defs>` +
			`<g clip-path="url(#c)"><path d="M0 0h1v1z" clip-rule="evenodd" fill-rule="evenodd" fill="#000000"/></g>`
		_, err := parseSVG(fmt.Appendf(nil, svg, content))
		assert.NoError(t, err)
	})
}

func TestBlendDifference(t *testing.T) {
	// Opaque white on opaque pastel inverts the pastel.
	dst := layer{0.8, 0.6, 0.4, 1}
	blendDifference(dst, layer{1, 1, 1, 1})
	assert.InDeltaSlice(t, []float32{0.2, 0.4, 0.6, 1}, dst, 1e-6)

	// Half covered white over opaque black blends to half gray.
	dst = layer{0, 0, 0, 1}
	blendDifference(dst, layer{0.5, 0.5, 0.5, 0.5})
	assert.InDeltaSlice(t, []float32{0.5, 0.5, 0.5, 1}, dst, 1e-6)

	// Over a transparent backdrop, the source is drawn as is.
	dst = layer{0, 0, 0, 0}
	blendDifference(dst, layer{0.25, 0.5, 0, 0.5})
	assert.InDeltaSlice(t, []float32{0.25, 0.5, 0, 0.5}, dst, 1e-6)
}

func TestSourceOver(t *testing.T) {
	dst := layer{0, 0, 1, 1}
	sourceOver(dst, layer{0.5, 0, 0, 0.5})
	assert.InDeltaSlice(t, []float32{0.5, 0, 0.5, 1}, dst, 1e-6)
}

func TestGeneratePNG(t *testing.T) {
	const size = 64
	decode := func(t *testing.T, seed string) ([]byte, image.Image) {
		data, err := GeneratePNG(seed, size)
		require.NoError(t, err)
		img, err := png.Decode(bytes.NewReader(data))
		require.NoError(t, err)
		return data, img
	}

	alice, img := decode(t, "alice")
	assert.Equal(t, image.Rect(0, 0, size, size), img.Bounds())

	var backgrounds []string
	for _, c := range backgroundColors {
		backgrounds = append(backgrounds, "#"+c)
	}
	r, g, b, a := img.At(0, 0).RGBA()
	assert.Equal(t, uint32(0xffff), a, "top-left pixel is not opaque")
	assert.Contains(t, backgrounds, fmt.Sprintf("#%02x%02x%02x", r>>8, g>>8, b>>8))

	again, _ := decode(t, "alice")
	assert.Equal(t, alice, again, "same seed draws a different avatar")

	bob, _ := decode(t, "bob")
	assert.NotEqual(t, alice, bob, "different seeds draw the same avatar")
}
