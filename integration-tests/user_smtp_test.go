package integrationtests

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"math/big"
	"net"
	"net/http"
	"net/textproto"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gogs.io/gogs/internal/auth"
	"gogs.io/gogs/internal/auth/smtp"
	"gogs.io/gogs/internal/database"
)

func TestUserSignInWithSMTP(t *testing.T) {
	t.Parallel()

	cert := selfSignedCertificate(t)
	password := UniqueName("password-")
	// assertSignIn signs in through the source with a wrong password, then with
	// the right one, which creates the user of the source.
	assertSignIn := func(t *testing.T, source *database.LoginSource) {
		name := UniqueName("smtp-")
		login := name + "@example.com"
		_, status, body := signInWithSource(t, source, login, password+"x")
		assert.Equal(t, http.StatusUnauthorized, status, "response: %s", body)
		assertNoAccount(t, name)

		c, status, body := signInWithSource(t, source, login, password)
		require.Equal(t, http.StatusOK, status, "response: %s", body)
		assert.Equal(t, name, c.username())
		u, err := database.Handle.Users().GetByUsername(t.Context(), name)
		require.NoError(t, err)
		assert.Equal(t, source.ID, u.LoginSource)
		assert.Equal(t, login, u.Email)
	}

	// Port 465 expects TLS from the start of the connection, i.e., implicit TLS.
	t.Run("implicit TLS", func(t *testing.T) {
		l, err := net.Listen("tcp", "127.0.0.1:465")
		if err != nil {
			msg := "Listen on port 465, e.g., after `sudo sysctl -w net.ipv4.ip_unprivileged_port_start=465`: " + err.Error()
			if os.Getenv("CI") != "" {
				t.Fatal(msg)
			}
			t.Skip(msg)
		}
		serveSMTP(t, tls.NewListener(l, &tls.Config{Certificates: []tls.Certificate{cert}}), true, cert, password)

		for _, authType := range smtp.AuthTypes {
			t.Run(authType, func(t *testing.T) {
				assertSignIn(t, seedSMTPSource(t, authType, 465, true))
			})
		}

		t.Run("untrusted certificate", func(t *testing.T) {
			source := seedSMTPSource(t, smtp.Plain, 465, false)
			name := UniqueName("smtp-")
			_, status, body := signInWithSource(t, source, name+"@example.com", password)
			assert.Equal(t, http.StatusInternalServerError, status, "response: %s", body)
			// Any failure to talk to the server is a 500, e.g., a plaintext
			// connection that gets no greeting.
			assert.Contains(t, string(body), "x509:")
			assertNoAccount(t, name)
		})
	})

	t.Run("STARTTLS", func(t *testing.T) {
		l, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		serveSMTP(t, l, false, cert, password)
		assertSignIn(t, seedSMTPSource(t, smtp.Login, l.Addr().(*net.TCPAddr).Port, true))
	})
}

// seedSMTPSource creates an active SMTP authentication source for the server
// on the port of 127.0.0.1, with TLS enabled, which trusts any certificate of
// the server when skipVerify is true.
func seedSMTPSource(t *testing.T, authType string, port int, skipVerify bool) *database.LoginSource {
	t.Helper()

	source, err := database.Handle.LoginSources().Create(t.Context(), database.CreateLoginSourceOptions{
		Type:      auth.SMTP,
		Name:      UniqueName("smtp-"),
		Activated: true,
		Config: &smtp.Config{
			Auth:       authType,
			Host:       "127.0.0.1",
			Port:       port,
			TLS:        true,
			SkipVerify: skipVerify,
		},
	})
	require.NoError(t, err)
	// Deleting fails once a user signed in through the source, which then stays
	// with the user.
	t.Cleanup(func() { _ = database.Handle.LoginSources().DeleteByID(context.Background(), source.ID) })
	return source
}

// selfSignedCertificate returns a certificate for 127.0.0.1 that no one
// trusts.
func selfSignedCertificate(t *testing.T) tls.Certificate {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "127.0.0.1"},
		IPAddresses:  []net.IP{net.IPv4(127, 0, 0, 1)},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}

// serveSMTP serves SMTP on the listener until the test ends, and accepts any
// login with the password through AUTH PLAIN or AUTH LOGIN. Like most servers,
// it only offers AUTH over TLS, i.e., from the start when the listener is a TLS
// one, or after STARTTLS.
func serveSMTP(t *testing.T, l net.Listener, isTLS bool, cert tls.Certificate, password string) {
	t.Helper()

	t.Cleanup(func() { _ = l.Close() })
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			go serveSMTPConn(conn, isTLS, cert, password)
		}
	}()
}

func serveSMTPConn(conn net.Conn, isTLS bool, cert tls.Certificate, password string) {
	defer func() { _ = conn.Close() }()
	// A client that waits for the greeting in plaintext while the server waits
	// for a TLS handshake gets the connection closed, as by real servers.
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))

	tc := textproto.NewConn(conn)
	if tc.PrintfLine("220 localhost ESMTP") != nil {
		return
	}
	// readResponse sends the prompt of an AUTH mechanism, and reads and decodes
	// the line the client answers with.
	readResponse := func(prompt string) (string, bool) {
		if tc.PrintfLine("334 %s", base64.StdEncoding.EncodeToString([]byte(prompt))) != nil {
			return "", false
		}
		line, err := tc.ReadLine()
		if err != nil {
			return "", false
		}
		b, err := base64.StdEncoding.DecodeString(line)
		return string(b), err == nil
	}
	for {
		line, err := tc.ReadLine()
		if err != nil {
			return
		}
		verb, arg, _ := strings.Cut(line, " ")
		switch strings.ToUpper(verb) {
		case "EHLO", "HELO":
			ext := "250 STARTTLS"
			if isTLS {
				ext = "250 AUTH PLAIN LOGIN"
			}
			_ = tc.PrintfLine("250-localhost")
			_ = tc.PrintfLine("%s", ext)
		case "STARTTLS":
			if isTLS {
				_ = tc.PrintfLine("503 Already running TLS")
				continue
			}
			_ = tc.PrintfLine("220 Ready to start TLS")
			conn = tls.Server(conn, &tls.Config{Certificates: []tls.Certificate{cert}})
			tc = textproto.NewConn(conn)
			isTLS = true
		case "AUTH":
			if !isTLS {
				_ = tc.PrintfLine("538 Encryption required")
				continue
			}
			mechanism, initial, _ := strings.Cut(arg, " ")
			var gotPassword string
			ok := true
			switch strings.ToUpper(mechanism) {
			case "PLAIN":
				resp := initial
				if resp == "" {
					resp, ok = readResponse("")
				} else {
					b, err := base64.StdEncoding.DecodeString(resp)
					resp, ok = string(b), err == nil
				}
				fields := strings.Split(resp, "\x00")
				if ok && len(fields) == 3 {
					gotPassword = fields[2]
				}
			case "LOGIN":
				if initial == "" {
					_, ok = readResponse("Username:")
				}
				if ok {
					gotPassword, ok = readResponse("Password:")
				}
			default:
				_ = tc.PrintfLine("504 Unrecognized authentication type")
				continue
			}
			if ok && gotPassword == password {
				_ = tc.PrintfLine("235 Authentication successful")
			} else {
				_ = tc.PrintfLine("535 Authentication credentials invalid")
			}
		case "QUIT":
			_ = tc.PrintfLine("221 Bye")
			return
		default:
			_ = tc.PrintfLine("502 Command not implemented")
		}
	}
}
