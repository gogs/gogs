// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package process

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"sync"
	"time"

	log "gopkg.in/clog.v1"
)

var (
	ErrExecTimeout = errors.New("Process execution timeout")
)

const DEFAULT_TIMEOUT = 60 * time.Second

// Process represents a running process calls shell command.
type Process struct {
	PID         int64
	Description string
	Start       time.Time
	Cmd         *exec.Cmd
}

type pidCounter struct {
	sync.Mutex

	// The current number of pid, initial is 0, and increase 1 every time it's been used.
	pid int64
}

func (c *pidCounter) PID() int64 {
	c.pid++
	return c.pid
}

var counter = new(pidCounter)
var Processes []*Process

// Add adds a process to global list and returns its PID.
func Add(desc string, cmd *exec.Cmd) int64 {
	counter.Lock()
	defer counter.Unlock()

	pid := counter.PID()
	Processes = append(Processes, &Process{
		PID:         pid,
		Description: desc,
		Start:       time.Now(),
		Cmd:         cmd,
	})
	return pid
}

// Remove removes a process from global list.
// It returns true if the process is found and removed by given pid.
func Remove(pid int64) bool {
	counter.Lock()
	defer counter.Unlock()

	for i := range Processes {
		if Processes[i].PID == pid {
			Processes = append(Processes[:i], Processes[i+1:]...)
			return true
		}
	}
	return false
}

// Exec starts executing a shell command in given path, it tracks corresponding process and timeout.
func ExecDir(timeout time.Duration, dir, desc, cmdName string, args ...string) (string, string, error) {
	if timeout == -1 {
		timeout = DEFAULT_TIMEOUT
	}

	bufOut := new(bytes.Buffer)
	bufErr := new(bytes.Buffer)

	cmd := exec.Command(cmdName, args...)
	cmd.Dir = dir
	cmd.Stdout = bufOut
	cmd.Stderr = bufErr
	if err := cmd.Start(); err != nil {
		return "", err.Error(), err
	}

	pid := Add(desc, cmd)
	done := make(chan error)
	go func() {
		done <- cmd.Wait()
	}()

	var err error
	select {
	case <-time.After(timeout):
		if errKill := Kill(pid); errKill != nil {
			log.Error(2, "Fail to kill timeout process [pid: %d, desc: %s]: %v", pid, desc, errKill)
		}
		<-done
		return "", ErrExecTimeout.Error(), ErrExecTimeout
	case err = <-done:
	}

	Remove(pid)
	return bufOut.String(), bufErr.String(), err
}

// Exec starts executing a shell command, it tracks corresponding process and timeout.
func ExecTimeout(timeout time.Duration, desc, cmdName string, args ...string) (string, string, error) {
	return ExecDir(timeout, "", desc, cmdName, args...)
}

// Exec starts executing a shell command, it tracks corresponding its process and use default timeout.
func Exec(desc, cmdName string, args ...string) (string, string, error) {
	return ExecDir(-1, "", desc, cmdName, args...)
}

// Kill kills and removes a process from global list.
func Kill(pid int64) error {
	for _, proc := range Processes {
		if proc.PID == pid {
			if proc.Cmd != nil && proc.Cmd.Process != nil &&
				proc.Cmd.ProcessState != nil && !proc.Cmd.ProcessState.Exited() {
				if err := proc.Cmd.Process.Kill(); err != nil {
					return fmt.Errorf("fail to kill process [pid: %d, desc: %s]: %v", proc.PID, proc.Description, err)
				}
			}
			Remove(pid)
			return nil
		}
	}
	return nil
}
