package public

import (
	"embed"
)

//go:embed assets/* css/* img/* js/* plugins/*
var Files embed.FS
