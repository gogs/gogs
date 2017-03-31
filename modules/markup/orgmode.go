// Copyright 2017 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package markup

import (
	"path/filepath"
	"strings"

	"github.com/chaseadamsio/goorgeous"
)

var orgModeExtensions = []string{".org"}

// IsOrgModeFile reports whether name looks like a Org-mode file based on its extension.
func IsOrgModeFile(name string) bool {
	extension := strings.ToLower(filepath.Ext(name))
	for _, ext := range orgModeExtensions {
		if strings.ToLower(ext) == extension {
			return true
		}
	}
	return false
}

// RawOrgMode renders content in Org-mode syntax to HTML without handling special links.
func RawOrgMode(body []byte, urlPrefix string) []byte {
	return goorgeous.OrgCommon(body)
}

// OrgMode takes a string or []byte and renders to HTML in Org-mode syntax with special links.
func OrgMode(input interface{}, urlPrefix string, metas map[string]string) []byte {
	return Render(ORG_MODE, input, urlPrefix, metas)
}
