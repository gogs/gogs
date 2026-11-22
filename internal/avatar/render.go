package avatar

import (
	"bytes"
	"encoding/xml"
	"image"
	"image/png"
	"io"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/cockroachdb/errors"

	"gogs.io/gogs/internal/avatar/raster"
)

const (
	// flattenTolerance is how far in device pixels flattened curves may stray
	// from the true curve, well below what anti-aliasing can show.
	flattenTolerance = 0.2
	// maxUseDepth bounds how deeply <use> may nest, so that a reference cycle
	// fails instead of recursing forever.
	maxUseDepth = 16
)

// GeneratePNG returns the PNG of the avatar of the kind drawn from the seed,
// size×size pixels.
func GeneratePNG(kind Kind, seed string, size int) ([]byte, error) {
	if size <= 0 {
		return nil, errors.Newf("invalid size %d", size)
	}
	svg, err := Generate(kind, seed)
	if err != nil {
		return nil, errors.Wrap(err, "generate SVG")
	}
	doc, err := parseSVG(svg)
	if err != nil {
		return nil, errors.Wrap(err, "parse SVG")
	}
	pixels, err := doc.render(size)
	if err != nil {
		return nil, errors.Wrap(err, "render")
	}

	var buf bytes.Buffer
	if err = png.Encode(&buf, toNRGBA(pixels, size)); err != nil {
		return nil, errors.Wrap(err, "encode PNG")
	}
	return buf.Bytes(), nil
}

// paint is a solid color with each channel in [0, 1], or none.
type paint struct {
	none    bool
	r, g, b float32
}

// element is a parsed SVG element of the subset that DiceBear generates. Fields
// that do not apply to an element keep their zero values.
type element struct {
	name     string
	id       string
	parent   *element
	children []*element

	// transform is nil when the element has none.
	transform *raster.Matrix

	// fill, stroke and strokeWidth are nil when inherited.
	fill        *paint
	stroke      *paint
	strokeWidth *float64
	fillRule    raster.FillRule

	d                    string
	x, y, width, height  float64
	cx, cy, rx, ry       float64
	viewBox              [4]float64
	hasViewBox           bool
	maskID               string
	blendDifference      bool
	href                 string
	hasMaskTypeLuminance bool
}

type document struct {
	root *element
	ids  map[string]*element
}

// allowedAttributes lists the attributes each supported element may carry,
// besides ignorableAttributes.
var allowedAttributes = map[string][]string{
	"svg":     {"viewBox", "fill"},
	"defs":    {},
	"g":       {"id", "transform", "fill", "stroke", "stroke-width", "mask", "style"},
	"use":     {"href", "transform"},
	"path":    {"d", "fill", "fill-rule", "stroke", "stroke-width"},
	"rect":    {"x", "y", "width", "height", "fill", "rx", "ry"},
	"ellipse": {"cx", "cy", "rx", "ry", "fill", "stroke", "stroke-width"},
	"mask":    {"id", "style"},
}

// ignorableAttributes do not change the pixels: clipping only ever covers the
// whole canvas, which clips anyway, and classes only matter to stylesheets,
// which the subset has none of.
var ignorableAttributes = []string{"clip-path", "clip-rule", "class", "aria-hidden", "shape-rendering", "xmlns"}

// ignorableElements are skipped along with their content, for the same reasons
// as ignorableAttributes.
var ignorableElements = []string{"metadata", "clipPath"}

// parseSVG parses the subset of SVG that DiceBear generates, and returns an
// error for anything outside of it rather than drawing it wrong.
func parseSVG(data []byte) (*document, error) {
	doc := &document{ids: make(map[string]*element)}
	dec := xml.NewDecoder(bytes.NewReader(data))
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		} else if err != nil {
			return nil, errors.Wrap(err, "decode XML")
		}

		switch t := tok.(type) {
		case xml.StartElement:
			if doc.root != nil {
				return nil, errors.New("more than one root element")
			}
			if t.Name.Local != "svg" {
				return nil, errors.Newf("root element is <%s>, not <svg>", t.Name.Local)
			}
			doc.root, err = doc.parseElement(dec, t, nil)
			if err != nil {
				return nil, err
			}
		case xml.CharData:
			if err = checkBlank(t); err != nil {
				return nil, err
			}
		}
	}
	if doc.root == nil {
		return nil, errors.New("no <svg> element")
	}
	if !doc.root.hasViewBox {
		return nil, errors.New("<svg> has no viewBox")
	}
	if err := doc.resolve(doc.root); err != nil {
		return nil, err
	}
	return doc, nil
}

func checkBlank(data xml.CharData) error {
	if len(bytes.TrimSpace(data)) > 0 {
		return errors.Newf("unsupported text %q", string(data))
	}
	return nil
}

func (doc *document) parseElement(dec *xml.Decoder, start xml.StartElement, parent *element) (*element, error) {
	e := &element{name: start.Name.Local, parent: parent}
	allowed, ok := allowedAttributes[e.name]
	if !ok {
		return nil, errors.Newf("unsupported element <%s>", e.name)
	}
	for _, attr := range start.Attr {
		if err := e.setAttribute(attr, allowed); err != nil {
			return nil, errors.Wrapf(err, "<%s>", e.name)
		}
	}
	switch {
	case e.name == "path" && e.d == "":
		return nil, errors.New("<path> has no d")
	case e.name == "use" && e.href == "":
		return nil, errors.New("<use> has no href")
	case e.name == "mask" && !e.hasMaskTypeLuminance:
		return nil, errors.New("<mask> is not mask-type:luminance")
	}
	if e.id != "" {
		if _, ok := doc.ids[e.id]; ok {
			return nil, errors.Newf("duplicate id %q", e.id)
		}
		doc.ids[e.id] = e
	}

	for {
		tok, err := dec.Token()
		if err != nil {
			return nil, errors.Wrap(err, "decode XML")
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if slices.Contains(ignorableElements, t.Name.Local) {
				if err = dec.Skip(); err != nil {
					return nil, errors.Wrap(err, "decode XML")
				}
				continue
			}
			if t.Name.Local == "svg" {
				return nil, errors.New("nested <svg>")
			}
			child, err := doc.parseElement(dec, t, e)
			if err != nil {
				return nil, err
			}
			e.children = append(e.children, child)
		case xml.EndElement:
			return e, nil
		case xml.CharData:
			if err = checkBlank(t); err != nil {
				return nil, err
			}
		}
	}
}

func (e *element) setAttribute(attr xml.Attr, allowed []string) error {
	name := attr.Name.Local
	if attr.Name.Space == "xmlns" {
		return nil
	} else if attr.Name.Space != "" {
		name = attr.Name.Space + ":" + name
	}
	if slices.Contains(ignorableAttributes, name) {
		return nil
	}
	if !slices.Contains(allowed, name) {
		return errors.Newf("unsupported attribute %q", name)
	}

	value := attr.Value
	var err error
	switch name {
	case "id":
		e.id = value
	case "viewBox":
		e.viewBox, err = parseViewBox(value)
		e.hasViewBox = true
	case "transform":
		var m raster.Matrix
		m, err = parseTransform(value)
		e.transform = &m
	case "fill", "stroke":
		var p paint
		p, err = parsePaint(value)
		if name == "fill" {
			e.fill = &p
		} else {
			e.stroke = &p
		}
	case "stroke-width":
		var w float64
		w, err = parseNumber(value)
		if err == nil && w < 0 {
			err = errors.New("negative")
		}
		e.strokeWidth = &w
	case "fill-rule":
		switch value {
		case "nonzero":
			e.fillRule = raster.NonZero
		case "evenodd":
			e.fillRule = raster.EvenOdd
		default:
			err = errors.New("unknown fill rule")
		}
	case "d":
		e.d = value
	case "x":
		e.x, err = parseNumber(value)
	case "y":
		e.y, err = parseNumber(value)
	case "width":
		e.width, err = parseNumber(value)
	case "height":
		e.height, err = parseNumber(value)
	case "cx":
		e.cx, err = parseNumber(value)
	case "cy":
		e.cy, err = parseNumber(value)
	case "rx", "ry":
		var r float64
		r, err = parseNumber(value)
		if err == nil && e.name == "rect" && r != 0 {
			err = errors.New("rounded rectangles are unsupported")
		}
		if name == "rx" {
			e.rx = r
		} else {
			e.ry = r
		}
	case "mask":
		id, hasPrefix := strings.CutPrefix(value, "url(#")
		id, hasSuffix := strings.CutSuffix(id, ")")
		if !hasPrefix || !hasSuffix || id == "" {
			err = errors.New("not url(#id)")
		}
		e.maskID = id
	case "href":
		id, ok := strings.CutPrefix(value, "#")
		if !ok || id == "" {
			err = errors.New("not #id")
		}
		e.href = id
	case "style":
		err = e.setStyle(value)
	}
	return errors.Wrapf(err, "attribute %s=%q", name, value)
}

func (e *element) setStyle(style string) error {
	for decl := range strings.SplitSeq(style, ";") {
		if strings.TrimSpace(decl) == "" {
			continue
		}
		prop, value, ok := strings.Cut(decl, ":")
		prop, value = strings.TrimSpace(prop), strings.TrimSpace(value)
		switch {
		case ok && e.name == "g" && prop == "mix-blend-mode" && value == "difference":
			e.blendDifference = true
		case ok && e.name == "mask" && prop == "mask-type" && value == "luminance":
			e.hasMaskTypeLuminance = true
		default:
			return errors.Newf("unsupported style %q", strings.TrimSpace(decl))
		}
	}
	return nil
}

// resolve checks that every reference in the tree of e points to an element
// of the right kind.
func (doc *document) resolve(e *element) error {
	if e.href != "" {
		target, ok := doc.ids[e.href]
		if !ok {
			return errors.Newf("<use> references unknown id %q", e.href)
		}
		if target.name == "mask" {
			return errors.Newf("<use> references <mask> %q", e.href)
		}
	}
	if e.maskID != "" {
		target, ok := doc.ids[e.maskID]
		if !ok || target.name != "mask" {
			return errors.Newf("mask references unknown <mask> %q", e.maskID)
		}
	}
	for _, child := range e.children {
		if err := doc.resolve(child); err != nil {
			return err
		}
	}
	return nil
}

func parseNumber(s string) (float64, error) {
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0, errors.Newf("invalid number %q", s)
	}
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, errors.Newf("non-finite number %q", s)
	}
	return v, nil
}

func parseViewBox(s string) (box [4]float64, err error) {
	fields := strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' || r == '\n' || r == '\r' })
	if len(fields) != len(box) {
		return box, errors.Newf("want 4 numbers, got %d", len(fields))
	}
	for i, field := range fields {
		if box[i], err = parseNumber(field); err != nil {
			return box, err
		}
	}
	if box[2] <= 0 || box[3] <= 0 {
		return box, errors.New("empty view box")
	}
	return box, nil
}

// parseTransform parses a list of matrix, translate, scale and rotate
// transforms into the matrix of applying them from the last to the first.
func parseTransform(s string) (raster.Matrix, error) {
	m := raster.Identity
	rest := strings.TrimSpace(s)
	if rest == "" {
		return m, errors.New("empty transform")
	}
	for rest != "" {
		name, after, ok := strings.Cut(rest, "(")
		if !ok {
			return m, errors.New("unsupported transform")
		}
		list, after, ok := strings.Cut(after, ")")
		if !ok {
			return m, errors.New("unsupported transform")
		}
		rest = strings.TrimLeft(after, " \t\n\r,")

		var args []float64
		for field := range strings.FieldsFuncSeq(list, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' || r == '\n' || r == '\r' }) {
			v, err := parseNumber(field)
			if err != nil {
				return m, err
			}
			args = append(args, v)
		}

		switch name = strings.TrimSpace(name); {
		case name == "matrix" && len(args) == 6:
			m = m.Mul(raster.Matrix(args))
		case name == "translate" && len(args) == 1:
			m = m.Translate(args[0], 0)
		case name == "translate" && len(args) == 2:
			m = m.Translate(args[0], args[1])
		case name == "scale" && len(args) == 1:
			m = m.Scale(args[0], args[0])
		case name == "scale" && len(args) == 2:
			m = m.Scale(args[0], args[1])
		case name == "rotate" && len(args) == 1:
			m = m.Rotate(args[0])
		case name == "rotate" && len(args) == 3:
			m = m.Translate(args[1], args[2]).Rotate(args[0]).Translate(-args[1], -args[2])
		default:
			return m, errors.Newf("unsupported transform %s with %d arguments", name, len(args))
		}
	}
	return m, nil
}

func parsePaint(s string) (paint, error) {
	switch s = strings.TrimSpace(s); s {
	case "none":
		return paint{none: true}, nil
	case "white":
		return paint{r: 1, g: 1, b: 1}, nil
	}
	hex, ok := strings.CutPrefix(s, "#")
	if !ok || len(hex) != 6 {
		return paint{}, errors.Newf("unsupported color %q", s)
	}
	v, err := strconv.ParseUint(hex, 16, 32)
	if err != nil {
		return paint{}, errors.Newf("unsupported color %q", s)
	}
	return paint{
		r: float32(v>>16&0xff) / 255,
		g: float32(v>>8&0xff) / 255,
		b: float32(v&0xff) / 255,
	}, nil
}

// state is what an element inherits from its context.
type state struct {
	m           raster.Matrix
	fill        paint
	stroke      paint
	strokeWidth float64
}

// initialState holds the initial values of the properties in SVG.
func initialState(m raster.Matrix) state {
	return state{m: m, stroke: paint{none: true}, strokeWidth: 1}
}

// within returns the state of the content of e in the context s.
func (s state) within(e *element) state {
	if e.transform != nil {
		s.m = s.m.Mul(*e.transform)
	}
	if e.fill != nil {
		s.fill = *e.fill
	}
	if e.stroke != nil {
		s.stroke = *e.stroke
	}
	if e.strokeWidth != nil {
		s.strokeWidth = *e.strokeWidth
	}
	return s
}

// layer holds premultiplied RGBA pixels, row by row.
type layer []float32

type renderer struct {
	doc  *document
	size int
}

// render paints the document onto a transparent size×size layer.
func (doc *document) render(size int) (layer, error) {
	box := doc.root.viewBox
	m := raster.Identity.
		Scale(float64(size)/box[2], float64(size)/box[3]).
		Translate(-box[0], -box[1])
	r := &renderer{doc: doc, size: size}
	dst := r.newLayer()
	if err := r.draw(dst, doc.root, initialState(m), 0); err != nil {
		return nil, err
	}
	return dst, nil
}

func (r *renderer) newLayer() layer {
	return make(layer, r.size*r.size*4)
}

func (r *renderer) draw(dst layer, e *element, s state, depth int) error {
	if depth > maxUseDepth {
		return errors.New("<use> nests too deeply")
	}
	s = s.within(e)

	switch e.name {
	case "defs", "mask":
		return nil
	case "use":
		return r.draw(dst, r.doc.ids[e.href], s, depth+1)
	case "path":
		p, err := raster.ParseSVGPath(e.d, s.m, flattenTolerance)
		if err != nil {
			return errors.Wrap(err, "parse path data")
		}
		return r.paintShape(dst, p, e.fillRule, s)
	case "rect":
		return r.paintShape(dst, raster.Rect(e.x, e.y, e.width, e.height, s.m), raster.NonZero, s)
	case "ellipse":
		return r.paintShape(dst, raster.Ellipse(e.cx, e.cy, e.rx, e.ry, s.m, flattenTolerance), raster.NonZero, s)
	}

	if e.maskID == "" && !e.blendDifference {
		return r.drawChildren(dst, e.children, s, depth)
	}

	// Masks and blending apply to the group as a whole, so it is painted on
	// its own layer first.
	group := r.newLayer()
	if err := r.drawChildren(group, e.children, s, depth); err != nil {
		return err
	}
	if e.maskID != "" {
		coverage, err := r.maskCoverage(r.doc.ids[e.maskID], s.m, depth)
		if err != nil {
			return err
		}
		for i, c := range coverage {
			for k := i * 4; k < i*4+4; k++ {
				group[k] *= c
			}
		}
	}
	if e.blendDifference {
		blendDifference(dst, group)
	} else {
		sourceOver(dst, group)
	}
	return nil
}

func (r *renderer) drawChildren(dst layer, children []*element, s state, depth int) error {
	for _, child := range children {
		if err := r.draw(dst, child, s, depth); err != nil {
			return err
		}
	}
	return nil
}

// maskCoverage returns the luminance of the content of the mask, per pixel. The
// content is drawn in the user space m of the masked element, and inherits
// properties from the ancestors of the mask rather than from the masked element.
func (r *renderer) maskCoverage(mask *element, m raster.Matrix, depth int) ([]float32, error) {
	var ancestors []*element
	for a := mask.parent; a != nil; a = a.parent {
		ancestors = append(ancestors, a)
	}
	s := initialState(raster.Identity)
	for i := len(ancestors) - 1; i >= 0; i-- {
		s = s.within(ancestors[i])
	}
	s = s.within(mask)
	s.m = m

	content := r.newLayer()
	if err := r.drawChildren(content, mask.children, s, depth); err != nil {
		return nil, err
	}
	coverage := make([]float32, r.size*r.size)
	for i := range coverage {
		// Premultiplied channels give luminance times alpha, which is what a
		// luminance mask uses.
		px := content[i*4 : i*4+4]
		coverage[i] = 0.2125*px[0] + 0.7154*px[1] + 0.0721*px[2]
	}
	return coverage, nil
}

// paintShape fills and then strokes p as s says, which is the paint order of
// SVG.
func (r *renderer) paintShape(dst layer, p *raster.Path, rule raster.FillRule, s state) error {
	if !s.fill.none {
		if err := r.paintCoverage(dst, raster.Fill(p, rule, r.size, r.size), s.fill); err != nil {
			return err
		}
	}
	if !s.stroke.none && s.strokeWidth > 0 {
		// Transforms here never skew or scale unevenly, so the scale factor of
		// widths is the square root of the determinant.
		scale := math.Sqrt(math.Abs(s.m[0]*s.m[3] - s.m[1]*s.m[2]))
		outline := raster.Stroke(p, s.strokeWidth*scale)
		if err := r.paintCoverage(dst, raster.Fill(outline, raster.NonZero, r.size, r.size), s.stroke); err != nil {
			return err
		}
	}
	return nil
}

// paintCoverage paints the color over dst with the coverage as alpha.
func (r *renderer) paintCoverage(dst layer, coverage *raster.Mask, color paint) error {
	if coverage.W != r.size || coverage.H != r.size || len(coverage.A) != r.size*r.size {
		return errors.Newf("coverage is %d×%d, want %d×%d", coverage.W, coverage.H, r.size, r.size)
	}
	for i, a := range coverage.A {
		if a <= 0 {
			continue
		}
		px := dst[i*4 : i*4+4]
		px[0] = color.r*a + px[0]*(1-a)
		px[1] = color.g*a + px[1]*(1-a)
		px[2] = color.b*a + px[2]*(1-a)
		px[3] = a + px[3]*(1-a)
	}
	return nil
}

// sourceOver composites src over dst.
func sourceOver(dst, src layer) {
	for i := 0; i < len(dst); i += 4 {
		as := src[i+3]
		if as <= 0 {
			continue
		}
		for k := i; k < i+4; k++ {
			dst[k] = src[k] + dst[k]*(1-as)
		}
	}
}

// blendDifference composites src onto dst with the difference blend mode, by
// the general formula of the W3C compositing spec on premultiplied colors:
// co = cs*(1 - ab) + cb*(1 - as) + as*ab*|Cb - Cs|, where as*ab*|Cb - Cs|
// equals |as*cb - ab*cs|.
func blendDifference(dst, src layer) {
	for i := 0; i < len(dst); i += 4 {
		as, ab := src[i+3], dst[i+3]
		if as <= 0 {
			continue
		}
		for k := i; k < i+3; k++ {
			cs, cb := src[k], dst[k]
			dst[k] = cs*(1-ab) + cb*(1-as) + float32(math.Abs(float64(as*cb-ab*cs)))
		}
		dst[i+3] = as + ab*(1-as)
	}
}

// toNRGBA converts premultiplied pixels to an image.
func toNRGBA(pixels layer, size int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	for i := 0; i < len(pixels); i += 4 {
		a := clamp01(pixels[i+3])
		if a == 0 {
			continue
		}
		for k := 0; k < 3; k++ {
			img.Pix[i+k] = to8(clamp01(pixels[i+k] / a))
		}
		img.Pix[i+3] = to8(a)
	}
	return img
}

func clamp01(v float32) float32 {
	return min(max(v, 0), 1)
}

func to8(v float32) uint8 {
	return uint8(math.Round(float64(v) * 255))
}
