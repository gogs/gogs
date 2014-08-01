// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package apiv1

import (
	"reflect"

	"github.com/Unknwon/macaron"
	"github.com/macaron-contrib/i18n"

	"github.com/gogits/gogs/modules/auth"
	"github.com/gogits/gogs/modules/log"
	"github.com/gogits/gogs/modules/middleware/binding"
)

type MarkdownForm struct {
	Text    string `form:"text" binding:"Required"`
	Mode    string `form:"mode"`
	Context string `form:"context"`
}

func (f *MarkdownForm) Validate(ctx *macaron.Context, errs *binding.Errors, l i18n.Locale) {
	validateApiReq(errs, ctx.Data, f, l)
}

func validateApiReq(errs *binding.Errors, data map[string]interface{}, f interface{}, l i18n.Locale) {
	if errs.Count() == 0 {
		return
	} else if len(errs.Overall) > 0 {
		for _, err := range errs.Overall {
			log.Error(4, "%s: %v", reflect.TypeOf(f), err)
		}
		return
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

		if err, ok := errs.Fields[field.Name]; ok {
			switch err {
			case binding.BindingRequireError:
				data["ErrorMsg"] = fieldName + " cannot be empty"
			case binding.BindingAlphaDashError:
				data["ErrorMsg"] = fieldName + " must be valid alpha or numeric or dash(-_) characters"
			case binding.BindingAlphaDashDotError:
				data["ErrorMsg"] = fieldName + " must be valid alpha or numeric or dash(-_) or dot characters"
			case binding.BindingMinSizeError:
				data["ErrorMsg"] = fieldName + " must contain at least " + auth.GetMinSize(field) + " characters"
			case binding.BindingMaxSizeError:
				data["ErrorMsg"] = fieldName + " must contain at most " + auth.GetMaxSize(field) + " characters"
			case binding.BindingEmailError:
				data["ErrorMsg"] = fieldName + " is not a valid e-mail address"
			case binding.BindingUrlError:
				data["ErrorMsg"] = fieldName + " is not a valid URL"
			default:
				data["ErrorMsg"] = "Unknown error: " + err
			}
			return
		}
	}
}
