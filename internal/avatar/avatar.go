package avatar

import (
	"sync"

	"github.com/cockroachdb/errors"
	dicebear "github.com/dicebear/dicebear-go/v10"
	"github.com/dicebear/styles/v10"
)

// DefaultSize is the height and width that uploaded avatars are scaled to and
// generated avatars are drawn at.
const DefaultSize = 290

// Kind is what an avatar stands for, which decides the style it is drawn in.
type Kind int

const (
	// Person avatars are drawn in the DiceBear Notionists style.
	Person Kind = iota
	// Organization avatars are drawn in the DiceBear Shapes style, so they do
	// not look like people.
	Organization
)

// backgroundColors are the pastels behind the black ink of the Notionists
// drawings, one of which is picked by the seed.
var backgroundColors = []string{"b6e3f4", "c0aede", "d1d4f9", "ffd5dc", "ffdfbf"}

// The styles are parsed once, because parsing and validating their definitions
// takes far longer than drawing an avatar.
var (
	notionistsStyle = sync.OnceValues(func() (*dicebear.Style, error) {
		return dicebear.NewStyle([]byte(styles.Notionists))
	})
	shapesStyle = sync.OnceValues(func() (*dicebear.Style, error) {
		return dicebear.NewStyle([]byte(styles.Shapes))
	})
)

// Generate returns the SVG of the avatar of the kind drawn from the given seed.
// The same kind and seed always draw the same avatar.
func Generate(kind Kind, seed string) ([]byte, error) {
	parse, options := notionistsStyle, map[string]any{"backgroundColor": backgroundColors}
	if kind == Organization {
		parse, options = shapesStyle, map[string]any{}
	}
	style, err := parse()
	if err != nil {
		return nil, errors.Wrap(err, "parse style")
	}
	options["seed"] = seed
	a, err := dicebear.NewAvatar(style, options)
	if err != nil {
		return nil, errors.Wrap(err, "draw avatar")
	}
	return []byte(a.SVG()), nil
}
