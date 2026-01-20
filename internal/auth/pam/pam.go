//go:build pam

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
