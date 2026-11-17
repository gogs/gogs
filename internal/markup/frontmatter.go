package markup

import (
	"gopkg.in/yaml.v3"

	"gogs.io/gogs/internal/markup/dariomark"
)

// maxFrontMatterDepth is how deeply collections of front matter may nest.
const maxFrontMatterDepth = 16

// frontMatterValue returns the value of YAML front matter for Dariomark to
// render as a table, see dariomark.Options.FrontMatter. Like GitHub, YAML that
// is empty or is not a mapping is not front matter, and YAML with aliases is
// invalid, which also rules out aliases that multiply nodes, e.g., "billion
// laughs", or contain themselves. So is YAML that nests too deeply.
func frontMatterValue(src []byte) dariomark.FrontMatterValue {
	var doc yaml.Node
	if err := yaml.Unmarshal(src, &doc); err != nil {
		return dariomark.FrontMatterValue{Kind: dariomark.FrontMatterInvalid}
	}
	if len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode || len(doc.Content[0].Content) == 0 {
		return dariomark.FrontMatterValue{}
	}
	v, ok := convertFrontMatter(doc.Content[0], 0)
	if !ok {
		return dariomark.FrontMatterValue{Kind: dariomark.FrontMatterInvalid}
	}
	return v
}

// convertFrontMatter converts the YAML node n at depth, and reports false when
// n has aliases, keys other than scalars, or nests too deeply.
func convertFrontMatter(n *yaml.Node, depth int) (dariomark.FrontMatterValue, bool) {
	if depth > maxFrontMatterDepth {
		return dariomark.FrontMatterValue{}, false
	}
	switch n.Kind {
	case yaml.ScalarNode:
		v := dariomark.FrontMatterValue{Kind: dariomark.FrontMatterScalar}
		if n.ShortTag() != "!!null" {
			v.Text = n.Value
		}
		return v, true
	case yaml.MappingNode:
		v := dariomark.FrontMatterValue{
			Kind:   dariomark.FrontMatterMapping,
			Keys:   make([]string, 0, len(n.Content)/2),
			Values: make([]dariomark.FrontMatterValue, 0, len(n.Content)/2),
		}
		for i := 0; i+1 < len(n.Content); i += 2 {
			key := n.Content[i]
			if key.Kind != yaml.ScalarNode {
				return v, false
			}
			value, ok := convertFrontMatter(n.Content[i+1], depth+1)
			if !ok {
				return v, false
			}
			v.Keys = append(v.Keys, key.Value)
			v.Values = append(v.Values, value)
		}
		return v, true
	case yaml.SequenceNode:
		v := dariomark.FrontMatterValue{
			Kind:   dariomark.FrontMatterSequence,
			Values: make([]dariomark.FrontMatterValue, 0, len(n.Content)),
		}
		for _, item := range n.Content {
			value, ok := convertFrontMatter(item, depth+1)
			if !ok {
				return v, false
			}
			v.Values = append(v.Values, value)
		}
		return v, true
	}
	return dariomark.FrontMatterValue{}, false
}
