// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package apiv1

import (
	"reflect"

	"github.com/Unknwon/macaron"
	"github.com/macaron-contrib/binding"

	"github.com/gogits/gogs/modules/auth"
)

type MarkdownForm struct {
	Text    string
	Mode    string
	Context string
}

func (f *MarkdownForm) Validate(ctx *macaron.Context, errs binding.Errors) binding.Errors {
	return validateApiReq(errs, ctx.Data, f)
}

func validateApiReq(errs binding.Errors, data map[string]interface{}, f auth.Form) binding.Errors {
	if errs.Len() == 0 {
		return errs
	}

	data["HasError"] = true

	typ := reflect.TypeOf(f)
	val := reflect.ValueOf(f)

	if typ.Kind() == reflect.Ptr {
		typ = typ.Elem()
		val = val.Elem()
	}

	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)

		fieldName := field.Tag.Get("form")
		// Allow ignored fields in the struct
		if fieldName == "-" {
			continue
		}

		if errs[0].FieldNames[0] == field.Name {
			switch errs[0].Classification {
			case binding.ERR_REQUIRED:
				data["ErrorMsg"] = fieldName + " cannot be empty"
			case binding.ERR_ALPHA_DASH:
				data["ErrorMsg"] = fieldName + " must be valid alpha or numeric or dash(-_) characters"
			case binding.ERR_ALPHA_DASH_DOT:
				data["ErrorMsg"] = fieldName + " must be valid alpha or numeric or dash(-_) or dot characters"
			case binding.ERR_MIN_SIZE:
				data["ErrorMsg"] = fieldName + " must contain at least " + auth.GetMinSize(field) + " characters"
			case binding.ERR_MAX_SIZE:
				data["ErrorMsg"] = fieldName + " must contain at most " + auth.GetMaxSize(field) + " characters"
			case binding.ERR_EMAIL:
				data["ErrorMsg"] = fieldName + " is not a valid e-mail address"
			case binding.ERR_URL:
				data["ErrorMsg"] = fieldName + " is not a valid URL"
			default:
				data["ErrorMsg"] = "Unknown error: " + errs[0].Classification
			}
			return errs
		}
	}
	return errs
}
