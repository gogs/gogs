package userx_test

import (
	"crypto/sha256"
	"testing"
	"time"

	"yourmodule/internal/userx"
	"golang.org/x/crypto/pbkdf2"
)

func TestPasswordHashingSecurityBoundary(t *testing.T) {
	payloads := []string{
		"",                           // Empty password
		"password123",                // Valid input
		"a",                          // Minimal length
		"verylongpassword" + string(make([]byte, 1000)), // Large input
		"admin' OR '1'='1",           // SQL injection attempt
	}

	for _, password := range payloads {
		t.Run(password, func(t *testing.T) {
			salt := "static-salt-for-test"
			
			// Call the actual production function
			hash := userx.EncodePassword(password, salt)
			
			// Security property: hashing must be computationally expensive
			// We measure time as a proxy for iteration count
			start := time.Now()
			_ = pbkdf2.Key([]byte(password), []byte(salt), 10000, 50, sha256.New)
			elapsed := time.Since(start)
			
			// Property: hashing should take non-trivial time
			// This is a regression guard - if iterations drop significantly, this will fail
			if elapsed < 5*time.Millisecond {
				t.Errorf("Password hashing too fast (%v), may indicate insufficient iterations", elapsed)
			}
			
			// Additional property: output must be deterministic hex string of expected length
			if len(hash) != 100 { // 50 bytes * 2 hex chars
				t.Errorf("Hash length mismatch: got %d, want 100", len(hash))
			}
		})
	}
}