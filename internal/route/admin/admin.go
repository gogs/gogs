// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package admin

import (
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/json-iterator/go"
	"github.com/unknwon/com"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/cron"
	"gogs.io/gogs/internal/db"
	"gogs.io/gogs/internal/mailer"
	"gogs.io/gogs/internal/process"
	"gogs.io/gogs/internal/tool"
)

const (
	DASHBOARD = "admin/dashboard"
	CONFIG    = "admin/config"
	MONITOR   = "admin/monitor"
)

// initTime is the time when the application was initialized.
var initTime = time.Now()

var sysStatus struct {
	Uptime       string
	NumGoroutine int

	// General statistics.
	MemAllocated string // bytes allocated and still in use
	MemTotal     string // bytes allocated (even if freed)
	MemSys       string // bytes obtained from system (sum of XxxSys below)
	Lookups      uint64 // number of pointer lookups
	MemMallocs   uint64 // number of mallocs
	MemFrees     uint64 // number of frees

	// Main allocation heap statistics.
	HeapAlloc    string // bytes allocated and still in use
	HeapSys      string // bytes obtained from system
	HeapIdle     string // bytes in idle spans
	HeapInuse    string // bytes in non-idle span
	HeapReleased string // bytes released to the OS
	HeapObjects  uint64 // total number of allocated objects

	// Low-level fixed-size structure allocator statistics.
	//	Inuse is bytes used now.
	//	Sys is bytes obtained from system.
	StackInuse  string // bootstrap stacks
	StackSys    string
	MSpanInuse  string // mspan structures
	MSpanSys    string
	MCacheInuse string // mcache structures
	MCacheSys   string
	BuckHashSys string // profiling bucket hash table
	GCSys       string // GC metadata
	OtherSys    string // other system allocations

	// Garbage collector statistics.
	NextGC       string // next run in HeapAlloc time (bytes)
	LastGC       string // last run in absolute time (ns)
	PauseTotalNs string
	PauseNs      string // circular buffer of recent GC pause times, most recent at [(NumGC+255)%256]
	NumGC        uint32
}

func updateSystemStatus() {
	sysStatus.Uptime = tool.TimeSincePro(initTime)

	m := new(runtime.MemStats)
	runtime.ReadMemStats(m)
	sysStatus.NumGoroutine = runtime.NumGoroutine()

	sysStatus.MemAllocated = tool.FileSize(int64(m.Alloc))
	sysStatus.MemTotal = tool.FileSize(int64(m.TotalAlloc))
	sysStatus.MemSys = tool.FileSize(int64(m.Sys))
	sysStatus.Lookups = m.Lookups
	sysStatus.MemMallocs = m.Mallocs
	sysStatus.MemFrees = m.Frees

	sysStatus.HeapAlloc = tool.FileSize(int64(m.HeapAlloc))
	sysStatus.HeapSys = tool.FileSize(int64(m.HeapSys))
	sysStatus.HeapIdle = tool.FileSize(int64(m.HeapIdle))
	sysStatus.HeapInuse = tool.FileSize(int64(m.HeapInuse))
	sysStatus.HeapReleased = tool.FileSize(int64(m.HeapReleased))
	sysStatus.HeapObjects = m.HeapObjects

	sysStatus.StackInuse = tool.FileSize(int64(m.StackInuse))
	sysStatus.StackSys = tool.FileSize(int64(m.StackSys))
	sysStatus.MSpanInuse = tool.FileSize(int64(m.MSpanInuse))
	sysStatus.MSpanSys = tool.FileSize(int64(m.MSpanSys))
	sysStatus.MCacheInuse = tool.FileSize(int64(m.MCacheInuse))
	sysStatus.MCacheSys = tool.FileSize(int64(m.MCacheSys))
	sysStatus.BuckHashSys = tool.FileSize(int64(m.BuckHashSys))
	sysStatus.GCSys = tool.FileSize(int64(m.GCSys))
	sysStatus.OtherSys = tool.FileSize(int64(m.OtherSys))

	sysStatus.NextGC = tool.FileSize(int64(m.NextGC))
	sysStatus.LastGC = fmt.Sprintf("%.1fs", float64(time.Now().UnixNano()-int64(m.LastGC))/1000/1000/1000)
	sysStatus.PauseTotalNs = fmt.Sprintf("%.1fs", float64(m.PauseTotalNs)/1000/1000/1000)
	sysStatus.PauseNs = fmt.Sprintf("%.3fs", float64(m.PauseNs[(m.NumGC+255)%256])/1000/1000/1000)
	sysStatus.NumGC = m.NumGC
}

// Operation types.
type AdminOperation int

const (
	CLEAN_INACTIVATE_USER AdminOperation = iota + 1
	CLEAN_REPO_ARCHIVES
	CLEAN_MISSING_REPOS
	GIT_GC_REPOS
	SYNC_SSH_AUTHORIZED_KEY
	SYNC_REPOSITORY_HOOKS
	REINIT_MISSING_REPOSITORY
)

func Dashboard(c *context.Context) {
	c.Title("admin.dashboard")
	c.PageIs("Admin")
	c.PageIs("AdminDashboard")

	// Run operation.
	op, _ := com.StrTo(c.Query("op")).Int()
	if op > 0 {
		var err error
		var success string

		switch AdminOperation(op) {
		case CLEAN_INACTIVATE_USER:
			success = c.Tr("admin.dashboard.delete_inactivate_accounts_success")
			err = db.DeleteInactivateUsers()
		case CLEAN_REPO_ARCHIVES:
			success = c.Tr("admin.dashboard.delete_repo_archives_success")
			err = db.DeleteRepositoryArchives()
		case CLEAN_MISSING_REPOS:
			success = c.Tr("admin.dashboard.delete_missing_repos_success")
			err = db.DeleteMissingRepositories()
		case GIT_GC_REPOS:
			success = c.Tr("admin.dashboard.git_gc_repos_success")
			err = db.GitGcRepos()
		case SYNC_SSH_AUTHORIZED_KEY:
			success = c.Tr("admin.dashboard.resync_all_sshkeys_success")
			err = db.RewriteAuthorizedKeys()
		case SYNC_REPOSITORY_HOOKS:
			success = c.Tr("admin.dashboard.resync_all_hooks_success")
			err = db.SyncRepositoryHooks()
		case REINIT_MISSING_REPOSITORY:
			success = c.Tr("admin.dashboard.reinit_missing_repos_success")
			err = db.ReinitMissingRepositories()
		}

		if err != nil {
			c.Flash.Error(err.Error())
		} else {
			c.Flash.Success(success)
		}
		c.SubURLRedirect("/admin")
		return
	}

	c.Data["GitVersion"] = conf.Git.Version
	c.Data["GoVersion"] = runtime.Version()
	c.Data["BuildTime"] = conf.BuildTime
	c.Data["BuildCommit"] = conf.BuildCommit

	c.Data["Stats"] = db.GetStatistic()
	// FIXME: update periodically
	updateSystemStatus()
	c.Data["SysStatus"] = sysStatus
	c.Success(DASHBOARD)
}

func SendTestMail(c *context.Context) {
	email := c.Query("email")
	// Send a test email to the user's email address and redirect back to Config
	if err := mailer.SendTestMail(email); err != nil {
		c.Flash.Error(c.Tr("admin.config.test_mail_failed", email, err))
	} else {
		c.Flash.Info(c.Tr("admin.config.test_mail_sent", email))
	}

	c.Redirect(conf.Server.Subpath + "/admin/config")
}

func Config(c *context.Context) {
	c.Title("admin.config")
	c.PageIs("Admin")
	c.PageIs("AdminConfig")

	c.Data["App"] = conf.App
	c.Data["Server"] = conf.Server
	c.Data["SSH"] = conf.SSH
	c.Data["Repository"] = conf.Repository
	c.Data["Database"] = conf.Database
	c.Data["Security"] = conf.Security

	c.Data["LogRootPath"] = conf.LogRootPath

	c.Data["HTTP"] = conf.HTTP

	c.Data["Service"] = conf.Service
	c.Data["Webhook"] = conf.Webhook

	c.Data["MailerEnabled"] = false
	if conf.MailService != nil {
		c.Data["MailerEnabled"] = true
		c.Data["Mailer"] = conf.MailService
	}

	c.Data["CacheAdapter"] = conf.CacheAdapter
	c.Data["CacheInterval"] = conf.CacheInterval
	c.Data["CacheConn"] = conf.CacheConn

	c.Data["SessionConfig"] = conf.SessionConfig

	c.Data["DisableGravatar"] = conf.DisableGravatar
	c.Data["EnableFederatedAvatar"] = conf.EnableFederatedAvatar

	c.Data["Git"] = conf.Git

	type logger struct {
		Mode, Config string
	}
	loggers := make([]*logger, len(conf.LogModes))
	for i := range conf.LogModes {
		loggers[i] = &logger{
			Mode: strings.Title(conf.LogModes[i]),
		}

		result, _ := jsoniter.MarshalIndent(conf.LogConfigs[i], "", "  ")
		loggers[i].Config = string(result)
	}
	c.Data["Loggers"] = loggers

	c.HTML(200, CONFIG)
}

func Monitor(c *context.Context) {
	c.Data["Title"] = c.Tr("admin.monitor")
	c.Data["PageIsAdmin"] = true
	c.Data["PageIsAdminMonitor"] = true
	c.Data["Processes"] = process.Processes
	c.Data["Entries"] = cron.ListTasks()
	c.HTML(200, MONITOR)
}
