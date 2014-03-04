// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package auth

import (
	"fmt"

	"github.com/gogits/validation"
)

func GenerateErrorMsg(e *validation.ValidationError) string {
	return fmt.Sprintf("%v", e.LimitValue)
}
