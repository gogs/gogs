// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package cron

import (
	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/cron"
	"github.com/gogits/gogs/modules/setting"
)

var c = cron.New()

func NewCronContext() {
	if setting.Cron.UpdateMirror.Enabled {
		c.AddFunc("Update mirrors", setting.Cron.UpdateMirror.Schedule, models.MirrorUpdate)
		if setting.Cron.UpdateMirror.RunAtStart {
			go models.MirrorUpdate()
		}
	}
	if setting.Cron.RepoHealthCheck.Enabled {
		c.AddFunc("Repository health check", setting.Cron.RepoHealthCheck.Schedule, models.GitFsck)
		if setting.Cron.RepoHealthCheck.RunAtStart {
			go models.GitFsck()
		}
	}
	if setting.Cron.CheckRepoStats.Enabled {
		c.AddFunc("Check repository statistics", setting.Cron.CheckRepoStats.Schedule, models.CheckRepoStats)
		if setting.Cron.CheckRepoStats.RunAtStart {
			go models.CheckRepoStats()
		}
	}
	c.Start()
}

// ListTasks returns all running cron tasks.
func ListTasks() []*cron.Entry {
	return c.Entries()
}
