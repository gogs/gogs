package database

import (
	"testing"

	"gorm.io/gorm"
)

// SetMockHandle sets the global database Handle to a DB backed by the given
// gorm.DB instance for the duration of the test, restoring the original value
// when the test completes.
func SetMockHandle(t *testing.T, db *gorm.DB) {
	before := Handle
	Handle = &DB{db: db}
	t.Cleanup(func() {
		Handle = before
	})
}
