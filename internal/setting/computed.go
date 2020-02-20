// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package setting

var supportWindowsService bool

// SupportWindowsService returns true if running as Windows Service is supported.
func SupportWindowsService() bool {
	return supportWindowsService
}
