package email

import (
	"crypto/tls"
	"io"
	"net"
	"net/smtp"
	"os"
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

func newMessageFrom(to []string, from, subject, htmlBody string) *message {
	log.Trace("NewMessageFrom (htmlBody):\n%s", htmlBody)

	m := gomail.NewMsg()
	if err := m.From(from); err != nil {
		log.Error("Failed to set From address %q: %v", from, err)
	}
	if err := m.To(to...); err != nil {
		log.Error("Failed to set To addresses: %v", err)
	}
	m.Subject(conf.Email.SubjectPrefix + subject)
	m.SetDate()

	if conf.Email.UsePlainText || conf.Email.AddPlainTextAlt {
		plainBody, err := html2text.FromString(htmlBody)
		if err != nil {
			log.Error("html2text.FromString: %v", err)
			m.SetBodyString(gomail.TypeTextHTML, htmlBody)
		} else if conf.Email.UsePlainText {
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
	}
}

func newMessage(to []string, subject, body string) *message {
	return newMessageFrom(to, conf.Email.From, subject, body)
}

type loginAuth struct {
	username, password string
}

func newLoginAuth(username, password string) smtp.Auth {
	return &loginAuth{username, password}
}

func (*loginAuth) Start(_ *smtp.ServerInfo) (string, []byte, error) {
	return "LOGIN", []byte{}, nil
}

func (a *loginAuth) Next(fromServer []byte, more bool) ([]byte, error) {
	if more {
		switch string(fromServer) {
		case "Username:":
			return []byte(a.username), nil
		case "Password:":
			return []byte(a.password), nil
		default:
			return nil, errors.Newf("unknwon fromServer: %s", string(fromServer))
		}
	}
	return nil, nil
}

type smtpSender struct{}

func (*smtpSender) Send(from string, to []string, msg io.WriterTo) error {
	opts := conf.Email

	host, port, err := net.SplitHostPort(opts.Host)
	if err != nil {
		return err
	}

	tlsconfig := &tls.Config{
		InsecureSkipVerify: opts.SkipVerify,
		ServerName:         host,
	}

	if opts.UseCertificate {
		cert, err := tls.LoadX509KeyPair(opts.CertFile, opts.KeyFile)
		if err != nil {
			return err
		}
		tlsconfig.Certificates = []tls.Certificate{cert}
	}

	conn, err := net.Dial("tcp", net.JoinHostPort(host, port))
	if err != nil {
		return err
	}
	defer conn.Close()

	isSecureConn := false
	// Start TLS directly if the port ends with 465 (SMTPS protocol)
	if strings.HasSuffix(port, "465") {
		conn = tls.Client(conn, tlsconfig)
		isSecureConn = true
	}

	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return errors.Newf("NewClient: %v", err)
	}

	if !opts.DisableHELO {
		hostname := opts.HELOHostname
		if hostname == "" {
			hostname, err = os.Hostname()
			if err != nil {
				return err
			}
		}

		if err = client.Hello(hostname); err != nil {
			return errors.Newf("hello: %v", err)
		}
	}

	// If not using SMTPS, always use STARTTLS if available
	hasStartTLS, _ := client.Extension("STARTTLS")
	if !isSecureConn && hasStartTLS {
		if err = client.StartTLS(tlsconfig); err != nil {
			return errors.Newf("StartTLS: %v", err)
		}
	}

	canAuth, options := client.Extension("AUTH")
	if canAuth && len(opts.User) > 0 {
		var auth smtp.Auth

		if strings.Contains(options, "CRAM-MD5") {
			auth = smtp.CRAMMD5Auth(opts.User, opts.Password)
		} else if strings.Contains(options, "PLAIN") {
			auth = smtp.PlainAuth("", opts.User, opts.Password, host)
		} else if strings.Contains(options, "LOGIN") {
			auth = newLoginAuth(opts.User, opts.Password)
		}

		if auth != nil {
			if err = client.Auth(auth); err != nil {
				return errors.Newf("auth: %v", err)
			}
		}
	}

	if err = client.Mail(from); err != nil {
		return errors.Newf("mail: %v", err)
	}

	for _, rec := range to {
		if err = client.Rcpt(rec); err != nil {
			return errors.Newf("rcpt: %v", err)
		}
	}

	w, err := client.Data()
	if err != nil {
		return errors.Newf("data: %v", err)
	} else if _, err = msg.WriteTo(w); err != nil {
		return errors.Newf("write to: %v", err)
	} else if err = w.Close(); err != nil {
		return errors.Newf("close: %v", err)
	}

	return client.Quit()
}

func sendMessage(sender *smtpSender, msg *message) error {
	from, err := msg.msg.GetSender(false)
	if err != nil {
		return errors.Wrap(err, "get sender")
	}

	recipients, err := msg.msg.GetRecipients()
	if err != nil {
		return errors.Wrap(err, "get recipients")
	}

	return sender.Send(from, recipients, msg.msg)
}

func processMailQueue() {
	sender := &smtpSender{}
	for msg := range mailQueue {
		to := strings.Join(msg.msg.GetToString(), ", ")
		log.Trace("New e-mail sending request %s: %s", to, msg.info)
		if err := sendMessage(sender, msg); err != nil {
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

// Send puts new message object into mail queue.
// It returns without confirmation (mail processed asynchronously) in normal cases,
// but waits/blocks under hook mode to make sure mail has been sent.
func Send(msg *message) {
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
