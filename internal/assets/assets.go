package assets

import (
	"strings"
)

// IsErrNotFound returns true if the error is asset not found.
func IsErrNotFound(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "not found")
}
