package email

import (
	"crypto/tls"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
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

type message struct {
	info        string
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

func (m *message) getHeader(field string) []string {
	return m.header[field]
}

func sanitizeHeaderValue(v string) string {
	return strings.NewReplacer("\r", "", "\n", "").Replace(v)
}

// foldHeaderValue inserts RFC 5322 folding whitespace (CRLF + space) into a
// header value so that no line exceeds 78 characters. It folds at comma
// boundaries, which is appropriate for address lists.
func foldHeaderValue(prefixLen int, value string) string {
	const maxLine = 78
	if prefixLen+len(value) <= maxLine {
		return value
	}

	var buf strings.Builder
	lineLen := prefixLen
	for i, part := range strings.Split(value, ",") {
		segment := part
		if i > 0 {
			segment = "," + segment
		}
		if lineLen+len(segment) > maxLine && lineLen > prefixLen {
			buf.WriteString("\r\n ")
			lineLen = 1
			segment = strings.TrimLeft(segment, " ")
		}
		buf.WriteString(segment)
		lineLen += len(segment)
	}
	return buf.String()
}

type countingWriter struct {
	w io.Writer
	n int64
}

func (cw *countingWriter) Write(p []byte) (int, error) {
	n, err := cw.w.Write(p)
	cw.n += int64(n)
	return n, err
}

func (m *message) WriteTo(w io.Writer) (int64, error) {
	cw := &countingWriter{w: w}

	for _, field := range []string{"From", "To", "Subject", "Date"} {
		vals := m.header[field]
		if len(vals) == 0 {
			continue
		}

		var encoded string
		if field == "Subject" {
			encoded = mime.QEncoding.Encode("utf-8", vals[0])
		} else {
			encoded = sanitizeHeaderValue(strings.Join(vals, ", "))
		}
		if _, err := fmt.Fprintf(cw, "%s: %s\r\n", field, foldHeaderValue(len(field)+2, encoded)); err != nil {
			return cw.n, errors.Wrap(err, "write header")
		}
	}

	if len(m.altParts) > 0 {
		mw := multipart.NewWriter(cw)
		if _, err := fmt.Fprintf(cw, "MIME-Version: 1.0\r\n"); err != nil {
			return cw.n, errors.Wrap(err, "write MIME version")
		}
		if _, err := fmt.Fprintf(cw, "Content-Type: multipart/alternative; boundary=%s\r\n\r\n", mw.Boundary()); err != nil {
			return cw.n, errors.Wrap(err, "write content type")
		}

		mainHeader := textproto.MIMEHeader{}
		mainHeader.Set("Content-Type", m.contentType+"; charset=UTF-8")
		mainHeader.Set("Content-Transfer-Encoding", "quoted-printable")
		part, err := mw.CreatePart(mainHeader)
		if err != nil {
			return cw.n, errors.Wrap(err, "create main part")
		}
		if err := qpWrite(part, m.body); err != nil {
			return cw.n, errors.Wrap(err, "write main body")
		}

		for _, alt := range m.altParts {
			altHeader := textproto.MIMEHeader{}
			altHeader.Set("Content-Type", alt.contentType+"; charset=UTF-8")
			altHeader.Set("Content-Transfer-Encoding", "quoted-printable")
			part, err := mw.CreatePart(altHeader)
			if err != nil {
				return cw.n, errors.Wrap(err, "create alternative part")
			}
			if err := qpWrite(part, alt.body); err != nil {
				return cw.n, errors.Wrap(err, "write alternative body")
			}
		}
		if err := mw.Close(); err != nil {
			return cw.n, errors.Wrap(err, "close multipart writer")
		}
	} else {
		if _, err := fmt.Fprintf(cw, "MIME-Version: 1.0\r\n"); err != nil {
			return cw.n, errors.Wrap(err, "write MIME version")
		}
		if _, err := fmt.Fprintf(cw, "Content-Type: %s; charset=UTF-8\r\n", m.contentType); err != nil {
			return cw.n, errors.Wrap(err, "write content type")
		}
		if _, err := fmt.Fprintf(cw, "Content-Transfer-Encoding: quoted-printable\r\n\r\n"); err != nil {
			return cw.n, errors.Wrap(err, "write transfer encoding")
		}
		if err := qpWrite(cw, m.body); err != nil {
			return cw.n, errors.Wrap(err, "write body")
		}
	}

	return cw.n, nil
}

func qpWrite(w io.Writer, s string) error {
	qw := quotedprintable.NewWriter(w)
	if _, err := qw.Write([]byte(s)); err != nil {
		return err
	}
	return qw.Close()
}

func formatAddress(address, name string) string {
	addr := mail.Address{Name: name, Address: address}
	return addr.String()
}

func newMessageFrom(to []string, from, subject, htmlBody string) *message {
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

	msg := &message{
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
	from := msg.header["From"]
	if len(from) == 0 {
		return errors.New("missing From header")
	}
	addr, err := mail.ParseAddress(from[0])
	if err != nil {
		return errors.Wrap(err, "parse From address")
	}

	toHeaders := msg.header["To"]
	if len(toHeaders) == 0 {
		return errors.New("missing To header")
	}

	parsedAddrs, err := mail.ParseAddressList(strings.Join(toHeaders, ","))
	if err != nil {
		return errors.Wrap(err, "parse To addresses")
	}

	to := make([]string, 0, len(parsedAddrs))
	for _, a := range parsedAddrs {
		to = append(to, a.Address)
	}

	return sender.Send(addr.Address, to, msg)
}

func processMailQueue() {
	sender := &smtpSender{}
	for msg := range mailQueue {
		to := strings.Join(msg.getHeader("To"), ", ")
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
