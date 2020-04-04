// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package authutil

import (
	"encoding/base64"
	"net/http"
	"strings"
)

// DecodeBasic extracts username and password from given header using HTTP Basic Auth.
// It returns empty strings if values are not presented or not valid.
func DecodeBasic(header http.Header) (username, password string) {
	if len(header) == 0 {
		return "", ""
	}

	fields := strings.Fields(header.Get("Authorization"))
	if len(fields) != 2 || fields[0] != "Basic" {
		return "", ""
	}

	p, err := base64.StdEncoding.DecodeString(fields[1])
	if err != nil {
		return "", ""
	}

	creds := strings.SplitN(string(p), ":", 2)
	if len(creds) == 1 {
		return creds[0], ""
	}
	return creds[0], creds[1]
}
