// Copyright 2017 Unknwon
//
// Licensed under the Apache License, Version 2.0 (the "License"): you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
// WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
// License for the specific language governing permissions and limitations
// under the License.

package clog

import "fmt"

type ErrConfigObject struct {
	expect string
	got    interface{}
}

func (err ErrConfigObject) Error() string {
	return fmt.Sprintf("config object is not an instance of %s, instead got '%T'", err.expect, err.got)
}

type ErrInvalidLevel struct{}

func (err ErrInvalidLevel) Error() string {
	return "input level is not one of: TRACE, INFO, WARN, ERROR or FATAL"
}
