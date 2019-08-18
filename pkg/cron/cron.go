// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package cron

import (
	"time"

	log "gopkg.in/clog.v1"

	"github.com/gogs/cron"

	"github.com/gogs/gogs/models"
	"github.com/gogs/gogs/pkg/setting"
)

var c = cron.New()

func NewContext() {
	var (
		entry *cron.Entry
		err   error
	)
	if setting.Cron.UpdateMirror.Enabled {
		entry, err = c.AddFunc("Update mirrors", setting.Cron.UpdateMirror.Schedule, models.MirrorUpdate)
		if err != nil {
			log.Fatal(2, "Cron.(update mirrors): %v", err)
		}
		if setting.Cron.UpdateMirror.RunAtStart {
			entry.Prev = time.Now()
			entry.ExecTimes++
			go models.MirrorUpdate()
		}
	}
	if setting.Cron.RepoHealthCheck.Enabled {
		entry, err = c.AddFunc("Repository health check", setting.Cron.RepoHealthCheck.Schedule, models.GitFsck)
		if err != nil {
			log.Fatal(2, "Cron.(repository health check): %v", err)
		}
		if setting.Cron.RepoHealthCheck.RunAtStart {
			entry.Prev = time.Now()
			entry.ExecTimes++
			go models.GitFsck()
		}
	}
	if setting.Cron.CheckRepoStats.Enabled {
		entry, err = c.AddFunc("Check repository statistics", setting.Cron.CheckRepoStats.Schedule, models.CheckRepoStats)
		if err != nil {
			log.Fatal(2, "Cron.(check repository statistics): %v", err)
		}
		if setting.Cron.CheckRepoStats.RunAtStart {
			entry.Prev = time.Now()
			entry.ExecTimes++
			go models.CheckRepoStats()
		}
	}
	if setting.Cron.RepoArchiveCleanup.Enabled {
		entry, err = c.AddFunc("Repository archive cleanup", setting.Cron.RepoArchiveCleanup.Schedule, models.DeleteOldRepositoryArchives)
		if err != nil {
			log.Fatal(2, "Cron.(repository archive cleanup): %v", err)
		}
		if setting.Cron.RepoArchiveCleanup.RunAtStart {
			entry.Prev = time.Now()
			entry.ExecTimes++
			go models.DeleteOldRepositoryArchives()
		}
	}
	c.Start()
}

// ListTasks returns all running cron tasks.
func ListTasks() []*cron.Entry {
	return c.Entries()
}
