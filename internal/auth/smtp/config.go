// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package smtp

import (
	"crypto/tls"
	"fmt"
	"net/smtp"

	"github.com/pkg/errors"
)

// Config contains configuration for SMTP authentication.
//
// ⚠️ WARNING: Change to the field name must preserve the INI key name for backward compatibility.
type Config struct {
	Auth           string
	Host           string
	Port           int
	AllowedDomains string
	TLS            bool `ini:"tls"`
	SkipVerify     bool
}

func (c *Config) doAuth(auth smtp.Auth) error {
	client, err := smtp.Dial(fmt.Sprintf("%s:%d", c.Host, c.Port))
	if err != nil {
		return err
	}
	defer client.Close()

	if err = client.Hello("gogs"); err != nil {
		return err
	}

	if c.TLS {
		if ok, _ := client.Extension("STARTTLS"); ok {
			if err = client.StartTLS(&tls.Config{
				InsecureSkipVerify: c.SkipVerify,
				ServerName:         c.Host,
			}); err != nil {
				return err
			}
		} else {
			return errors.New("SMTP server does not support TLS")
		}
	}

	if ok, _ := client.Extension("AUTH"); ok {
		if err = client.Auth(auth); err != nil {
			return err
		}
		return nil
	}
	return errors.New("unsupported SMTP authentication method")
}
