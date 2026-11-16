package email

import (
	"bufio"
	"net"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gogs.io/gogs/internal/conf"
)

// fakeSMTPServer accepts emails without authentication and rejects recipients
// whose address starts with "reject". With dropAfter set to "DATA" or "RSET",
// it closes each connection after replying to that command for the first email.
type fakeSMTPServer struct {
	listener  net.Listener
	dropAfter string

	mu          sync.Mutex
	connections int
	// delivered has the To header of every delivered email.
	delivered []string
}

func newFakeSMTPServer(t *testing.T, dropAfter string) *fakeSMTPServer {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	s := &fakeSMTPServer{listener: l, dropAfter: dropAfter}
	t.Cleanup(func() { _ = l.Close() })
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			s.mu.Lock()
			s.connections++
			s.mu.Unlock()
			go s.serve(conn)
		}
	}()
	return s
}

func (s *fakeSMTPServer) serve(conn net.Conn) {
	defer func() { _ = conn.Close() }()
	r := bufio.NewReader(conn)
	reply := func(line string) { _, _ = conn.Write([]byte(line + "\r\n")) }
	reply("220 fake ESMTP")
	delivered := 0
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		cmd := strings.ToUpper(strings.TrimSpace(line))
		switch {
		case strings.HasPrefix(cmd, "EHLO"), strings.HasPrefix(cmd, "HELO"):
			reply("250-fake")
			reply("250 8BITMIME")
		case strings.HasPrefix(cmd, "RCPT TO:<REJECT"):
			reply("550 5.1.1 no such user")
		case cmd == "DATA":
			reply("354 go ahead")
			var to string
			for {
				l, err := r.ReadString('\n')
				if err != nil {
					return
				}
				l = strings.TrimRight(l, "\r\n")
				if l == "." {
					break
				}
				if strings.HasPrefix(l, "To: ") && to == "" {
					to = strings.TrimPrefix(l, "To: ")
				}
			}
			s.mu.Lock()
			s.delivered = append(s.delivered, to)
			s.mu.Unlock()
			reply("250 queued")
			delivered++
			if s.dropAfter == "DATA" && delivered == 1 {
				return
			}
		case cmd == "RSET":
			reply("250 ok")
			if s.dropAfter == "RSET" && delivered == 1 {
				return
			}
		case cmd == "QUIT":
			reply("221 bye")
			return
		default:
			reply("250 ok")
		}
	}
}

func (s *fakeSMTPServer) result() (connections int, delivered []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.connections, append([]string(nil), s.delivered...)
}

func useFakeSMTPServer(t *testing.T, s *fakeSMTPServer) {
	t.Helper()
	before := conf.Email
	t.Cleanup(func() { conf.Email = before })
	conf.Email.Host = s.listener.Addr().String()
	conf.Email.User = ""
	conf.Email.From = "Gogs <gogs@example.test>"
}

func TestSendMessage(t *testing.T) {
	setupTemplates(t)
	mail := &Mail{Subject: "Issue", HTML: "<p>Hello</p>", Text: "Hello"}

	t.Run("one email per recipient over one connection", func(t *testing.T) {
		s := newFakeSMTPServer(t, "")
		useFakeSMTPServer(t, s)

		msg, err := newMessage([]string{"alice@example.test", "bob@example.test", "carol@example.test"}, mail)
		require.NoError(t, err)
		require.NoError(t, sendMessage(msg))

		connections, delivered := s.result()
		assert.Equal(t, 1, connections)
		assert.Equal(t, []string{"<alice@example.test>", "<bob@example.test>", "<carol@example.test>"}, delivered)
	})

	t.Run("reconnects when the server closes the connection", func(t *testing.T) {
		// Like an idle timeout: the next email finds the connection closed and
		// is sent again over a new one.
		s := newFakeSMTPServer(t, "RSET")
		useFakeSMTPServer(t, s)

		msg, err := newMessage([]string{"alice@example.test", "bob@example.test", "carol@example.test"}, mail)
		require.NoError(t, err)
		require.NoError(t, sendMessage(msg))

		connections, delivered := s.result()
		assert.Equal(t, 3, connections)
		assert.Equal(t, []string{"<alice@example.test>", "<bob@example.test>", "<carol@example.test>"}, delivered)
	})

	t.Run("does not send again an email the server accepted", func(t *testing.T) {
		s := newFakeSMTPServer(t, "DATA")
		useFakeSMTPServer(t, s)

		msg, err := newMessage([]string{"alice@example.test", "bob@example.test"}, mail)
		require.NoError(t, err)
		require.NoError(t, sendMessage(msg))

		connections, delivered := s.result()
		assert.Equal(t, 2, connections)
		assert.Equal(t, []string{"<alice@example.test>", "<bob@example.test>"}, delivered)
	})

	t.Run("a rejected recipient does not stop the others", func(t *testing.T) {
		s := newFakeSMTPServer(t, "")
		useFakeSMTPServer(t, s)

		msg, err := newMessage([]string{"alice@example.test", "reject@example.test", "carol@example.test"}, mail)
		require.NoError(t, err)
		err = sendMessage(msg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "reject@example.test")

		connections, delivered := s.result()
		assert.Equal(t, 1, connections)
		assert.Equal(t, []string{"<alice@example.test>", "<carol@example.test>"}, delivered)
	})
}
