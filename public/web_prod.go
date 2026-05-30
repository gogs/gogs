//go:build prod

package public

import "embed"

//go:embed all:dist
var WebAssets embed.FS
