// Copyright 2017 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package markup

import (
	"bytes"
	"path/filepath"
	"strings"

	"github.com/niklasfasching/go-org/org"
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
func RawOrgMode(body []byte, urlPrefix string) (result []byte) {
	html, err := org.New().Silent().Parse(bytes.NewReader(body), urlPrefix).Write(org.NewHTMLWriter())
	if err != nil {
		return []byte(err.Error())
	}
	return []byte(html)
}

// OrgMode takes a string or []byte and renders to HTML in Org-mode syntax with special links.
func OrgMode(input any, urlPrefix string, metas map[string]string) []byte {
	return Render(TypeOrgMode, input, urlPrefix, metas)
}
