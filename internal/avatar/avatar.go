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

// backgroundColors are the pastels behind the black ink of the drawings, one of
// which is picked by the seed.
var backgroundColors = []string{"b6e3f4", "c0aede", "d1d4f9", "ffd5dc", "ffdfbf"}

// notionistsStyle parses the style once, because parsing and validating its
// definition takes far longer than drawing an avatar.
var notionistsStyle = sync.OnceValues(func() (*dicebear.Style, error) {
	return dicebear.NewStyle([]byte(styles.Notionists))
})

// Generate returns the SVG of the avatar drawn from the given seed in the
// DiceBear Notionists style. The same seed always draws the same avatar.
func Generate(seed string) ([]byte, error) {
	style, err := notionistsStyle()
	if err != nil {
		return nil, errors.Wrap(err, "parse style")
	}
	a, err := dicebear.NewAvatar(style, map[string]any{
		"seed":            seed,
		"backgroundColor": backgroundColors,
	})
	if err != nil {
		return nil, errors.Wrap(err, "draw avatar")
	}
	return []byte(a.SVG()), nil
}
