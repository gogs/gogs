// Copyright (c) 2013 GPMGo Members. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"testing"
	"time"
)

func TestGPM(t *testing.T) {
	fmt.Println("gpm v0.2.0 Build 0524")

	fmt.Println("\nBuilding gpm application...")
	// Build application.
	var args []string
	args = append(args, "build")
	executeCommand("go", args)

	fmt.Println("\nStart testing command Install...")
	fmt.Println("This package depends on `install_test2`.")
	time.Sleep(2 * time.Second)
	args = make([]string, 0)
	args = append(args, "install")
	args = append(args, "-p")
	args = append(args, "github.com/GPMGoTest/install_test")
	executeCommand("gpm", args)

	fmt.Println("\nStart testing command Remove...")
	fmt.Println("Let's remove `install_test` and `install_test2`.")
	time.Sleep(2 * time.Second)
	args = make([]string, 0)
	args = append(args, "remove")
	args = append(args, "github.com/GPMGoTest/install_test")
	args = append(args, "github.com/GPMGoTest/install_test2")
	executeCommand("gpm", args)
}
