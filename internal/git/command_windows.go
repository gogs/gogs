//go:build windows

package git

import (
	osexec "os/exec"

	"github.com/cockroachdb/errors"
	"golang.org/x/sys/windows"
)

// processTree is the job object that the command and every process it starts
// run in. Windows has no process groups, and "git.exe" in the PATH is often a
// launcher that runs the actual Git as a child process.
type processTree struct {
	job windows.Handle
}

// newProcessTree creates a job object for the command and terminates the whole
// job when the command is canceled, so that processes Git starts do not
// outlive it.
func newProcessTree(c *osexec.Cmd) (*processTree, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, errors.Wrap(err, "create job object")
	}
	c.Cancel = func() error {
		return windows.TerminateJobObject(job, 1)
	}
	return &processTree{job: job}, nil
}

// attach assigns the started command to the job object. Processes it starts
// from then on join the job as well.
func (t *processTree) attach(c *osexec.Cmd) error {
	process, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(c.Process.Pid))
	if err != nil {
		return errors.Wrap(err, "open process")
	}
	defer func() { _ = windows.CloseHandle(process) }()
	return windows.AssignProcessToJobObject(t.job, process)
}

// close releases the job object. Processes still running in it, e.g., started
// in the background by a hook, keep running.
func (t *processTree) close() {
	_ = windows.CloseHandle(t.job)
}
