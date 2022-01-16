// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package cron

import (
	"time"

	log "unknwon.dev/clog/v2"

	"github.com/gogs/cron"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/db"
)

var c = cron.New()

func NewContext() {
	var (
		entry *cron.Entry
		err   error
	)
	if conf.Cron.UpdateMirror.Enabled {
		entry, err = c.AddFunc("Update mirrors", conf.Cron.UpdateMirror.Schedule, db.MirrorUpdate)
		if err != nil {
			log.Fatal("Cron.(update mirrors): %v", err)
		}
		if conf.Cron.UpdateMirror.RunAtStart {
			entry.Prev = time.Now()
			entry.ExecTimes++
			go db.MirrorUpdate()
		}
	}
	if conf.Cron.RepoHealthCheck.Enabled {
		entry, err = c.AddFunc("Repository health check", conf.Cron.RepoHealthCheck.Schedule, db.GitFsck)
		if err != nil {
			log.Fatal("Cron.(repository health check): %v", err)
		}
		if conf.Cron.RepoHealthCheck.RunAtStart {
			entry.Prev = time.Now()
			entry.ExecTimes++
			go db.GitFsck()
		}
	}
	if conf.Cron.CheckRepoStats.Enabled {
		entry, err = c.AddFunc("Check repository statistics", conf.Cron.CheckRepoStats.Schedule, db.CheckRepoStats)
		if err != nil {
			log.Fatal("Cron.(check repository statistics): %v", err)
		}
		if conf.Cron.CheckRepoStats.RunAtStart {
			entry.Prev = time.Now()
			entry.ExecTimes++
			go db.CheckRepoStats()
		}
	}
	if conf.Cron.RepoArchiveCleanup.Enabled {
		entry, err = c.AddFunc("Repository archive cleanup", conf.Cron.RepoArchiveCleanup.Schedule, db.DeleteOldRepositoryArchives)
		if err != nil {
			log.Fatal("Cron.(repository archive cleanup): %v", err)
		}
		if conf.Cron.RepoArchiveCleanup.RunAtStart {
			entry.Prev = time.Now()
			entry.ExecTimes++
			go db.DeleteOldRepositoryArchives()
		}
	}
	c.Start()
}

// ListTasks returns all running cron tasks.
func ListTasks() []*cron.Entry {
	return c.Entries()
}
