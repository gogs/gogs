// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// +build go1.2

// Run as a windows service:
//	sc create gogs binPath= "C:\gogs\gogs.exe web" displayName= "Go Git Service"
//	Set `gogs` Service Attribute: Run as the normal User (Windows Start Button -> Control Panel -> Admin Tools -> Services)
//	net start gogs
//	net stop gogs
//	sc delete gogs

// Gogs(Go Git Service) is a painless self-hosted Git Service written in Go.
package main

import (
	"os"
	"runtime"
	"time"

	"code.google.com/p/winsvc/svc"
	"github.com/codegangsta/cli"
	"github.com/gogits/gogs/cmd"
	"github.com/gogits/gogs/modules/log"
	"github.com/gogits/gogs/modules/setting"
)

const APP_VER = "0.5.8.1122 Beta"

func init() {
	runtime.GOMAXPROCS(runtime.NumCPU())
	setting.AppVer = APP_VER

	workDir, _ := setting.WorkDir()
	os.Chdir(workDir)
}

func Main() {
	app := cli.NewApp()
	app.Name = "Gogs"
	app.Usage = "Go Git Service"
	app.Version = APP_VER
	app.Commands = []cli.Command{
		cmd.CmdWeb,
		cmd.CmdServ,
		cmd.CmdUpdate,
		cmd.CmdFix,
		cmd.CmdDump,
		cmd.CmdCert,
	}
	app.Flags = append(app.Flags, []cli.Flag{}...)
	app.Run(os.Args)
}

func main() {
	isIntSess, err := svc.IsAnInteractiveSession()
	if err != nil {
		log.Fatal(log.TRACE, "Determine if we are running in an interactive session failed: %v", err)
	}
	if !isIntSess {
		serviceName := `gogs`
		log.Info("Starting %s as service", serviceName)
		if err = svc.Run(serviceName, new(gogsService)); err != nil {
			log.Fatal(log.ERROR, "%s service failed: %v", serviceName, err)
			return
		}
		log.Info("%s service stopped", serviceName)
		return
	}

	Main()
}

type gogsService struct{}

func (m *gogsService) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (ssec bool, errno uint32) {
	const cmdsAccepted = svc.AcceptStop | svc.AcceptShutdown | svc.AcceptPauseAndContinue
	changes <- svc.Status{State: svc.StartPending}
	changes <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}
	go Main()
loop:
	for {
		select {
		case c := <-r:
			switch c.Cmd {
			case svc.Interrogate:
				changes <- c.CurrentStatus
				// testing deadlock from https://code.google.com/p/winsvc/issues/detail?id=4
				time.Sleep(100 * time.Millisecond)
				changes <- c.CurrentStatus
			case svc.Stop, svc.Shutdown:
				break loop
			case svc.Pause:
				changes <- svc.Status{State: svc.Paused, Accepts: cmdsAccepted}
			case svc.Continue:
				changes <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}
			default:
				log.Error(log.ERROR, "unexpected control request #%d", c)
			}
		}
	}
	changes <- svc.Status{State: svc.StopPending}
	return
}
