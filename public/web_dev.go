//go:build !prod

package public

import "embed"

// WebAssets is empty in dev builds because requests are proxied to the live
// Vite server. It is declared so that prod and dev builds share the same symbol.
var WebAssets embed.FS
