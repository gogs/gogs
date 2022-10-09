//go:build !pam

// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package pam

import (
	"github.com/pkg/errors"
)

func (*Config) doAuth(_, _ string) error {
	return errors.New("PAM not supported")
}
