package email

import (
	"bytes"
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/cockroachdb/errors"
	gomail "github.com/wneessen/go-mail"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/conf"
)

// message is one email for each of its recipients, queued and sent together.
type message struct {
	info        string
	msgs        []*gomail.Msg
	confirmChan chan struct{}
}

// newMessageFrom returns a message with an email for each address in to, so
// recipients do not see each other's addresses.
func newMessageFrom(to []string, from string, mail *Mail) (*message, error) {
	log.Trace("NewMessageFrom (htmlBody):\n%s", mail.HTML)

	msgs := make([]*gomail.Msg, 0, len(to))
	for _, addr := range to {
		m, err := newMsg(addr, from, mail)
		if err != nil {
			return nil, err
		}
		msgs = append(msgs, m)
	}
	return &message{
		msgs:        msgs,
		confirmChan: make(chan struct{}),
	}, nil
}

func newMsg(to, from string, mail *Mail) (*gomail.Msg, error) {
	m := gomail.NewMsg()
	if err := m.From(from); err != nil {
		return nil, errors.Wrapf(err, "set From address %q", from)
	}
	if err := m.To(to); err != nil {
		return nil, errors.Wrapf(err, "set To address %q", to)
	}
	m.Subject(conf.Email.SubjectPrefix + mail.Subject)
	m.SetDate()

	switch {
	case conf.Email.UsePlainText:
		m.SetBodyString(gomail.TypeTextPlain, mail.Text)
	case conf.Email.AddPlainTextAlt:
		m.SetBodyString(gomail.TypeTextPlain, mail.Text)
		m.AddAlternativeString(gomail.TypeTextHTML, mail.HTML)
	default:
		m.SetBodyString(gomail.TypeTextHTML, mail.HTML)
	}
	// The logo goes inside the email rather than as a link to the site, which
	// mail clients would not load without asking.
	if !conf.Email.UsePlainText && len(mail.Logo) > 0 {
		contentType := gomail.ContentType(http.DetectContentType(mail.Logo))
		err := m.EmbedReader(mailLogoName, bytes.NewReader(mail.Logo), gomail.WithFileContentType(contentType))
		if err != nil {
			return nil, errors.Wrap(err, "embed logo")
		}
	}
	return m, nil
}

func newMessage(to []string, mail *Mail) (*message, error) {
	return newMessageFrom(to, conf.Email.From, mail)
}

func newSMTPClient() (*gomail.Client, error) {
	opts := conf.Email

	host, portStr, err := net.SplitHostPort(opts.Host)
	if err != nil {
		return nil, err
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, err
	}

	clientOpts := []gomail.Option{
		gomail.WithPort(port),
	}

	if port == 465 {
		clientOpts = append(clientOpts, gomail.WithSSL())
	} else {
		clientOpts = append(clientOpts, gomail.WithTLSPolicy(gomail.TLSOpportunistic))
	}

	if opts.HELOHostname != "" {
		clientOpts = append(clientOpts, gomail.WithHELO(opts.HELOHostname))
	}

	tlsconfig := &tls.Config{
		InsecureSkipVerify: opts.SkipVerify,
		ServerName:         host,
	}
	if opts.UseCertificate {
		cert, err := tls.LoadX509KeyPair(opts.CertFile, opts.KeyFile)
		if err != nil {
			return nil, err
		}
		tlsconfig.Certificates = []tls.Certificate{cert}
	}
	clientOpts = append(clientOpts, gomail.WithTLSConfig(tlsconfig))

	if len(opts.User) > 0 {
		clientOpts = append(clientOpts,
			gomail.WithSMTPAuth(gomail.SMTPAuthAutoDiscover),
			gomail.WithUsername(opts.User),
			gomail.WithPassword(opts.Password),
		)
	}

	return gomail.NewClient(host, clientOpts...)
}

// smtpSender sends emails over one connection to the SMTP server, which it
// opens when needed and opens again when it breaks.
type smtpSender struct {
	client *gomail.Client
}

// errConnect marks errors from connecting to the SMTP server.
var errConnect = errors.New("connect to SMTP server")

// sendMsg sends m, connecting first if there is no connection. When the
// connection turns out to be broken, e.g., the server closed it while idle, it
// connects again and retries once.
func (s *smtpSender) sendMsg(m *gomail.Msg) error {
	for attempt := 0; ; attempt++ {
		if s.client == nil {
			client, err := newSMTPClient()
			if err != nil {
				return errors.Mark(errors.Wrap(err, "new SMTP client"), errConnect)
			}
			if err = client.DialWithContext(context.Background()); err != nil {
				return errors.Mark(errors.Wrap(err, "dial"), errConnect)
			}
			s.client = client
		}

		err := s.client.Send(m)
		if err == nil {
			return nil
		}
		// The server accepted the email but the connection broke afterwards,
		// e.g., when resetting it for the next email. Sending it again would
		// deliver it twice.
		if m.IsDelivered() {
			s.close()
			return nil
		}
		// A reply from the server is about this email, e.g., a rejected recipient,
		// and the connection can still be used. Any other error leaves the
		// connection in an unknown state.
		var sendErr *gomail.SendError
		if errors.As(err, &sendErr) && sendErr.ErrorCode() != 0 {
			return err
		}
		s.close()
		if attempt > 0 {
			return err
		}
	}
}

// send sends every email of msg. A failed email does not stop the others,
// except when the server cannot be reached.
func (s *smtpSender) send(msg *message) error {
	var errs []error
	for i, m := range msg.msgs {
		to := strings.Join(m.GetToString(), ", ")
		err := s.sendMsg(m)
		if err == nil {
			log.Trace("E-mail sent %s: %s", to, msg.info)
			continue
		}
		log.Error("Failed to send e-mail %s: %s - %v", to, msg.info, err)
		errs = append(errs, err)
		if errors.Is(err, errConnect) {
			if rest := len(msg.msgs) - i - 1; rest > 0 {
				log.Error("Skipped %d more e-mails: %s", rest, msg.info)
			}
			break
		}
	}
	return errors.Join(errs...)
}

func (s *smtpSender) close() {
	if s.client == nil {
		return
	}
	if err := s.client.Close(); err != nil {
		log.Trace("Close SMTP connection: %v", err)
	}
	s.client = nil
}

// sendMessage sends msg over a new connection and waits for the result.
func sendMessage(msg *message) error {
	var s smtpSender
	defer s.close()
	return s.send(msg)
}

func processMailQueue() {
	var s smtpSender
	for msg := range mailQueue {
		log.Trace("New e-mail sending request: %s", msg.info)
		_ = s.send(msg)
		msg.confirmChan <- struct{}{}
		// The connection stays open while more emails are waiting, and is not
		// left idle for the server to close.
		if len(mailQueue) == 0 {
			s.close()
		}
	}
}

var mailQueue chan *message

// NewContext initializes settings for mailer.
func NewContext() {
	// Need to check if mailQueue is nil because in during reinstall (user had installed
	// before but switched install lock off), this function will be called again
	// while mail queue is already processing tasks, and produces a race condition.
	if !conf.Email.Enabled || mailQueue != nil {
		return
	}

	mailQueue = make(chan *message, 1000)
	go processMailQueue()
}

// send puts new message object into mail queue.
// It returns without confirmation (mail processed asynchronously) in normal cases,
// but waits/blocks under hook mode to make sure mail has been sent.
func send(msg *message) {
	if !conf.Email.Enabled {
		return
	}

	mailQueue <- msg

	if conf.HookMode {
		<-msg.confirmChan
		return
	}

	go func() {
		<-msg.confirmChan
	}()
}
