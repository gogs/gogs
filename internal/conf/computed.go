// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package conf

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

var hasMinWinSvc bool

// SupportWindowsService returns true if the application is built with Windows Service support.
func SupportWindowsService() bool {
	return hasMinWinSvc
}

// IsWindowsRuntime returns true if the current runtime in Windows.
func IsWindowsRuntime() bool {
	return runtime.GOOS == "windows"
}

var (
	appPath     string
	appPathOnce sync.Once
)

// AppPath returns the absolute path of the application's binary.
func AppPath() string {
	appPathOnce.Do(func() {
		var err error
		appPath, err = exec.LookPath(os.Args[0])
		if err != nil {
			panic("look executable path: " + err.Error())
		}

		appPath, err = filepath.Abs(appPath)
		if err != nil {
			panic("get absolute executable path: " + err.Error())
		}

		appPath = strings.ReplaceAll(appPath, "\\", "/")
	})

	return appPath
}

var (
	workDir     string
	workDirOnce sync.Once
)

// WorkDir returns the absolute path of work directory. It reads the value of envrionment
// variable GOGS_WORK_DIR when set. Otherwise, it uses the directory where the application's
// binary is located.
func WorkDir() string {
	workDirOnce.Do(func() {
		workDir = os.Getenv("GOGS_WORK_DIR")
		if workDir != "" {
			return
		}

		// NOTE: We don't use filepath.Dir here because it does not handle cases
		// where path starts with two "/" in Windows, e.g. "//psf/Home/..."
		appPath := AppPath()
		i := strings.LastIndex(appPath, "/")
		if i == -1 {
			panic("unreachable")
		}
		workDir = appPath[:i]
	})

	return workDir
}
