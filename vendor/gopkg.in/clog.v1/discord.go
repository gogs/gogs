// Copyright 2018 Unknwon
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
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"time"
)

type (
	discordEmbed struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		Timestamp   string `json:"timestamp"`
		Color       int    `json:"color"`
	}

	discordPayload struct {
		Username string          `json:"username,omitempty"`
		Embeds   []*discordEmbed `json:"embeds"`
	}
)

var (
	discordTitles = []string{
		"Tracing",
		"Information",
		"Warning",
		"Error",
		"Fatal",
	}

	discordColors = []int{
		0,        // Trace
		3843043,  // Info
		16761600, // Warn
		13041721, // Error
		9440319,  // Fatal
	}
)

type DiscordConfig struct {
	// Minimum level of messages to be processed.
	Level LEVEL
	// Buffer size defines how many messages can be queued before hangs.
	BufferSize int64
	// Discord webhook URL.
	URL string
	// Username to be shown for the message.
	// Leave empty to use default as set in the Discord.
	Username string
}

type discord struct {
	Adapter

	url      string
	username string
}

func newDiscord() Logger {
	return &discord{
		Adapter: Adapter{
			quitChan: make(chan struct{}),
		},
	}
}

func (d *discord) Level() LEVEL { return d.level }

func (d *discord) Init(v interface{}) error {
	cfg, ok := v.(DiscordConfig)
	if !ok {
		return ErrConfigObject{"DiscordConfig", v}
	}

	if !isValidLevel(cfg.Level) {
		return ErrInvalidLevel{}
	}
	d.level = cfg.Level

	if len(cfg.URL) == 0 {
		return errors.New("URL cannot be empty")
	}
	d.url = cfg.URL
	d.username = cfg.Username

	d.msgChan = make(chan *Message, cfg.BufferSize)
	return nil
}

func (d *discord) ExchangeChans(errorChan chan<- error) chan *Message {
	d.errorChan = errorChan
	return d.msgChan
}

func buildDiscordPayload(username string, msg *Message) (string, error) {
	payload := discordPayload{
		Username: username,
		Embeds: []*discordEmbed{
			{
				Title:       discordTitles[msg.Level],
				Description: msg.Body[8:],
				Timestamp:   time.Now().Format(time.RFC3339),
				Color:       discordColors[msg.Level],
			},
		},
	}
	p, err := json.Marshal(&payload)
	if err != nil {
		return "", err
	}
	return string(p), nil
}

type rateLimitMsg struct {
	RetryAfter int64 `json:"retry_after"`
}

func (d *discord) postMessage(r io.Reader) (int64, error) {
	resp, err := http.Post(d.url, "application/json", r)
	if err != nil {
		return -1, fmt.Errorf("HTTP Post: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 429 {
		rlMsg := &rateLimitMsg{}
		if err = json.NewDecoder(resp.Body).Decode(&rlMsg); err != nil {
			return -1, fmt.Errorf("decode rate limit message: %v", err)
		}

		return rlMsg.RetryAfter, nil
	} else if resp.StatusCode/100 != 2 {
		data, _ := ioutil.ReadAll(resp.Body)
		return -1, fmt.Errorf("%s", data)
	}

	return -1, nil
}

func (d *discord) write(msg *Message) {
	payload, err := buildDiscordPayload(d.username, msg)
	if err != nil {
		d.errorChan <- fmt.Errorf("discord: builddiscordPayload: %v", err)
		return
	}

	const RETRY_TIMES = 3
	// Due to discord limit, try at most x times with respect to "retry_after" parameter.
	for i := 1; i <= 3; i++ {
		retryAfter, err := d.postMessage(bytes.NewReader([]byte(payload)))
		if err != nil {
			d.errorChan <- fmt.Errorf("discord: postMessage: %v", err)
			return
		}

		if retryAfter > 0 {
			time.Sleep(time.Duration(retryAfter) * time.Millisecond)
			continue
		}

		return
	}

	d.errorChan <- fmt.Errorf("discord: failed to send message after %d retries", RETRY_TIMES)
}

func (d *discord) Start() {
LOOP:
	for {
		select {
		case msg := <-d.msgChan:
			d.write(msg)
		case <-d.quitChan:
			break LOOP
		}
	}

	for {
		if len(d.msgChan) == 0 {
			break
		}

		d.write(<-d.msgChan)
	}
	d.quitChan <- struct{}{} // Notify the cleanup is done.
}

func (d *discord) Destroy() {
	d.quitChan <- struct{}{}
	<-d.quitChan

	close(d.msgChan)
	close(d.quitChan)
}

func init() {
	Register(DISCORD, newDiscord)
}
