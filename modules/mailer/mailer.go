// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package mailer

import (
	"fmt"
	"net/smtp"
	"strings"

	"github.com/gogits/gogs/modules/log"
	"github.com/gogits/gogs/modules/setting"
)

type Message struct {
	To      []string
	From    string
	Subject string
	Body    string
	User    string
	Type    string
	Massive bool
	Info    string
}

// create mail content
func (m Message) Content() string {
	// set mail type
	contentType := "text/plain; charset=UTF-8"
	if m.Type == "html" {
		contentType = "text/html; charset=UTF-8"
	}

	// create mail content
	content := "From: " + m.From + "<" + m.User +
		">\r\nSubject: " + m.Subject + "\r\nContent-Type: " + contentType + "\r\n\r\n" + m.Body
	return content
}

var mailQueue chan *Message

func NewMailerContext() {
	mailQueue = make(chan *Message, setting.Cfg.MustInt("mailer", "SEND_BUFFER_LEN", 10))
	go processMailQueue()
}

func processMailQueue() {
	for {
		select {
		case msg := <-mailQueue:
			num, err := Send(msg)
			tos := strings.Join(msg.To, "; ")
			info := ""
			if err != nil {
				if len(msg.Info) > 0 {
					info = ", info: " + msg.Info
				}
				log.Error(fmt.Sprintf("Async sent email %d succeed, not send emails: %s%s err: %s", num, tos, info, err))
			} else {
				log.Trace(fmt.Sprintf("Async sent email %d succeed, sent emails: %s%s", num, tos, info))
			}
		}
	}
}

// Direct Send mail message
func Send(msg *Message) (int, error) {
	log.Trace("Sending mails to: %s", strings.Join(msg.To, "; "))
	host := strings.Split(setting.MailService.Host, ":")

	// get message body
	content := msg.Content()

	if len(msg.To) == 0 {
		return 0, fmt.Errorf("empty receive emails")
	} else if len(msg.Body) == 0 {
		return 0, fmt.Errorf("empty email body")
	}

	auth := smtp.PlainAuth("", setting.MailService.User, setting.MailService.Passwd, host[0])

	if msg.Massive {
		// send mail to multiple emails one by one
		num := 0
		for _, to := range msg.To {
			body := []byte("To: " + to + "\r\n" + content)
			err := smtp.SendMail(setting.MailService.Host, auth, msg.From, []string{to}, body)
			if err != nil {
				return num, err
			}
			num++
		}
		return num, nil
	} else {
		body := []byte("To: " + strings.Join(msg.To, ";") + "\r\n" + content)

		// send to multiple emails in one message
		err := smtp.SendMail(setting.MailService.Host, auth, msg.From, msg.To, body)
		if err != nil {
			return 0, err
		} else {
			return 1, nil
		}
	}
}

// Async Send mail message
func SendAsync(msg *Message) {
	go func() {
		mailQueue <- msg
	}()
}

// Create html mail message
func NewHtmlMessage(To []string, From, Subject, Body string) Message {
	return Message{
		To:      To,
		From:    From,
		Subject: Subject,
		Body:    Body,
		Type:    "html",
	}
}
