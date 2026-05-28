package ptrx

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDeref(t *testing.T) {
	t.Run("nil pointer returns default", func(t *testing.T) {
		assert.Equal(t, 42, Deref(nil, 42))
		assert.Equal(t, "", Deref(nil, ""))
		assert.Equal(t, "fallback", Deref(nil, "fallback"))
		assert.Equal(t, false, Deref(nil, false))
	})

	t.Run("non-nil pointer returns dereferenced value", func(t *testing.T) {
		intVal := 7
		assert.Equal(t, 7, Deref(&intVal, 0))

		strVal := "hello"
		assert.Equal(t, "hello", Deref(&strVal, "default"))

		boolVal := true
		assert.Equal(t, true, Deref(&boolVal, false))
	})

	t.Run("zero value pointer returns zero value", func(t *testing.T) {
		zeroInt := 0
		assert.Equal(t, 0, Deref(&zeroInt, 99))

		emptyStr := ""
		assert.Equal(t, "", Deref(&emptyStr, "fallback"))
	})
}
