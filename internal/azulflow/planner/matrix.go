package planner

import (
	"strings"
	"unicode/utf16"

	"github.com/cockroachdb/errors"

	"gogs.io/gogs/internal/azulflow/expr"
	"gogs.io/gogs/internal/azulflow/template"
)

const (
	// MaxMatrixJobs is the most jobs one matrix can produce, as on GitHub.
	MaxMatrixJobs = 256
	// maxMatrixProduct bounds the combinations considered before exclusions,
	// so that a huge product that excludes almost everything cannot stall
	// the planner.
	maxMatrixProduct = 1 << 20
	maxJobNameLength = 100
)

// configuration is one job of a strategy.
type configuration struct {
	matrix *expr.Object // Nil for a job without a matrix.
	name   string       // The default display name.
}

// strategy is an evaluated job strategy.
type strategy struct {
	failFast    bool
	maxParallel int // Zero means no limit.
	matrix      bool
	configs     []configuration
}

// context returns the strategy context of the configuration i.
func (s *strategy) context(i int) *expr.Object {
	ctx := expr.NewObject()
	ctx.Set("fail-fast", s.failFast)
	ctx.Set("job-index", float64(i))
	ctx.Set("job-total", float64(len(s.configs)))
	switch {
	case !s.matrix:
		ctx.Set("max-parallel", 1.0)
	case s.maxParallel > 0:
		ctx.Set("max-parallel", float64(s.maxParallel))
	default:
		ctx.Set("max-parallel", float64(len(s.configs)))
	}
	return ctx
}

// convertStrategy converts an evaluated strategy, as GitHub's
// ConvertToStrategy does. Problems with the matrix are reported to ctx.
func convertStrategy(ctx *template.Context, t *template.Token, jobName string) (*strategy, error) {
	s := &strategy{failFast: true}
	if err := t.AssertMapping("strategy"); err != nil {
		return nil, err
	}
	var b *matrixBuilder
	for _, p := range t.Map {
		if err := p.Key.AssertString("strategy key"); err != nil {
			return nil, err
		}
		v := p.Value
		switch p.Key.Str {
		case "fail-fast":
			if err := v.AssertBoolean("strategy fail-fast"); err != nil {
				return nil, err
			}
			s.failFast = v.Bool
		case "max-parallel":
			if err := v.AssertNumber("strategy max-parallel"); err != nil {
				return nil, err
			}
			s.maxParallel = int(v.Num)
		case "matrix":
			if err := v.AssertMapping("matrix"); err != nil {
				return nil, err
			}
			b = &matrixBuilder{ctx: ctx, jobName: jobName, vectors: expr.NewObject()}
			hasVector, hasInclude := false, false
			for _, m := range v.Map {
				if err := m.Key.AssertString("matrix key"); err != nil {
					return nil, err
				}
				switch m.Key.Str {
				case "include":
					if err := m.Value.AssertSequence("matrix includes"); err != nil {
						return nil, err
					}
					hasInclude = len(m.Value.Seq) > 0
					b.include = m.Value
				case "exclude":
					if err := m.Value.AssertSequence("matrix excludes"); err != nil {
						return nil, err
					}
					b.exclude = m.Value
				default:
					hasVector = true
					if err := m.Value.AssertSequence("matrix vector value"); err != nil {
						return nil, err
					}
					if len(m.Value.Seq) == 0 {
						ctx.Error(m.Value, "Matrix vector '"+m.Key.Str+"' does not contain any values")
					} else {
						b.vectors.Set(m.Key.Str, m.Value.Value())
					}
				}
			}
			if !hasVector && !hasInclude {
				ctx.Error(v, "Matrix must define at least one vector")
			}
		default:
			return nil, errors.New("Error while reading 'strategy key'. Unexpected value '" + p.Key.Str + "'")
		}
	}
	if b != nil && len(ctx.Errors()) == 0 {
		configs, err := b.build()
		if err != nil {
			return nil, err
		}
		s.configs = configs
		s.matrix = len(configs) > 0
	}
	return s, nil
}

// matrixBuilder is a port of GitHub's MatrixBuilder.
type matrixBuilder struct {
	ctx     *template.Context
	jobName string
	vectors *expr.Object
	include *template.Token
	exclude *template.Token
}

// matrixFilter compares the leaves of an include or exclude entry with a
// combination, the way GitHub compares them with expressions such as
// matrix['os'] == 'linux'.
type matrixFilter struct {
	leaves []matrixLeaf
}

type matrixLeaf struct {
	path  []any // Mapping keys and sequence indexes.
	value any
}

func newMatrixFilter(entries []*template.Pair) *matrixFilter {
	f := &matrixFilter{}
	var walk func(path []any, t *template.Token)
	walk = func(path []any, t *template.Token) {
		switch t.Kind {
		case template.Mapping:
			for _, p := range t.Map {
				walk(append(path[:len(path):len(path)], p.Key.Str), p.Value)
			}
		case template.Sequence:
			for i, item := range t.Seq {
				walk(append(path[:len(path):len(path)], i), item)
			}
		default:
			f.leaves = append(f.leaves, matrixLeaf{path: path, value: t.Value()})
		}
	}
	for _, p := range entries {
		walk([]any{p.Key.Str}, p.Value)
	}
	return f
}

func (f *matrixFilter) match(matrix *expr.Object) bool {
	for _, leaf := range f.leaves {
		var v any = matrix
		for _, seg := range leaf.path {
			switch seg := seg.(type) {
			case string:
				obj, _ := v.(*expr.Object)
				v, _ = obj.Get(seg)
			case int:
				arr, _ := v.([]any)
				if seg < len(arr) {
					v = arr[seg]
				} else {
					v = nil
				}
			}
		}
		if !expr.Equal(v, leaf.value) {
			return false
		}
	}
	return true
}

type matrixInclude struct {
	filter  *matrixFilter
	entries []*template.Pair // The entries for vectors.
	extra   *expr.Object     // The other entries.
	matched bool
}

func (b *matrixBuilder) build() ([]configuration, error) {
	var includes []*matrixInclude
	if b.include != nil {
		for _, item := range b.include.Seq {
			if err := item.AssertMapping("matrix includes item"); err != nil {
				return nil, err
			}
			inc := &matrixInclude{extra: expr.NewObject()}
			for _, p := range item.Map {
				if err := p.Key.AssertString("matrix include item key"); err != nil {
					return nil, err
				}
				if _, ok := b.vectors.Get(p.Key.Str); ok {
					inc.entries = append(inc.entries, p)
				} else {
					inc.extra.Set(p.Key.Str, p.Value.Value())
				}
			}
			if len(inc.entries) == 0 && inc.extra.Len() == 0 {
				b.ctx.Error(item, "Matrix include mapping does not contain any values")
				continue
			}
			inc.filter = newMatrixFilter(inc.entries)
			includes = append(includes, inc)
		}
	}
	var excludes []*matrixFilter
	if b.exclude != nil {
		for _, item := range b.exclude.Seq {
			if err := item.AssertMapping("matrix excludes item"); err != nil {
				return nil, err
			}
			if len(item.Map) == 0 {
				b.ctx.Error(item, "Matrix exclude filter must not be empty")
				continue
			}
			for _, p := range item.Map {
				if err := p.Key.AssertString("matrix excludes item key"); err != nil {
					return nil, err
				}
				if _, ok := b.vectors.Get(p.Key.Str); !ok {
					b.ctx.Error(p.Key, "Matrix exclude key '"+p.Key.Str+"' does not match any key within the matrix")
				}
			}
			excludes = append(excludes, newMatrixFilter(item.Map))
		}
	}
	if len(b.ctx.Errors()) > 0 {
		return nil, nil
	}

	keys := b.vectors.Keys()
	size := 0
	if len(keys) > 0 {
		size = 1
		for _, k := range keys {
			v, _ := b.vectors.Get(k)
			size *= len(v.([]any))
			if size > maxMatrixProduct {
				return nil, errors.Newf("The matrix has more than %d combinations before exclusions", maxMatrixProduct)
			}
		}
	}

	var configs []configuration
	add := func(c configuration) error {
		configs = append(configs, c)
		if len(configs) > MaxMatrixJobs {
			return errors.Newf("The matrix produced more than %d jobs", MaxMatrixJobs)
		}
		return nil
	}
	for i := range size {
		combo := expr.NewObject()
		block := size
		for _, k := range keys {
			v, _ := b.vectors.Get(k)
			vector := v.([]any)
			block /= len(vector)
			combo.Set(k, vector[(i/block)%len(vector)])
		}
		if matchesAny(excludes, combo) {
			continue
		}
		extra := expr.NewObject()
		for _, inc := range includes {
			if inc.filter.match(combo) {
				inc.matched = true
				for _, k := range inc.extra.Keys() {
					v, _ := inc.extra.Get(k)
					extra.Set(k, v)
				}
			}
		}
		if err := add(b.configuration(combo, extra)); err != nil {
			return nil, err
		}
	}
	for _, inc := range includes {
		if inc.matched {
			continue
		}
		combo := expr.NewObject()
		for _, p := range inc.entries {
			combo.Set(p.Key.Str, p.Value.Value())
		}
		for _, k := range inc.extra.Keys() {
			v, _ := inc.extra.Get(k)
			combo.Set(k, v)
		}
		if err := add(b.configuration(combo, nil)); err != nil {
			return nil, err
		}
	}
	return configs, nil
}

func matchesAny(filters []*matrixFilter, combo *expr.Object) bool {
	for _, f := range filters {
		if f.match(combo) {
			return true
		}
	}
	return false
}

// configuration names a combination after its values, then adds the extra
// values of the include entries it matched, which the name leaves out.
func (b *matrixBuilder) configuration(combo, extra *expr.Object) configuration {
	var segments []string
	var walk func(v any)
	walk = func(v any) {
		switch v := v.(type) {
		case []any:
			for _, item := range v {
				walk(item)
			}
		case *expr.Object:
			for _, k := range v.Keys() {
				item, _ := v.Get(k)
				walk(item)
			}
		case bool, float64, string:
			if s := expr.ToString(v); s != "" {
				segments = append(segments, s)
			}
		}
	}
	walk(combo)
	for _, k := range extra.Keys() {
		v, _ := extra.Get(k)
		combo.Set(k, v)
	}
	return configuration{matrix: combo, name: jobName(b.jobName, segments)}
}

// jobName is the display name of a job, with the values of its matrix
// combination in parentheses, cut to 100 characters as GitHub does.
func jobName(base string, segments []string) string {
	if base == "" {
		return ""
	}
	name := base
	if len(segments) > 0 {
		name += " (" + strings.Join(segments, ", ") + ")"
	}
	if units := utf16.Encode([]rune(name)); len(units) > maxJobNameLength {
		name = string(utf16.Decode(units[:maxJobNameLength-3])) + "..."
	}
	return name
}
