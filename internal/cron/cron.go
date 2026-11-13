package cron

import (
	"time"

	"github.com/gogs/cron"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/logx"
)

var c = cron.New()

func NewContext(logger *logx.Logger) {
	var (
		entry *cron.Entry
		err   error
	)
	if conf.Cron.UpdateMirror.Enabled {
		entry, err = c.AddFunc("Update mirrors", conf.Cron.UpdateMirror.Schedule, func() { database.MirrorUpdate(logger) })
		if err != nil {
			logger.Fatalf("Cron.(update mirrors): %v", err)
		}
		if conf.Cron.UpdateMirror.RunAtStart {
			entry.Prev = time.Now()
			entry.ExecTimes++
			go database.MirrorUpdate(logger)
		}
	}
	if conf.Cron.RepoHealthCheck.Enabled {
		entry, err = c.AddFunc("Repository health check", conf.Cron.RepoHealthCheck.Schedule, func() { database.GitFsck(logger) })
		if err != nil {
			logger.Fatalf("Cron.(repository health check): %v", err)
		}
		if conf.Cron.RepoHealthCheck.RunAtStart {
			entry.Prev = time.Now()
			entry.ExecTimes++
			go database.GitFsck(logger)
		}
	}
	if conf.Cron.CheckRepoStats.Enabled {
		entry, err = c.AddFunc("Check repository statistics", conf.Cron.CheckRepoStats.Schedule, func() { database.CheckRepoStats(logger) })
		if err != nil {
			logger.Fatalf("Cron.(check repository statistics): %v", err)
		}
		if conf.Cron.CheckRepoStats.RunAtStart {
			entry.Prev = time.Now()
			entry.ExecTimes++
			go database.CheckRepoStats(logger)
		}
	}
	if conf.Cron.RepoArchiveCleanup.Enabled {
		entry, err = c.AddFunc("Repository archive cleanup", conf.Cron.RepoArchiveCleanup.Schedule, func() { database.DeleteOldRepositoryArchives(logger) })
		if err != nil {
			logger.Fatalf("Cron.(repository archive cleanup): %v", err)
		}
		if conf.Cron.RepoArchiveCleanup.RunAtStart {
			entry.Prev = time.Now()
			entry.ExecTimes++
			go database.DeleteOldRepositoryArchives(logger)
		}
	}
	c.Start()
}

// ListTasks returns all running cron tasks.
func ListTasks() []*cron.Entry {
	return c.Entries()
}
