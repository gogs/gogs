// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package semverutil

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCheck(t *testing.T) {
	tests := []struct {
		version1   string
		comparison string
		version2   string
		expVal     bool
	}{
		{version1: "1.0.c", comparison: ">", version2: "0.9", expVal: false},
		{version1: "1.0.1", comparison: ">", version2: "0.9.a", expVal: false},

		{version1: "7.2", comparison: ">=", version2: "5.1", expVal: true},
		{version1: "1.8.3.1", comparison: ">=", version2: "1.8.3", expVal: true},
		{version1: "0.12.0+dev", comparison: ">=", version2: "0.11.68.1023", expVal: true},
	}
	for _, test := range tests {
		t.Run(test.version1+" "+test.comparison+" "+test.version2, func(t *testing.T) {
			assert.Equal(t, test.expVal, Compare(test.version1, test.comparison, test.version2))
		})
	}
}
