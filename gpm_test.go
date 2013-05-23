// Copyright (c) 2013 GPMGo Members. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"testing"
)

func TestGPM(t *testing.T) {
	fmt.Println("gpm v0.1.5 Build 0522")

	// Build application.
	var args []string
	args = append(args, "build")
	executeCommand("go", args)

	fmt.Println("Start testing command Install...")
	args = make([]string, 0)
	args = append(args, "install")
	args = append(args, "-p")
	args = append(args, "bitbucket.org/zombiezen/gopdf/pdf")
	executeCommand("gpm", args)

	fmt.Println("Start testing command Remove...")
	args = make([]string, 0)
	args = append(args, "remove")
	args = append(args, "bitbucket.org/zombiezen/gopdf/pdf")
	executeCommand("gpm", args)
}
