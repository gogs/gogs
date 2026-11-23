package dbx

import (
	"fmt"
)

// Quote adds surrounding double quotes for all given arguments before being
// formatted if the dialect is PostgreSQL, e.g., the gorm.DB.Name.
func Quote(dialect, format string, args ...string) string {
	anys := make([]any, len(args))
	for i := range args {
		if dialect == "postgres" {
			anys[i] = `"` + args[i] + `"`
		} else {
			anys[i] = args[i]
		}
	}
	return fmt.Sprintf(format, anys...)
}
