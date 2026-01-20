package convert

import (
	"gogs.io/gogs/internal/conf"
)

// ToCorrectPageSize makes sure page size is in allowed range.
func ToCorrectPageSize(size int) int {
	if size <= 0 {
		size = 10
	} else if size > conf.API.MaxResponseItems {
		size = conf.API.MaxResponseItems
	}
	return size
}
