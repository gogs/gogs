package testx

import (
	"os"
	"strings"
)

// InTest is true if the current binary looks like a test artifact.
var InTest = len(os.Args) > 0 && strings.HasSuffix(strings.TrimSuffix(os.Args[0], ".exe"), ".test")
