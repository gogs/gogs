// Copyright (c) 2013 GPMGo Members. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package utils

import (
	"runtime"
	"testing"
)

var remotePaths = []string{
	"github.com/coocood/qbs",
	"code.google.com/p/draw2d",
	"launchpad.net/goamz",
	"bitbucket.org/gotamer/conv",
}

func TestIsValidRemotePath(t *testing.T) {
	for _, p := range remotePaths {
		if !IsValidRemotePath(p) {
			t.Errorf("Invalid remote path: %s", p)
		}
	}
}

var importPaths = []string{
	"github.com/coocood/qbs/test",
	"code.google.com/p/draw2d/test",
	"launchpad.net/goamz/test",
	"bitbucket.org/gotamer/conv/test",
}

func TestGetProjectPath(t *testing.T) {
	// Should return same path.
	for _, p := range remotePaths {
		if p != GetProjectPath(p) {
			t.Errorf("Fail to get projet path: %s", p)
		}
	}

	// Should return same path for remote paths.
	for i, p := range remotePaths {
		if remotePaths[i] != GetProjectPath(p) {
			t.Errorf("Fail to verify projet path: %s", p)
		}
	}
}

func TestGetExecuteName(t *testing.T) {
	// Non-windows.
	if runtime.GOOS != "windows" && GetExecuteName("gtihub.com/astaxie/beego") != "beego" {
		t.Errorf("Fail to verify execute name in non-windows.")
	}

	// Windows.
	if runtime.GOOS == "windows" && GetExecuteName("gtihub.com/astaxie/beego") != "beego.exe" {
		t.Errorf("Fail to verify execute name in windows.")
	}
}

var docFiles = []string{
	"Readme",
	"readme.md",
	"README",
	"README.MD",
	"main.go",
	"LICENse",
}

func TestIsDocFile(t *testing.T) {
	for _, v := range docFiles {
		if !IsDocFile(v) {
			t.Errorf("Fail to verify doc file: %s", v)
		}
	}
}
