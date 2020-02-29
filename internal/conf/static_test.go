// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package conf

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_i18n_DateLang(t *testing.T) {
	s := &i18n{
		dateLangs: map[string]string{
			"en-US": "en",
			"zh-CN": "zh",
		},
	}

	tests := []struct {
		lang string
		want string
	}{
		{lang: "en-US", want: "en"},
		{lang: "zh-CN", want: "zh"},
		{lang: "jp-JP", want: "en"},
	}
	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			assert.Equal(t, test.want, s.DateLang(test.lang))
		})
	}
}
