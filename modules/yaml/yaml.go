// Copyright 2016 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package yaml

import (
	"fmt"
	"strings"
	"reflect"
	"path/filepath"

	"gopkg.in/yaml.v2"
	"github.com/microcosm-cc/bluemonday"
	"github.com/gogits/gogs/modules/log"
)

var Sanitizer = bluemonday.UGCPolicy()

// IsYamlFile reports whether name looks like a Yaml file
// based on its extension.
func IsYamlFile(name string) bool {
	name = strings.ToLower(name)
	if ".yaml" == filepath.Ext(name) {
		return true
	}
	return false
}

func renderHorizontalHtmlTable(m yaml.MapSlice) string {
	var thead, tbody, table string
	var mi yaml.MapItem
	for _, mi = range m {
		key := mi.Key
		value := mi.Value

		if  key != nil && reflect.TypeOf(key).String() == "yaml.MapSlice" {
			key = renderHorizontalHtmlTable(key.(yaml.MapSlice))
		}
		thead += fmt.Sprintf("<th>%v</th>", key)

		if value != nil && reflect.TypeOf(value).String() == "yaml.MapSlice" {
			value = renderHorizontalHtmlTable(value.(yaml.MapSlice))
		}
		tbody += fmt.Sprintf("<td>%v</td>", value)
	}

	table = ""
	if len(thead) > 0 {
		table = fmt.Sprintf(`<table data="yaml-metadata"><thead><tr>%s</tr></thead><tbody><tr>%s</tr></table>`, thead, tbody)
	}
	return table
}

func renderVerticalHtmlTable(m []yaml.MapSlice) string {
	var ms yaml.MapSlice
	var mi yaml.MapItem
	var table string

	for _, ms = range m {
		table += `<table data="yaml-metadata">`
		for _, mi = range ms {
			key := mi.Key
			value := mi.Value

			table += `<tr>`
			if key != nil && reflect.TypeOf(key).String() == "yaml.MapSlice" {
				key = renderHorizontalHtmlTable(key.(yaml.MapSlice))
			} else if key != nil && reflect.TypeOf(key).String() == "[]interface {}" {
				var ks string
				for _, ki := range key.([]interface {}) {
					log.Info("KI: %v", ki)
					log.Info("Type: %s", reflect.TypeOf(ki).String())
					ks += renderHorizontalHtmlTable(ki.(yaml.MapSlice))
				}
				key = ks
			}
			table += fmt.Sprintf("<td>%v</td>", key)

			if value != nil && reflect.TypeOf(value).String() == "yaml.MapSlice" {
				value = renderHorizontalHtmlTable(value.(yaml.MapSlice))
			} else if value != nil && reflect.TypeOf(value).String() == "[]interface {}" {
				value = value.([]interface{})
				v := make([]yaml.MapSlice, len(value.([]interface{})))
				for i, vs := range value.([]interface{}) {
					v[i] = vs.(yaml.MapSlice)
				}
				value = renderVerticalHtmlTable(v)
			}
			if key == "slug" {
				value = fmt.Sprintf(`<a href="content/%v.md">%v</a>`, value, value)
			}
			table += fmt.Sprintf("<td>%v</td>", value)

			table += `</tr>`
		}
		table += "</table>"
	}

	return table
}

func RenderYaml(data []byte) []byte {
	mss := []yaml.MapSlice{}

	if len(data) < 1 {
		return data
	}

	lines := strings.Split(string(data), "\r\n")
	if len(lines) == 1 {
		lines = strings.Split(string(data), "\n")
	}
	if len(lines) < 1 {
		return data
	}

	if err := yaml.Unmarshal(data, &mss); err != nil {
		ms := yaml.MapSlice{}
		if err := yaml.Unmarshal(data, &ms); err != nil {
			return data
		}
		return []byte(renderHorizontalHtmlTable(ms))
	}  else {
		return []byte(renderVerticalHtmlTable(mss))
	}
}

func RenderMarkdownYaml(data []byte) []byte {
	mss := []yaml.MapSlice{}

	if len(data) < 1 {
		return []byte("")
	}

	lines := strings.Split(string(data), "\r\n")
	if len(lines) == 1 {
		lines = strings.Split(string(data), "\n")
	}
	if len(lines) < 1 || lines[0] != "---" {
		return []byte("")
	}

	if err := yaml.Unmarshal(data, &mss); err != nil {
		ms := yaml.MapSlice{}
		if err := yaml.Unmarshal(data, &ms); err != nil {
			return []byte("")
		}
		return []byte(renderHorizontalHtmlTable(ms))
	} else {
		return []byte(renderVerticalHtmlTable(mss))
	}
}

func StripYamlFromText(data []byte) []byte {
	mss := []yaml.MapSlice{}
	if err := yaml.Unmarshal(data, &mss); err != nil {
		ms := yaml.MapSlice{}
		if err := yaml.Unmarshal(data, &ms); err != nil {
			return data
		}
	}

	lines := strings.Split(string(data), "\r\n")
	if len(lines) == 1 {
		lines = strings.Split(string(data), "\n")
	}
	if len(lines) < 1 || lines[0] != "---" {
		return data
	}
	body := ""
	atBody := false
	for i, line := range lines {
		if i == 0 {
			continue
		}
		if line == "---" {
			atBody = true
		} else if atBody {
			body += line+"\n"
		}
	}
	return []byte(body)
}

func Render(rawBytes []byte) []byte {
	result := RenderYaml(rawBytes)
	result = Sanitizer.SanitizeBytes(result)
	return result
}

// Renders the YAML and text as a string
func RenderString(rawBytes []byte) string {
	return string(Render(rawBytes))
}

