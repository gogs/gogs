package email

import (
	"crypto/tls"
	"net"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/cockroachdb/errors"
	"github.com/inbucket/html2text"
	gomail "github.com/wneessen/go-mail"
	"unknwon.dev/x/logx"

	"gogs.io/gogs/internal/conf"
)

type message struct {
	info        string
	msg         *gomail.Msg
	confirmChan chan struct{}
}

func newMessageFrom(logger *logx.Logger, opts conf.EmailOpts, to []string, from, subject, htmlBody string) (*message, error) {
	logger.Debug("Creating email message", "subject", subject, "htmlBody", htmlBody)

	m := gomail.NewMsg()
	if err := m.From(from); err != nil {
		return nil, errors.Wrapf(err, "set From address %q", from)
	}
	if err := m.To(to...); err != nil {
		return nil, errors.Wrap(err, "set To addresses")
	}
	m.Subject(opts.SubjectPrefix + subject)
	m.SetDate()

	if opts.UsePlainText || opts.AddPlainTextAlt {
		plainBody, err := html2text.FromString(htmlBody)
		if err != nil {
			return nil, errors.Wrap(err, "convert HTML to plain text")
		}
		if opts.UsePlainText {
			m.SetBodyString(gomail.TypeTextPlain, plainBody)
		} else {
			m.SetBodyString(gomail.TypeTextPlain, plainBody)
			m.AddAlternativeString(gomail.TypeTextHTML, htmlBody)
		}
	} else {
		m.SetBodyString(gomail.TypeTextHTML, htmlBody)
	}

	return &message{
		msg:         m,
		confirmChan: make(chan struct{}),
	}, nil
}

func newMessage(logger *logx.Logger, opts conf.EmailOpts, to []string, subject, body string) (*message, error) {
	return newMessageFrom(logger, opts, to, opts.From, subject, body)
}

func newSMTPClient(opts conf.EmailOpts) (*gomail.Client, error) {
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

func sendMessage(opts conf.EmailOpts, msg *message) error {
	client, err := newSMTPClient(opts)
	if err != nil {
		return err
	}
	return client.DialAndSend(msg.msg)
}

func processMailQueue(logger *logx.Logger, opts conf.EmailOpts) {
	for msg := range mailQueue {
		queued.Add(-1)
		to := strings.Join(msg.msg.GetToString(), ", ")
		logger.Debug("Sending email", "to", to, "description", msg.info)
		if err := sendMessage(opts, msg); err != nil {
			logger.Error("Failed to send email", "to", to, "description", msg.info, "error", err)
		} else {
			logger.Debug("Sent email", "to", to, "description", msg.info)
		}
		msg.confirmChan <- struct{}{}
	}
}

var mailQueue chan *message

// queued counts the messages in the mail queue, since the queue itself is
// replaced while it is read during a reinstall.
var queued atomic.Int64

// QueueLen returns how many messages wait to be sent.
func QueueLen() int64 {
	return queued.Load()
}

// NewContext starts sending the queued mail with the settings when sending
// mail is enabled.
func NewContext(logger *logx.Logger, opts conf.EmailOpts) {
	// Need to check if mailQueue is nil because during reinstall (user had installed
	// before but switched install lock off), this function is called again while
	// the mail queue is already processing tasks, which produces a race condition.
	if !opts.Enabled || mailQueue != nil {
		return
	}

	mailQueue = make(chan *message, 1000)
	go processMailQueue(logger, opts)
}

// send puts new message object into mail queue.
// It returns without confirmation (mail processed asynchronously) in normal cases,
// but blocks in hook mode to make sure mail has been sent.
func send(opts Options, msg *message) {
	if !opts.Email.Enabled {
		return
	}

	queued.Add(1)
	mailQueue <- msg

	if opts.HookMode {
		<-msg.confirmChan
		return
	}

	go func() {
		<-msg.confirmChan
	}()
}
