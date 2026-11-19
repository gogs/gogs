//go:build !(((linux || darwin) && (amd64 || arm64)) || (windows && amd64))

package sqlite

import (
	"database/sql/driver"

	"github.com/cockroachdb/errors"
	"github.com/glebarez/go-sqlite"
)

// Driver is the database/sql driver, registered as "sqlite" by the engine.
var Driver driver.Driver = &sqlite.Driver{}

// extendedCode returns the extended result code of the SQLite error, or 0
// when the error does not come from SQLite.
func extendedCode(err error) int {
	var sqliteErr *sqlite.Error
	if errors.As(err, &sqliteErr) {
		// The engine returns extended result codes.
		return sqliteErr.Code()
	}
	return 0
}
