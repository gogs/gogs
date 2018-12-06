// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package user

import (
	"os"
)

func CurrentUsername() string {
	user, err := user.Current()
	if err != nil {
		log.Fatal(err)
	}

	curUserName := user.Username
	if len(curUserName) > 0 {
		return curUserName
	}

	curUserName = os.Getenv("USER")
	if len(curUserName) > 0 {
		return curUserName
	}

	return os.Getenv("USERNAME")
}
