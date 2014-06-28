// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package process

import (
	"bytes"
	"fmt"
	"os/exec"
	"time"

	"github.com/gogits/gogs/modules/log"
)

// Process represents a working process inherit from Gogs.
type Process struct {
	Pid         int64 // Process ID, not system one.
	Description string
	Start       time.Time
	Cmd         *exec.Cmd
}

// List of existing processes.
var (
	curPid    int64 = 1
	Processes []*Process
)

// Add adds a existing process and returns its PID.
func Add(desc string, cmd *exec.Cmd) int64 {
	pid := curPid
	Processes = append(Processes, &Process{
		Pid:         pid,
		Description: desc,
		Start:       time.Now(),
		Cmd:         cmd,
	})
	curPid++
	return pid
}

func ExecDir(dir, desc, cmdName string, args ...string) (string, string, error) {
	bufOut := new(bytes.Buffer)
	bufErr := new(bytes.Buffer)

	cmd := exec.Command(cmdName, args...)
	cmd.Dir = dir
	cmd.Stdout = bufOut
	cmd.Stderr = bufErr

	pid := Add(desc, cmd)
	err := cmd.Run()
	if errKill := Kill(pid); errKill != nil {
		log.Error("Exec: %v", pid, desc, errKill)
	}
	return bufOut.String(), bufErr.String(), err
}

// Exec starts executing a command and record its process.
func Exec(desc, cmdName string, args ...string) (string, string, error) {
	return ExecDir("", desc, cmdName, args...)
}

// Remove removes a process from list.
func Remove(pid int64) {
	for i, proc := range Processes {
		if proc.Pid == pid {
			Processes = append(Processes[:i], Processes[i+1:]...)
			return
		}
	}
}

// Kill kills and removes a process from list.
func Kill(pid int64) error {
	for i, proc := range Processes {
		if proc.Pid == pid {
			if proc.Cmd.Process != nil && proc.Cmd.ProcessState != nil && !proc.Cmd.ProcessState.Exited() {
				if err := proc.Cmd.Process.Kill(); err != nil {
					return fmt.Errorf("fail to kill process(%d/%s): %v", proc.Pid, proc.Description, err)
				}
			}
			Processes = append(Processes[:i], Processes[i+1:]...)
			return nil
		}
	}
	return nil
}
