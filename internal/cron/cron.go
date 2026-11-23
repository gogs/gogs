package cron

import (
	"context"
	"time"

	"github.com/gogs/cron"
	"unknwon.dev/x/logx"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/database"
)

var c = cron.New()

// NewContext schedules the enabled cron tasks with the cron settings, for the
// repositories under the repository root. forcePrivate makes repositories
// whose statistics are checked private.
func NewContext(logger *logx.Logger, opts conf.CronOpts, repositoryRoot string, forcePrivate bool) {
	var (
		entry *cron.Entry
		err   error
	)
	if opts.UpdateMirror.Enabled {
		entry, err = c.AddFunc("Update mirrors", opts.UpdateMirror.Schedule, func() { database.MirrorUpdate(logger) })
		if err != nil {
			logger.FatalContext(context.Background(), "Failed to add cron task", "task", "Update mirrors", "schedule", opts.UpdateMirror.Schedule, "error", err)
		}
		if opts.UpdateMirror.RunAtStart {
			entry.Prev = time.Now()
			entry.ExecTimes++
			go database.MirrorUpdate(logger)
		}
	}
	if opts.RepoHealthCheck.Enabled {
		entry, err = c.AddFunc("Repository health check", opts.RepoHealthCheck.Schedule, func() {
			database.GitFsck(logger, repositoryRoot, opts.RepoHealthCheck.Timeout, opts.RepoHealthCheck.Args)
		})
		if err != nil {
			logger.FatalContext(context.Background(), "Failed to add cron task", "task", "Repository health check", "schedule", opts.RepoHealthCheck.Schedule, "error", err)
		}
		if opts.RepoHealthCheck.RunAtStart {
			entry.Prev = time.Now()
			entry.ExecTimes++
			go database.GitFsck(logger, repositoryRoot, opts.RepoHealthCheck.Timeout, opts.RepoHealthCheck.Args)
		}
	}
	if opts.CheckRepoStats.Enabled {
		entry, err = c.AddFunc("Check repository statistics", opts.CheckRepoStats.Schedule, func() { database.CheckRepoStats(logger, repositoryRoot, forcePrivate) })
		if err != nil {
			logger.FatalContext(context.Background(), "Failed to add cron task", "task", "Check repository statistics", "schedule", opts.CheckRepoStats.Schedule, "error", err)
		}
		if opts.CheckRepoStats.RunAtStart {
			entry.Prev = time.Now()
			entry.ExecTimes++
			go database.CheckRepoStats(logger, repositoryRoot, forcePrivate)
		}
	}
	if opts.RepoArchiveCleanup.Enabled {
		entry, err = c.AddFunc("Repository archive cleanup", opts.RepoArchiveCleanup.Schedule, func() {
			database.DeleteOldRepositoryArchives(logger, repositoryRoot, opts.RepoArchiveCleanup.OlderThan)
		})
		if err != nil {
			logger.FatalContext(context.Background(), "Failed to add cron task", "task", "Repository archive cleanup", "schedule", opts.RepoArchiveCleanup.Schedule, "error", err)
		}
		if opts.RepoArchiveCleanup.RunAtStart {
			entry.Prev = time.Now()
			entry.ExecTimes++
			go database.DeleteOldRepositoryArchives(logger, repositoryRoot, opts.RepoArchiveCleanup.OlderThan)
		}
	}
	c.Start()
}

// ListTasks returns all running cron tasks.
func ListTasks() []*cron.Entry {
	return c.Entries()
}
