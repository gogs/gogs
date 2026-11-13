package public

import (
	"embed"
)

//go:embed assets/* img/* plugins/*
var Files embed.FS
