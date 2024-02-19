// Copyright 2017 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package database

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_parseRemoteUpdateOutput(t *testing.T) {
	tests := []struct {
		output     string
		expResults []*mirrorSyncResult
	}{
		{
			`
From https://try.gogs.io/unknwon/upsteam
 * [new branch]      develop    -> develop
   b0bb24f..1d85a4f  master     -> master
 - [deleted]         (none)     -> bugfix
`,
			[]*mirrorSyncResult{
				{"develop", gitShortEmptyID, ""},
				{"master", "b0bb24f", "1d85a4f"},
				{"bugfix", "", gitShortEmptyID},
			},
		},
	}
	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			assert.Equal(t, test.expResults, parseRemoteUpdateOutput(test.output))
		})
	}
}
