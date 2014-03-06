// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package auth

import (
	"net/http"
	"reflect"

	"github.com/codegangsta/martini"

	"github.com/gogits/binding"

	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/utils/log"
)

type RegisterForm struct {
	UserName string `form:"username" binding:"Required;AlphaDash;MinSize(5);MaxSize(30)"`
	Email    string `form:"email" binding:"Required;Email;MaxSize(50)"`
	Password string `form:"passwd" binding:"Required;MinSize(6);MaxSize(30)"`
}

func (r *RegisterForm) Validate(errors *binding.Errors, req *http.Request, context martini.Context) {
	if req.Method == "GET" || errors.Count() == 0 {
		return
	}

	data := context.Get(reflect.TypeOf(base.TmplData{})).Interface().(base.TmplData)
	data["HasError"] = true
	AssignForm(r, data)

	if len(errors.Overall) > 0 {
		for _, err := range errors.Overall {
			log.Error("RegisterForm.Validate: %v", err)
		}
		return
	}

	if err, ok := errors.Fields["UserName"]; ok {
		data["Err_Username"] = true
		switch err {
		case binding.RequireError:
			data["ErrorMsg"] = "Username cannot be empty"
		case binding.AlphaDashError:
			data["ErrorMsg"] = "Username must be valid alpha or numeric or dash(-_) characters"
		case binding.MinSizeError:
			data["ErrorMsg"] = "Username at least has 5 characters"
		case binding.MaxSizeError:
			data["ErrorMsg"] = "Username at most has 30 characters"
		default:
			data["ErrorMsg"] = "Unknown error: " + err
		}
		return
	}

	if err, ok := errors.Fields["Email"]; ok {
		data["Err_Email"] = true
		switch err {
		case binding.RequireError:
			data["ErrorMsg"] = "E-mail address cannot be empty"
		case binding.EmailError:
			data["ErrorMsg"] = "E-mail address is not valid"
		case binding.MaxSizeError:
			data["ErrorMsg"] = "E-mail address at most has 50 characters"
		default:
			data["ErrorMsg"] = "Unknown error: " + err
		}
		return
	}

	if err, ok := errors.Fields["Password"]; ok {
		data["Err_Passwd"] = true
		switch err {
		case binding.RequireError:
			data["ErrorMsg"] = "Password cannot be empty"
		case binding.MinSizeError:
			data["ErrorMsg"] = "Password at least has 6 characters"
		case binding.MaxSizeError:
			data["ErrorMsg"] = "Password at most has 30 characters"
		default:
			data["ErrorMsg"] = "Unknown error: " + err
		}
		return
	}

}

// AssignForm assign form values back to the template data.
func AssignForm(form interface{}, data base.TmplData) {
	typ := reflect.TypeOf(form)
	val := reflect.ValueOf(form)

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

		data[fieldName] = val.Field(i).Interface()
	}
}
