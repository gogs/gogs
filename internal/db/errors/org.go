// Copyright 2018 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package errors

import "fmt"

type TeamNotExist struct {
	TeamID int64
	Name   string
}

func IsTeamNotExist(err error) bool {
	_, ok := err.(TeamNotExist)
	return ok
}

func (err TeamNotExist) Error() string {
	return fmt.Sprintf("team does not exist [team_id: %d, name: %s]", err.TeamID, err.Name)
}
