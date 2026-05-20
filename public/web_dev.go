//go:build !prod

package public

import "embed"

// WebAssets is empty in dev — requests are proxied to the live Vite server.
// Declared so the prod and dev builds share the same symbol.
var WebAssets embed.FS
