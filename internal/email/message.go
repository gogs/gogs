// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package email

import (
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/smtp"
	"os"
	"strings"
	"time"

	"github.com/jaytaylor/html2text"
	"gopkg.in/gomail.v2"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/conf"
)

type Message struct {
	Info string // Message information for log purpose.
	*gomail.Message
	confirmChan chan struct{}
}

// NewMessageFrom creates new mail message object with custom From header.
func NewMessageFrom(to []string, from, subject, htmlBody string) *Message {
	log.Trace("NewMessageFrom (htmlBody):\n%s", htmlBody)

	msg := gomail.NewMessage()
	msg.SetHeader("From", from)
	msg.SetHeader("To", to...)
	msg.SetHeader("Subject", conf.Email.SubjectPrefix+subject)
	msg.SetDateHeader("Date", time.Now())

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
	msg.SetBody(contentType, body)
	if switchedToPlaintext && conf.Email.AddPlainTextAlt && !conf.Email.UsePlainText {
		// The AddAlternative method name is confusing - adding html as an "alternative" will actually cause mail
		// clients to show it as first priority, and the text "main body" is the 2nd priority fallback.
		// See: https://godoc.org/gopkg.in/gomail.v2#Message.AddAlternative
		msg.AddAlternative("text/html", htmlBody)
	}
	return &Message{
		Message:     msg,
		confirmChan: make(chan struct{}),
	}
}

// NewMessage creates new mail message object with default From header.
func NewMessage(to []string, subject, body string) *Message {
	return NewMessageFrom(to, conf.Email.From, subject, body)
}

type loginAuth struct {
	username, password string
}

// SMTP AUTH LOGIN Auth Handler
func LoginAuth(username, password string) smtp.Auth {
	return &loginAuth{username, password}
}

func (a *loginAuth) Start(server *smtp.ServerInfo) (string, []byte, error) {
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
			return nil, fmt.Errorf("unknwon fromServer: %s", string(fromServer))
		}
	}
	return nil, nil
}

type Sender struct {
}

func (s *Sender) Send(from string, to []string, msg io.WriterTo) error {
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
		return fmt.Errorf("NewClient: %v", err)
	}

	if !opts.DisableHELO {
		hostname := opts.HELOHostname
		if len(hostname) == 0 {
			hostname, err = os.Hostname()
			if err != nil {
				return err
			}
		}

		if err = client.Hello(hostname); err != nil {
			return fmt.Errorf("Hello: %v", err)
		}
	}

	// If not using SMTPS, always use STARTTLS if available
	hasStartTLS, _ := client.Extension("STARTTLS")
	if !isSecureConn && hasStartTLS {
		if err = client.StartTLS(tlsconfig); err != nil {
			return fmt.Errorf("StartTLS: %v", err)
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
			// Patch for AUTH LOGIN
			auth = LoginAuth(opts.User, opts.Password)
		}

		if auth != nil {
			if err = client.Auth(auth); err != nil {
				return fmt.Errorf("Auth: %v", err)
			}
		}
	}

	if err = client.Mail(from); err != nil {
		return fmt.Errorf("Mail: %v", err)
	}

	for _, rec := range to {
		if err = client.Rcpt(rec); err != nil {
			return fmt.Errorf("Rcpt: %v", err)
		}
	}

	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("Data: %v", err)
	} else if _, err = msg.WriteTo(w); err != nil {
		return fmt.Errorf("WriteTo: %v", err)
	} else if err = w.Close(); err != nil {
		return fmt.Errorf("Close: %v", err)
	}

	return client.Quit()
}

func processMailQueue() {
	sender := &Sender{}
	for msg := range mailQueue {
		log.Trace("New e-mail sending request %s: %s", msg.GetHeader("To"), msg.Info)
		if err := gomail.Send(sender, msg.Message); err != nil {
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
	mailQueue <- msg

	if conf.HookMode {
		<-msg.confirmChan
		return
	}

	go func() {
		<-msg.confirmChan
	}()
}
