package v1

import (
	"gogs.io/gogs/internal/conf"
)

// toCorrectPageSize makes sure page size is in allowed range.
func toCorrectPageSize(size int) int {
	if size <= 0 {
		size = 10
	} else if size > conf.API.MaxResponseItems {
		size = conf.API.MaxResponseItems
	}
	return size
}
