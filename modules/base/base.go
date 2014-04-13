// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package base

type (
	// Type TmplData represents data in the templates.
	TmplData map[string]interface{}
)

// __________.__            .___.__
// \______   \__| ____    __| _/|__| ____    ____
//  |    |  _/  |/    \  / __ | |  |/    \  / ___\
//  |    |   \  |   |  \/ /_/ | |  |   |  \/ /_/  >
//  |______  /__|___|  /\____ | |__|___|  /\___  /
//         \/        \/      \/         \//_____/

// Errors represents the contract of the response body when the
// binding step fails before getting to the application.
type BindingErrors struct {
	Overall map[string]string `json:"overall"`
	Fields  map[string]string `json:"fields"`
}

// Total errors is the sum of errors with the request overall
// and errors on individual fields.
func (err BindingErrors) Count() int {
	return len(err.Overall) + len(err.Fields)
}

func (this *BindingErrors) Combine(other BindingErrors) {
	for key, val := range other.Fields {
		if _, exists := this.Fields[key]; !exists {
			this.Fields[key] = val
		}
	}
	for key, val := range other.Overall {
		if _, exists := this.Overall[key]; !exists {
			this.Overall[key] = val
		}
	}
}

const (
	BindingRequireError         string = "Required"
	BindingAlphaDashError       string = "AlphaDash"
	BindingMinSizeError         string = "MinSize"
	BindingMaxSizeError         string = "MaxSize"
	BindingEmailError           string = "Email"
	BindingUrlError             string = "Url"
	BindingDeserializationError string = "DeserializationError"
	BindingIntegerTypeError     string = "IntegerTypeError"
	BindingBooleanTypeError     string = "BooleanTypeError"
	BindingFloatTypeError       string = "FloatTypeError"
)

var GoGetMetas = make(map[string]bool)
