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

// ℹ️ README: This file contains configuration values that require computation to be useful.

// IsWindowsRuntime returns true if the current runtime in Windows.
func IsWindowsRuntime() bool {
	return runtime.GOOS == "windows"
}

// IsProdMode returns true if the application is running in production mode.
func IsProdMode() bool {
	return strings.EqualFold(App.RunMode, "prod")
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
	})

	return appPath
}

var (
	workDir     string
	workDirOnce sync.Once
)

// WorkDir returns the absolute path of work directory. It reads the value of envrionment
// variable GOGS_WORK_DIR. When not set, it uses the directory where the application's
// binary is located.
func WorkDir() string {
	workDirOnce.Do(func() {
		workDir = os.Getenv("GOGS_WORK_DIR")
		if workDir != "" {
			return
		}

		workDir = filepath.Dir(AppPath())
	})

	return workDir
}

var (
	customDir     string
	customDirOnce sync.Once
)

// CustomDir returns the absolute path of the custom directory that contains local overrides.
// It reads the value of envrionment variable GOGS_CUSTOM. When not set, it uses the work
// directory returned by WorkDir fucntion.
func CustomDir() string {
	customDirOnce.Do(func() {
		customDir = os.Getenv("GOGS_CUSTOM")
		if customDir != "" {
			return
		}

		customDir = filepath.Join(WorkDir(), "custom")
	})

	return customDir
}

var (
	homeDir     string
	homeDirOnce sync.Once
)

// HomeDir returns the home directory by reading environment variables. It may return empty
// string when environment variables are not set.
func HomeDir() string {
	homeDirOnce.Do(func() {
		homeDir = os.Getenv("HOME")
		if homeDir != "" {
			return
		}

		homeDir = os.Getenv("USERPROFILE")
		if homeDir != "" {
			return
		}

		homeDir = os.Getenv("HOMEDRIVE") + os.Getenv("HOMEPATH")
	})

	return homeDir
}
