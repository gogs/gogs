package dbx

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestQuote(t *testing.T) {
	got := Quote("postgres", "SELECT * FROM %s", "user")
	want := `SELECT * FROM "user"`
	assert.Equal(t, want, got)

	got = Quote("mysql", "SELECT * FROM %s", "user")
	want = `SELECT * FROM user`
	assert.Equal(t, want, got)
}
