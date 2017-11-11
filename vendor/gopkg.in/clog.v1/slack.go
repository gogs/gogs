// Copyright 2017 Unknwon
//
// Licensed under the Apache License, Version 2.0 (the "License"): you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
// WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
// License for the specific language governing permissions and limitations
// under the License.

package clog

import (
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
)

const (
	SLACK             = "slack"
	_SLACK_ATTACHMENT = `{
	"attachments": [
		{
			"text": "%s",
			"color": "%s"
		}
	]
}`
)

var slackColors = []string{
	"",        // Trace
	"#3aa3e3", // Info
	"warning", // Warn
	"danger",  // Error
	"#ff0200", // Fatal
}

type SlackConfig struct {
	// Minimum level of messages to be processed.
	Level LEVEL
	// Buffer size defines how many messages can be queued before hangs.
	BufferSize int64
	// Slack webhook URL.
	URL string
}

type slack struct {
	Adapter

	url string
}

func newSlack() Logger {
	return &slack{
		Adapter: Adapter{
			quitChan: make(chan struct{}),
		},
	}
}

func (s *slack) Level() LEVEL { return s.level }

func (s *slack) Init(v interface{}) error {
	cfg, ok := v.(SlackConfig)
	if !ok {
		return ErrConfigObject{"SlackConfig", v}
	}

	if !isValidLevel(cfg.Level) {
		return ErrInvalidLevel{}
	}
	s.level = cfg.Level

	if len(cfg.URL) == 0 {
		return errors.New("URL cannot be empty")
	}
	s.url = cfg.URL

	s.msgChan = make(chan *Message, cfg.BufferSize)
	return nil
}

func (s *slack) ExchangeChans(errorChan chan<- error) chan *Message {
	s.errorChan = errorChan
	return s.msgChan
}

func buildSlackAttachment(msg *Message) string {
	return fmt.Sprintf(_SLACK_ATTACHMENT, msg.Body, slackColors[msg.Level])
}

func (s *slack) write(msg *Message) {
	attachment := buildSlackAttachment(msg)
	resp, err := http.Post(s.url, "application/json", bytes.NewReader([]byte(attachment)))
	if err != nil {
		s.errorChan <- fmt.Errorf("slack: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode/100 != 2 {
		data, _ := ioutil.ReadAll(resp.Body)
		s.errorChan <- fmt.Errorf("slack: %s", data)
	}
}

func (s *slack) Start() {
LOOP:
	for {
		select {
		case msg := <-s.msgChan:
			s.write(msg)
		case <-s.quitChan:
			break LOOP
		}
	}

	for {
		if len(s.msgChan) == 0 {
			break
		}

		s.write(<-s.msgChan)
	}
	s.quitChan <- struct{}{} // Notify the cleanup is done.
}

func (s *slack) Destroy() {
	s.quitChan <- struct{}{}
	<-s.quitChan

	close(s.msgChan)
	close(s.quitChan)
}

func init() {
	Register(SLACK, newSlack)
}
