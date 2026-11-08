//go:build !windows

package main

import (
	"os"
	"os/signal"
	"syscall"

	"gogs.io/gogs/internal/git"
)

// Git commands run in their own process groups, so they do not receive the
// signals that stop Gogs, e.g., Ctrl-C in a terminal. Kill them, then let the
// signal stop Gogs as it would have otherwise.
func init() {
	var sigs []os.Signal
	for _, sig := range []os.Signal{syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP} {
		// Notify would stop ignoring a signal that is ignored, e.g., SIGHUP when
		// running with nohup.
		if !signal.Ignored(sig) {
			sigs = append(sigs, sig)
		}
	}
	if len(sigs) == 0 {
		return
	}

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, sigs...)
	go func() {
		sig := <-ch
		git.KillAll()
		signal.Reset(sig)
		_ = syscall.Kill(os.Getpid(), sig.(syscall.Signal))
	}()
}
