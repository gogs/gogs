package email

import (
	"crypto/tls"
	"net"
	"strconv"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/inbucket/html2text"
	gomail "github.com/wneessen/go-mail"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/conf"
)

type message struct {
	info        string
	msg         *gomail.Msg
	confirmChan chan struct{}
}

func newMessageFrom(to []string, from, subject, htmlBody string) (*message, error) {
	log.Trace("NewMessageFrom (htmlBody):\n%s", htmlBody)

	m := gomail.NewMsg()
	if err := m.From(from); err != nil {
		return nil, errors.Wrapf(err, "set From address %q", from)
	}
	if err := m.To(to...); err != nil {
		return nil, errors.Wrap(err, "set To addresses")
	}
	m.Subject(conf.Email.SubjectPrefix + subject)
	m.SetDate()

	if conf.Email.UsePlainText || conf.Email.AddPlainTextAlt {
		plainBody, err := html2text.FromString(htmlBody)
		if err != nil {
			return nil, errors.Wrap(err, "convert HTML to plain text")
		}
		if conf.Email.UsePlainText {
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

func newMessage(to []string, subject, body string) (*message, error) {
	return newMessageFrom(to, conf.Email.From, subject, body)
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
