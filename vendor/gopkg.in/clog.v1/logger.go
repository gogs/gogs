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

import "fmt"

// Logger is an interface for a logger adapter with specific mode and level.
type Logger interface {
	// Level returns minimum level of given logger.
	Level() LEVEL
	// Init accepts a config struct specific for given logger and performs any necessary initialization.
	Init(interface{}) error
	// ExchangeChans accepts error channel, and returns message receive channel.
	ExchangeChans(chan<- error) chan *Message
	// Start starts message processing.
	Start()
	// Destroy releases all resources.
	Destroy()
}

// Adapter contains common fields for any logger adapter. This struct should be used as embedded struct.
type Adapter struct {
	level     LEVEL
	msgChan   chan *Message
	quitChan  chan struct{}
	errorChan chan<- error
}

type Factory func() Logger

// factories keeps factory function of registered loggers.
var factories = map[MODE]Factory{}

func Register(mode MODE, f Factory) {
	if f == nil {
		panic("clog: register function is nil")
	}
	if factories[mode] != nil {
		panic("clog: register duplicated mode '" + mode + "'")
	}
	factories[mode] = f
}

type receiver struct {
	Logger
	mode    MODE
	msgChan chan *Message
}

var (
	// receivers is a list of loggers with their message channel for broadcasting.
	receivers []*receiver

	errorChan = make(chan error, 5)
	quitChan  = make(chan struct{})
)

func init() {
	// Start background error handling goroutine.
	go func() {
		for {
			select {
			case err := <-errorChan:
				fmt.Printf("clog: unable to write message: %v\n", err)
			case <-quitChan:
				return
			}
		}
	}()
}

// New initializes and appends a new logger to the receiver list.
// Calling this function multiple times will overwrite previous logger with same mode.
func New(mode MODE, cfg interface{}) error {
	factory, ok := factories[mode]
	if !ok {
		return fmt.Errorf("unknown mode '%s'", mode)
	}

	logger := factory()
	if err := logger.Init(cfg); err != nil {
		return err
	}
	msgChan := logger.ExchangeChans(errorChan)

	// Check and replace previous logger.
	hasFound := false
	for i := range receivers {
		if receivers[i].mode == mode {
			hasFound = true

			// Release previous logger.
			receivers[i].Destroy()

			// Update info to new one.
			receivers[i].Logger = logger
			receivers[i].msgChan = msgChan
			break
		}
	}
	if !hasFound {
		receivers = append(receivers, &receiver{
			Logger:  logger,
			mode:    mode,
			msgChan: msgChan,
		})
	}

	go logger.Start()
	return nil
}

// Delete removes logger from the receiver list.
func Delete(mode MODE) {
	foundIdx := -1
	for i := range receivers {
		if receivers[i].mode == mode {
			foundIdx = i
			receivers[i].Destroy()
		}
	}

	if foundIdx >= 0 {
		newList := make([]*receiver, len(receivers)-1)
		copy(newList, receivers[:foundIdx])
		copy(newList[foundIdx:], receivers[foundIdx+1:])
		receivers = newList
	}
}
