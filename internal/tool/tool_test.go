package tool

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestVerifyTimeLimitCode(t *testing.T) {
	const format = "200601021504"
	issued := time.Now().Add(-30 * time.Minute).Format(format)
	code := CreateTimeLimitCode("data", 60, issued)

	assert.True(t, VerifyTimeLimitCode("data", 60, code))
	assert.False(t, VerifyTimeLimitCode("other", 60, code))

	t.Run("expires by the issued lifetime", func(t *testing.T) {
		assert.False(t, VerifyTimeLimitCode("data", 60, CreateTimeLimitCode("data", 20, issued)))
	})

	t.Run("expires by a lowered configured lifetime", func(t *testing.T) {
		assert.False(t, VerifyTimeLimitCode("data", 20, code))
	})

	t.Run("raised configured lifetime does not extend", func(t *testing.T) {
		assert.False(t, VerifyTimeLimitCode("data", 600, CreateTimeLimitCode("data", 20, issued)))
	})
}
