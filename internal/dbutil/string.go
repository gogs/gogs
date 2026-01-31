package dbutil

import (
	"fmt"

	"gogs.io/gogs/internal/conf"
)

// Quote adds surrounding double quotes for all given arguments before being
// formatted if the current database is UsePostgreSQL.
func Quote(format string, args ...string) string {
	anys := make([]any, len(args))
	for i := range args {
		if conf.UsePostgreSQL {
			anys[i] = `"` + args[i] + `"`
		} else {
			anys[i] = args[i]
		}
	}
	return fmt.Sprintf(format, anys...)
}
