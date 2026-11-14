package process

import (
	"bytes"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/cockroachdb/errors"
	"unknwon.dev/x/logx"
)

var ErrExecTimeout = errors.New("process execution timeout")

const defaultTimeout = 60 * time.Second

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

var (
	counter   = new(pidCounter)
	Processes []*Process
)

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
func ExecDir(logger *logx.Logger, timeout time.Duration, dir, desc, cmdName string, args ...string) (string, string, error) {
	return ExecDirEnv(logger, timeout, dir, nil, desc, cmdName, args...)
}

// ExecDirEnv is the same as ExecDir but allows appending additional environment
// variables to the child process. Pass nil for env to inherit the parent
// process environment unchanged.
func ExecDirEnv(logger *logx.Logger, timeout time.Duration, dir string, env []string, desc, cmdName string, args ...string) (string, string, error) {
	if timeout == -1 {
		timeout = defaultTimeout
	}

	bufOut := new(bytes.Buffer)
	bufErr := new(bytes.Buffer)

	cmd := exec.Command(cmdName, args...)
	cmd.Dir = dir
	cmd.Stdout = bufOut
	cmd.Stderr = bufErr
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
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
			logger.Error("Failed to kill timed out process", "pid", pid, "description", desc, "error", errKill)
		}
		<-done //nolint:errcheck
		return "", ErrExecTimeout.Error(), ErrExecTimeout
	case err = <-done:
	}

	Remove(pid)
	return bufOut.String(), bufErr.String(), err
}

// Exec starts executing a shell command, it tracks corresponding process and timeout.
func ExecTimeout(logger *logx.Logger, timeout time.Duration, desc, cmdName string, args ...string) (string, string, error) {
	return ExecDir(logger, timeout, "", desc, cmdName, args...)
}

// ExecTimeoutEnv is the same as ExecTimeout but allows appending additional
// environment variables to the child process. Pass nil for env to inherit the
// parent process environment unchanged.
func ExecTimeoutEnv(logger *logx.Logger, timeout time.Duration, env []string, desc, cmdName string, args ...string) (string, string, error) {
	return ExecDirEnv(logger, timeout, "", env, desc, cmdName, args...)
}

// Exec starts executing a shell command, it tracks corresponding its process and use default timeout.
func Exec(logger *logx.Logger, desc, cmdName string, args ...string) (string, string, error) {
	return ExecDir(logger, -1, "", desc, cmdName, args...)
}

// Kill kills and removes a process from global list.
func Kill(pid int64) error {
	for _, proc := range Processes {
		if proc.PID == pid {
			if proc.Cmd != nil && proc.Cmd.Process != nil &&
				proc.Cmd.ProcessState != nil && !proc.Cmd.ProcessState.Exited() {
				if err := proc.Cmd.Process.Kill(); err != nil {
					return errors.Newf("fail to kill process [pid: %d, desc: %s]: %v", proc.PID, proc.Description, err)
				}
			}
			Remove(pid)
			return nil
		}
	}
	return nil
}
