//go:build !pam

package pam

import (
	"github.com/cockroachdb/errors"
)

func (*Config) doAuth(_, _ string) error {
	return errors.New("PAM not supported")
}
