package email

import (
	"crypto/tls"
	"net"
	"strconv"
	"strings"

	"github.com/cockroachdb/errors"
	gomail "github.com/wneessen/go-mail"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/conf"
)

type message struct {
	info        string
	msg         *gomail.Msg
	confirmChan chan struct{}
}

func newMessageFrom(to []string, from string, mail *Mail) (*message, error) {
	log.Trace("NewMessageFrom (htmlBody):\n%s", mail.HTML)

	m := gomail.NewMsg()
	if err := m.From(from); err != nil {
		return nil, errors.Wrapf(err, "set From address %q", from)
	}
	if err := m.To(to...); err != nil {
		return nil, errors.Wrap(err, "set To addresses")
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

	return &message{
		msg:         m,
		confirmChan: make(chan struct{}),
	}, nil
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

func sendMessage(msg *message) error {
	client, err := newSMTPClient()
	if err != nil {
		return err
	}
	return client.DialAndSend(msg.msg)
}

func processMailQueue() {
	for msg := range mailQueue {
		to := strings.Join(msg.msg.GetToString(), ", ")
		log.Trace("New e-mail sending request %s: %s", to, msg.info)
		if err := sendMessage(msg); err != nil {
			log.Error("Failed to send emails %s: %s - %v", to, msg.info, err)
		} else {
			log.Trace("E-mails sent %s: %s", to, msg.info)
		}
		msg.confirmChan <- struct{}{}
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
