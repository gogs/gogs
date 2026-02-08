package email

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net"
	"net/mail"
	"net/smtp"
	"net/textproto"
	"os"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/inbucket/html2text"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/conf"
)

type Message struct {
	Info        string
	header      map[string][]string
	contentType string
	body        string
	altParts    []altPart
	confirmChan chan struct{}
}

type altPart struct {
	contentType string
	body        string
}

func (m *Message) GetHeader(field string) []string {
	return m.header[field]
}

func (m *Message) WriteTo(w io.Writer) (int64, error) {
	var buf bytes.Buffer

	for _, field := range []string{"From", "To", "Subject", "Date"} {
		vals := m.header[field]
		for _, v := range vals {
			encoded := v
			if field == "Subject" {
				encoded = mime.QEncoding.Encode("utf-8", v)
			}
			fmt.Fprintf(&buf, "%s: %s\r\n", field, encoded)
		}
	}

	if len(m.altParts) > 0 {
		mw := multipart.NewWriter(&buf)
		fmt.Fprintf(&buf, "MIME-Version: 1.0\r\n")
		fmt.Fprintf(&buf, "Content-Type: multipart/alternative; boundary=%s\r\n\r\n", mw.Boundary())

		mainHeader := textproto.MIMEHeader{}
		mainHeader.Set("Content-Type", m.contentType+"; charset=UTF-8")
		mainHeader.Set("Content-Transfer-Encoding", "quoted-printable")
		part, err := mw.CreatePart(mainHeader)
		if err != nil {
			return 0, errors.Wrap(err, "create main part")
		}
		qpWrite(part, m.body)

		for _, alt := range m.altParts {
			altHeader := textproto.MIMEHeader{}
			altHeader.Set("Content-Type", alt.contentType+"; charset=UTF-8")
			altHeader.Set("Content-Transfer-Encoding", "quoted-printable")
			part, err := mw.CreatePart(altHeader)
			if err != nil {
				return 0, errors.Wrap(err, "create alternative part")
			}
			qpWrite(part, alt.body)
		}
		mw.Close()
	} else {
		fmt.Fprintf(&buf, "MIME-Version: 1.0\r\n")
		fmt.Fprintf(&buf, "Content-Type: %s; charset=UTF-8\r\n", m.contentType)
		fmt.Fprintf(&buf, "Content-Transfer-Encoding: quoted-printable\r\n\r\n")
		qpWrite(&buf, m.body)
	}

	n, err := w.Write(buf.Bytes())
	return int64(n), err
}

func qpWrite(w io.Writer, s string) {
	for i := 0; i < len(s); i++ {
		b := s[i]
		switch {
		case b == '\r' || b == '\n':
			fmt.Fprintf(w, "%c", b)
		case b == '=' || b > 126 || (b < 32 && b != '\t'):
			fmt.Fprintf(w, "=%02X", b)
		default:
			fmt.Fprintf(w, "%c", b)
		}
	}
}

// FormatAddress formats an email address with a display name per RFC 5322.
func FormatAddress(address, name string) string {
	addr := mail.Address{Name: name, Address: address}
	return addr.String()
}

// NewMessageFrom creates new mail message object with custom From header.
func NewMessageFrom(to []string, from, subject, htmlBody string) *Message {
	log.Trace("NewMessageFrom (htmlBody):\n%s", htmlBody)

	header := make(map[string][]string)
	header["From"] = []string{from}
	header["To"] = to
	header["Subject"] = []string{conf.Email.SubjectPrefix + subject}
	header["Date"] = []string{time.Now().Format(time.RFC1123Z)}

	contentType := "text/html"
	body := htmlBody
	switchedToPlaintext := false
	if conf.Email.UsePlainText || conf.Email.AddPlainTextAlt {
		plainBody, err := html2text.FromString(htmlBody)
		if err != nil {
			log.Error("html2text.FromString: %v", err)
		} else {
			contentType = "text/plain"
			body = plainBody
			switchedToPlaintext = true
		}
	}

	msg := &Message{
		header:      header,
		contentType: contentType,
		body:        body,
		confirmChan: make(chan struct{}),
	}

	if switchedToPlaintext && conf.Email.AddPlainTextAlt && !conf.Email.UsePlainText {
		msg.altParts = append(msg.altParts, altPart{
			contentType: "text/html",
			body:        htmlBody,
		})
	}
	return msg
}

// NewMessage creates new mail message object with default From header.
func NewMessage(to []string, subject, body string) *Message {
	return NewMessageFrom(to, conf.Email.From, subject, body)
}

type loginAuth struct {
	username, password string
}

// LoginAuth returns an smtp.Auth implementing the LOGIN authentication mechanism.
func LoginAuth(username, password string) smtp.Auth {
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

type Sender struct{}

func (*Sender) Send(from string, to []string, msg io.WriterTo) error {
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
			auth = LoginAuth(opts.User, opts.Password)
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

func sendMessage(sender *Sender, msg *Message) error {
	from := msg.header["From"]
	if len(from) == 0 {
		return errors.New("missing From header")
	}
	addr, err := mail.ParseAddress(from[0])
	if err != nil {
		return errors.Wrap(err, "parse From address")
	}

	var to []string
	for _, toAddr := range msg.header["To"] {
		parsed, err := mail.ParseAddress(toAddr)
		if err != nil {
			to = append(to, toAddr)
		} else {
			to = append(to, parsed.Address)
		}
	}

	return sender.Send(addr.Address, to, msg)
}

func processMailQueue() {
	sender := &Sender{}
	for msg := range mailQueue {
		log.Trace("New e-mail sending request %s: %s", msg.GetHeader("To"), msg.Info)
		if err := sendMessage(sender, msg); err != nil {
			log.Error("Failed to send emails %s: %s - %v", msg.GetHeader("To"), msg.Info, err)
		} else {
			log.Trace("E-mails sent %s: %s", msg.GetHeader("To"), msg.Info)
		}
		msg.confirmChan <- struct{}{}
	}
}

var mailQueue chan *Message

// NewContext initializes settings for mailer.
func NewContext() {
	// Need to check if mailQueue is nil because in during reinstall (user had installed
	// before but switched install lock off), this function will be called again
	// while mail queue is already processing tasks, and produces a race condition.
	if !conf.Email.Enabled || mailQueue != nil {
		return
	}

	mailQueue = make(chan *Message, 1000)
	go processMailQueue()
}

// Send puts new message object into mail queue.
// It returns without confirmation (mail processed asynchronously) in normal cases,
// but waits/blocks under hook mode to make sure mail has been sent.
func Send(msg *Message) {
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
