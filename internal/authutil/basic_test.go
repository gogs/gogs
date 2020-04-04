// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package authutil

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDecodeBasic(t *testing.T) {
	tests := []struct {
		name        string
		header      http.Header
		expUsername string
		expPassword string
	}{
		{
			name: "no header",
		},
		{
			name: "no authorization",
			header: http.Header{
				"Content-Type": []string{"text/plain"},
			},
		},
		{
			name: "malformed value",
			header: http.Header{
				"Authorization": []string{"Basic"},
			},
		},
		{
			name: "not basic",
			header: http.Header{
				"Authorization": []string{"Digest dummy"},
			},
		},
		{
			name: "bad encoding",
			header: http.Header{
				"Authorization": []string{"Basic not_base64"},
			},
		},

		{
			name: "only has username",
			header: http.Header{
				"Authorization": []string{"Basic dXNlcm5hbWU="},
			},
			expUsername: "username",
		},
		{
			name: "has username and password",
			header: http.Header{
				"Authorization": []string{"Basic dXNlcm5hbWU6cGFzc3dvcmQ="},
			},
			expUsername: "username",
			expPassword: "password",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			username, password := DecodeBasic(test.header)
			assert.Equal(t, test.expUsername, username)
			assert.Equal(t, test.expPassword, password)
		})
	}
}
