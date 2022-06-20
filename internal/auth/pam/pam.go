//go:build pam
// +build pam

// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package pam

import (
	"github.com/msteinert/pam"
	"github.com/pkg/errors"
)

func (c *Config) doAuth(login, password string) error {
	t, err := pam.StartFunc(c.ServiceName, login, func(s pam.Style, msg string) (string, error) {
		switch s {
		case pam.PromptEchoOff:
			return password, nil
		case pam.PromptEchoOn, pam.ErrorMsg, pam.TextInfo:
			return "", nil
		}
		return "", errors.Errorf("unrecognized PAM message style: %v - %s", s, msg)
	})
	if err != nil {
		return err
	}

	err = t.Authenticate(0)
	if err != nil {
		return err
	}
	return t.AcctMgmt(0)
}
