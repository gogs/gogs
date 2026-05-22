package smtp

import (
	"crypto/tls"
	"net"
	"net/smtp"
	"strconv"
	"time"

	"github.com/cockroachdb/errors"
)

// dialTimeout bounds how long the SMTP authentication flow waits on the
// underlying TCP connect. Without it, an unreachable or misspelled host hangs
// the sign-in request until the OS-level connect timeout (minutes).
const dialTimeout = 10 * time.Second

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
	addr := net.JoinHostPort(c.Host, strconv.Itoa(c.Port))
	conn, err := net.DialTimeout("tcp", addr, dialTimeout)
	if err != nil {
		return err
	}
	client, err := smtp.NewClient(conn, c.Host)
	if err != nil {
		_ = conn.Close()
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
