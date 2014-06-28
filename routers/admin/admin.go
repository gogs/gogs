// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package admin

import (
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/go-martini/martini"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/cron"
	"github.com/gogits/gogs/modules/middleware"
	"github.com/gogits/gogs/modules/process"
	"github.com/gogits/gogs/modules/setting"
)

const (
	DASHBOARD       base.TplName = "admin/dashboard"
	USERS           base.TplName = "admin/users"
	REPOS           base.TplName = "admin/repos"
	AUTHS           base.TplName = "admin/auths"
	CONFIG          base.TplName = "admin/config"
	MONITOR_PROCESS base.TplName = "admin/monitor/process"
	MONITOR_CRON    base.TplName = "admin/monitor/cron"
)

var startTime = time.Now()

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
	sysStatus.Uptime = base.TimeSincePro(startTime)

	m := new(runtime.MemStats)
	runtime.ReadMemStats(m)
	sysStatus.NumGoroutine = runtime.NumGoroutine()

	sysStatus.MemAllocated = base.FileSize(int64(m.Alloc))
	sysStatus.MemTotal = base.FileSize(int64(m.TotalAlloc))
	sysStatus.MemSys = base.FileSize(int64(m.Sys))
	sysStatus.Lookups = m.Lookups
	sysStatus.MemMallocs = m.Mallocs
	sysStatus.MemFrees = m.Frees

	sysStatus.HeapAlloc = base.FileSize(int64(m.HeapAlloc))
	sysStatus.HeapSys = base.FileSize(int64(m.HeapSys))
	sysStatus.HeapIdle = base.FileSize(int64(m.HeapIdle))
	sysStatus.HeapInuse = base.FileSize(int64(m.HeapInuse))
	sysStatus.HeapReleased = base.FileSize(int64(m.HeapReleased))
	sysStatus.HeapObjects = m.HeapObjects

	sysStatus.StackInuse = base.FileSize(int64(m.StackInuse))
	sysStatus.StackSys = base.FileSize(int64(m.StackSys))
	sysStatus.MSpanInuse = base.FileSize(int64(m.MSpanInuse))
	sysStatus.MSpanSys = base.FileSize(int64(m.MSpanSys))
	sysStatus.MCacheInuse = base.FileSize(int64(m.MCacheInuse))
	sysStatus.MCacheSys = base.FileSize(int64(m.MCacheSys))
	sysStatus.BuckHashSys = base.FileSize(int64(m.BuckHashSys))
	sysStatus.GCSys = base.FileSize(int64(m.GCSys))
	sysStatus.OtherSys = base.FileSize(int64(m.OtherSys))

	sysStatus.NextGC = base.FileSize(int64(m.NextGC))
	sysStatus.LastGC = fmt.Sprintf("%.1fs", float64(time.Now().UnixNano()-int64(m.LastGC))/1000/1000/1000)
	sysStatus.PauseTotalNs = fmt.Sprintf("%.1fs", float64(m.PauseTotalNs)/1000/1000/1000)
	sysStatus.PauseNs = fmt.Sprintf("%.3fs", float64(m.PauseNs[(m.NumGC+255)%256])/1000/1000/1000)
	sysStatus.NumGC = m.NumGC
}

// Operation types.
type AdminOperation int

const (
	CLEAN_UNBIND_OAUTH AdminOperation = iota + 1
	CLEAN_INACTIVATE_USER
)

func Dashboard(ctx *middleware.Context) {
	ctx.Data["Title"] = "Admin Dashboard"
	ctx.Data["PageIsDashboard"] = true

	// Run operation.
	op, _ := base.StrTo(ctx.Query("op")).Int()
	if op > 0 {
		var err error
		var success string

		switch AdminOperation(op) {
		case CLEAN_UNBIND_OAUTH:
			success = "All unbind OAuthes have been deleted."
			err = models.CleanUnbindOauth()
		case CLEAN_INACTIVATE_USER:
			success = "All inactivate accounts have been deleted."
			err = models.DeleteInactivateUsers()
		}

		if err != nil {
			ctx.Flash.Error(err.Error())
		} else {
			ctx.Flash.Success(success)
		}
		ctx.Redirect("/admin")
		return
	}

	ctx.Data["Stats"] = models.GetStatistic()
	updateSystemStatus()
	ctx.Data["SysStatus"] = sysStatus
	ctx.HTML(200, DASHBOARD)
}

func Users(ctx *middleware.Context) {
	ctx.Data["Title"] = "User Management"
	ctx.Data["PageIsUsers"] = true

	var err error
	ctx.Data["Users"], err = models.GetUsers(200, 0)
	if err != nil {
		ctx.Handle(500, "admin.Users(GetUsers)", err)
		return
	}
	ctx.HTML(200, USERS)
}

func Repositories(ctx *middleware.Context) {
	ctx.Data["Title"] = "Repository Management"
	ctx.Data["PageIsRepos"] = true

	var err error
	ctx.Data["Repos"], err = models.GetRepositoriesWithUsers(200, 0)
	if err != nil {
		ctx.Handle(500, "admin.Repositories", err)
		return
	}
	ctx.HTML(200, REPOS)
}

func Auths(ctx *middleware.Context) {
	ctx.Data["Title"] = "Auth Sources"
	ctx.Data["PageIsAuths"] = true

	var err error
	ctx.Data["Sources"], err = models.GetAuths()
	if err != nil {
		ctx.Handle(500, "admin.Auths", err)
		return
	}
	ctx.HTML(200, AUTHS)
}

func Config(ctx *middleware.Context) {
	ctx.Data["Title"] = "Server Configuration"
	ctx.Data["PageIsConfig"] = true

	ctx.Data["AppUrl"] = setting.AppUrl
	ctx.Data["Domain"] = setting.Domain
	ctx.Data["OfflineMode"] = setting.OfflineMode
	ctx.Data["DisableRouterLog"] = setting.DisableRouterLog
	ctx.Data["RunUser"] = setting.RunUser
	ctx.Data["RunMode"] = strings.Title(martini.Env)
	ctx.Data["RepoRootPath"] = setting.RepoRootPath
	ctx.Data["StaticRootPath"] = setting.StaticRootPath
	ctx.Data["LogRootPath"] = setting.LogRootPath
	ctx.Data["ScriptType"] = setting.ScriptType
	ctx.Data["ReverseProxyAuthUser"] = setting.ReverseProxyAuthUser

	ctx.Data["Service"] = setting.Service

	ctx.Data["DbCfg"] = models.DbCfg

	ctx.Data["WebhookTaskInterval"] = setting.WebhookTaskInterval
	ctx.Data["WebhookDeliverTimeout"] = setting.WebhookDeliverTimeout

	ctx.Data["MailerEnabled"] = false
	if setting.MailService != nil {
		ctx.Data["MailerEnabled"] = true
		ctx.Data["Mailer"] = setting.MailService
	}

	ctx.Data["OauthEnabled"] = false
	if setting.OauthService != nil {
		ctx.Data["OauthEnabled"] = true
		ctx.Data["Oauther"] = setting.OauthService
	}

	ctx.Data["CacheAdapter"] = setting.CacheAdapter
	ctx.Data["CacheConfig"] = setting.CacheConfig

	ctx.Data["SessionProvider"] = setting.SessionProvider
	ctx.Data["SessionConfig"] = setting.SessionConfig

	ctx.Data["PictureService"] = setting.PictureService
	ctx.Data["DisableGravatar"] = setting.DisableGravatar

	type logger struct {
		Mode, Config string
	}
	loggers := make([]*logger, len(setting.LogModes))
	for i := range setting.LogModes {
		loggers[i] = &logger{setting.LogModes[i], setting.LogConfigs[i]}
	}
	ctx.Data["Loggers"] = loggers

	ctx.HTML(200, CONFIG)
}

func Monitor(ctx *middleware.Context) {
	ctx.Data["Title"] = "Monitoring Center"
	ctx.Data["PageIsMonitor"] = true

	tab := ctx.Query("tab")
	switch tab {
	case "process":
		ctx.Data["PageIsMonitorProcess"] = true
		ctx.Data["Processes"] = process.Processes
		ctx.HTML(200, MONITOR_PROCESS)
	default:
		ctx.Data["PageIsMonitorCron"] = true
		ctx.Data["Entries"] = cron.ListEntries()
		ctx.HTML(200, MONITOR_CRON)
	}
}
