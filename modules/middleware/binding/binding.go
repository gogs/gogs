// Copyright 2013 The Martini Contrib Authors. All rights reserved.
// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package binding

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/go-martini/martini"
)

/*
	To the land of Middle-ware Earth:

		One func to rule them all,
		One func to find them,
		One func to bring them all,
		And in this package BIND them.
*/

// Bind accepts a copy of an empty struct and populates it with
// values from the request (if deserialization is successful). It
// wraps up the functionality of the Form and Json middleware
// according to the Content-Type of the request, and it guesses
// if no Content-Type is specified. Bind invokes the ErrorHandler
// middleware to bail out if errors occurred. If you want to perform
// your own error handling, use Form or Json middleware directly.
// An interface pointer can be added as a second argument in order
// to map the struct to a specific interface.
func Bind(obj interface{}, ifacePtr ...interface{}) martini.Handler {
	return func(context martini.Context, req *http.Request) {
		contentType := req.Header.Get("Content-Type")

		if strings.Contains(contentType, "form-urlencoded") {
			context.Invoke(Form(obj, ifacePtr...))
		} else if strings.Contains(contentType, "multipart/form-data") {
			context.Invoke(MultipartForm(obj, ifacePtr...))
		} else if strings.Contains(contentType, "json") {
			context.Invoke(Json(obj, ifacePtr...))
		} else {
			context.Invoke(Json(obj, ifacePtr...))
			if getErrors(context).Count() > 0 {
				context.Invoke(Form(obj, ifacePtr...))
			}
		}

		context.Invoke(ErrorHandler)
	}
}

// BindIgnErr will do the exactly same thing as Bind but without any
// error handling, which user has freedom to deal with them.
// This allows user take advantages of validation.
func BindIgnErr(obj interface{}, ifacePtr ...interface{}) martini.Handler {
	return func(context martini.Context, req *http.Request) {
		contentType := req.Header.Get("Content-Type")

		if strings.Contains(contentType, "form-urlencoded") {
			context.Invoke(Form(obj, ifacePtr...))
		} else if strings.Contains(contentType, "multipart/form-data") {
			context.Invoke(MultipartForm(obj, ifacePtr...))
		} else if strings.Contains(contentType, "json") {
			context.Invoke(Json(obj, ifacePtr...))
		} else {
			context.Invoke(Json(obj, ifacePtr...))
			if getErrors(context).Count() > 0 {
				context.Invoke(Form(obj, ifacePtr...))
			}
		}
	}
}

// Form is middleware to deserialize form-urlencoded data from the request.
// It gets data from the form-urlencoded body, if present, or from the
// query string. It uses the http.Request.ParseForm() method
// to perform deserialization, then reflection is used to map each field
// into the struct with the proper type. Structs with primitive slice types
// (bool, float, int, string) can support deserialization of repeated form
// keys, for example: key=val1&key=val2&key=val3
// An interface pointer can be added as a second argument in order
// to map the struct to a specific interface.
func Form(formStruct interface{}, ifacePtr ...interface{}) martini.Handler {
	return func(context martini.Context, req *http.Request) {
		ensureNotPointer(formStruct)
		formStruct := reflect.New(reflect.TypeOf(formStruct))
		errors := newErrors()
		parseErr := req.ParseForm()

		// Format validation of the request body or the URL would add considerable overhead,
		// and ParseForm does not complain when URL encoding is off.
		// Because an empty request body or url can also mean absence of all needed values,
		// it is not in all cases a bad request, so let's return 422.
		if parseErr != nil {
			errors.Overall[BindingDeserializationError] = parseErr.Error()
		}

		mapForm(formStruct, req.Form, errors)

		validateAndMap(formStruct, context, errors, ifacePtr...)
	}
}

func MultipartForm(formStruct interface{}, ifacePtr ...interface{}) martini.Handler {
	return func(context martini.Context, req *http.Request) {
		ensureNotPointer(formStruct)
		formStruct := reflect.New(reflect.TypeOf(formStruct))
		errors := newErrors()

		// Workaround for multipart forms returning nil instead of an error
		// when content is not multipart
		// https://code.google.com/p/go/issues/detail?id=6334
		multipartReader, err := req.MultipartReader()
		if err != nil {
			errors.Overall[BindingDeserializationError] = err.Error()
		} else {
			form, parseErr := multipartReader.ReadForm(MaxMemory)

			if parseErr != nil {
				errors.Overall[BindingDeserializationError] = parseErr.Error()
			}

			req.MultipartForm = form
		}

		mapForm(formStruct, req.MultipartForm.Value, errors)

		validateAndMap(formStruct, context, errors, ifacePtr...)
	}
}

// Json is middleware to deserialize a JSON payload from the request
// into the struct that is passed in. The resulting struct is then
// validated, but no error handling is actually performed here.
// An interface pointer can be added as a second argument in order
// to map the struct to a specific interface.
func Json(jsonStruct interface{}, ifacePtr ...interface{}) martini.Handler {
	return func(context martini.Context, req *http.Request) {
		ensureNotPointer(jsonStruct)
		jsonStruct := reflect.New(reflect.TypeOf(jsonStruct))
		errors := newErrors()

		if req.Body != nil {
			defer req.Body.Close()
		}

		if err := json.NewDecoder(req.Body).Decode(jsonStruct.Interface()); err != nil && err != io.EOF {
			errors.Overall[BindingDeserializationError] = err.Error()
		}

		validateAndMap(jsonStruct, context, errors, ifacePtr...)
	}
}

// Validate is middleware to enforce required fields. If the struct
// passed in is a Validator, then the user-defined Validate method
// is executed, and its errors are mapped to the context. This middleware
// performs no error handling: it merely detects them and maps them.
func Validate(obj interface{}) martini.Handler {
	return func(context martini.Context, req *http.Request) {
		errors := newErrors()
		validateStruct(errors, obj)

		if validator, ok := obj.(Validator); ok {
			validator.Validate(errors, req, context)
		}
		context.Map(*errors)
	}
}

var (
	alphaDashPattern    = regexp.MustCompile("[^\\d\\w-_]")
	alphaDashDotPattern = regexp.MustCompile("[^\\d\\w-_\\.]")
	emailPattern        = regexp.MustCompile("[\\w!#$%&'*+/=?^_`{|}~-]+(?:\\.[\\w!#$%&'*+/=?^_`{|}~-]+)*@(?:[\\w](?:[\\w-]*[\\w])?\\.)+[a-zA-Z0-9](?:[\\w-]*[\\w])?")
	urlPattern          = regexp.MustCompile(`(http|https):\/\/[\w\-_]+(\.[\w\-_]+)+([\w\-\.,@?^=%&amp;:/~\+#]*[\w\-\@?^=%&amp;/~\+#])?`)
)

func validateStruct(errors *Errors, obj interface{}) {
	typ := reflect.TypeOf(obj)
	val := reflect.ValueOf(obj)

	if typ.Kind() == reflect.Ptr {
		typ = typ.Elem()
		val = val.Elem()
	}

	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)

		// Allow ignored fields in the struct
		if field.Tag.Get("form") == "-" {
			continue
		}

		fieldValue := val.Field(i).Interface()
		if field.Type.Kind() == reflect.Struct {
			validateStruct(errors, fieldValue)
			continue
		}

		zero := reflect.Zero(field.Type).Interface()

		// Match rules.
		for _, rule := range strings.Split(field.Tag.Get("binding"), ";") {
			if len(rule) == 0 {
				continue
			}

			switch {
			case rule == "Required":
				if reflect.DeepEqual(zero, fieldValue) {
					errors.Fields[field.Name] = BindingRequireError
					break
				}
			case rule == "AlphaDash":
				if alphaDashPattern.MatchString(fmt.Sprintf("%v", fieldValue)) {
					errors.Fields[field.Name] = BindingAlphaDashError
					break
				}
			case rule == "AlphaDashDot":
				if alphaDashDotPattern.MatchString(fmt.Sprintf("%v", fieldValue)) {
					errors.Fields[field.Name] = BindingAlphaDashDotError
					break
				}
			case strings.HasPrefix(rule, "MinSize("):
				min, err := strconv.Atoi(rule[8 : len(rule)-1])
				if err != nil {
					errors.Overall["MinSize"] = err.Error()
					break
				}
				if str, ok := fieldValue.(string); ok && utf8.RuneCountInString(str) < min {
					errors.Fields[field.Name] = BindingMinSizeError
					break
				}
				v := reflect.ValueOf(fieldValue)
				if v.Kind() == reflect.Slice && v.Len() < min {
					errors.Fields[field.Name] = BindingMinSizeError
					break
				}
			case strings.HasPrefix(rule, "MaxSize("):
				max, err := strconv.Atoi(rule[8 : len(rule)-1])
				if err != nil {
					errors.Overall["MaxSize"] = err.Error()
					break
				}
				if str, ok := fieldValue.(string); ok && utf8.RuneCountInString(str) > max {
					errors.Fields[field.Name] = BindingMaxSizeError
					break
				}
				v := reflect.ValueOf(fieldValue)
				if v.Kind() == reflect.Slice && v.Len() > max {
					errors.Fields[field.Name] = BindingMinSizeError
					break
				}
			case rule == "Email":
				if !emailPattern.MatchString(fmt.Sprintf("%v", fieldValue)) {
					errors.Fields[field.Name] = BindingEmailError
					break
				}
			case rule == "Url":
				str := fmt.Sprintf("%v", fieldValue)
				if len(str) == 0 {
					continue
				} else if !urlPattern.MatchString(str) {
					errors.Fields[field.Name] = BindingUrlError
					break
				}
			}
		}
	}
}

func mapForm(formStruct reflect.Value, form map[string][]string, errors *Errors) {
	typ := formStruct.Elem().Type()

	for i := 0; i < typ.NumField(); i++ {
		typeField := typ.Field(i)
		if inputFieldName := typeField.Tag.Get("form"); inputFieldName != "" {
			structField := formStruct.Elem().Field(i)
			if !structField.CanSet() {
				continue
			}

			inputValue, exists := form[inputFieldName]

			if !exists {
				continue
			}

			numElems := len(inputValue)
			if structField.Kind() == reflect.Slice && numElems > 0 {
				sliceOf := structField.Type().Elem().Kind()
				slice := reflect.MakeSlice(structField.Type(), numElems, numElems)
				for i := 0; i < numElems; i++ {
					setWithProperType(sliceOf, inputValue[i], slice.Index(i), inputFieldName, errors)
				}
				formStruct.Elem().Field(i).Set(slice)
			} else {
				setWithProperType(typeField.Type.Kind(), inputValue[0], structField, inputFieldName, errors)
			}
		}
	}
}

// ErrorHandler simply counts the number of errors in the
// context and, if more than 0, writes a 400 Bad Request
// response and a JSON payload describing the errors with
// the "Content-Type" set to "application/json".
// Middleware remaining on the stack will not even see the request
// if, by this point, there are any errors.
// This is a "default" handler, of sorts, and you are
// welcome to use your own instead. The Bind middleware
// invokes this automatically for convenience.
func ErrorHandler(errs Errors, resp http.ResponseWriter) {
	if errs.Count() > 0 {
		resp.Header().Set("Content-Type", "application/json; charset=utf-8")
		if _, ok := errs.Overall[BindingDeserializationError]; ok {
			resp.WriteHeader(http.StatusBadRequest)
		} else {
			resp.WriteHeader(422)
		}
		errOutput, _ := json.Marshal(errs)
		resp.Write(errOutput)
		return
	}
}

// This sets the value in a struct of an indeterminate type to the
// matching value from the request (via Form middleware) in the
// same type, so that not all deserialized values have to be strings.
// Supported types are string, int, float, and bool.
func setWithProperType(valueKind reflect.Kind, val string, structField reflect.Value, nameInTag string, errors *Errors) {
	switch valueKind {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if val == "" {
			val = "0"
		}
		intVal, err := strconv.ParseInt(val, 10, 64)
		if err != nil {
			errors.Fields[nameInTag] = BindingIntegerTypeError
		} else {
			structField.SetInt(intVal)
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if val == "" {
			val = "0"
		}
		uintVal, err := strconv.ParseUint(val, 10, 64)
		if err != nil {
			errors.Fields[nameInTag] = BindingIntegerTypeError
		} else {
			structField.SetUint(uintVal)
		}
	case reflect.Bool:
		structField.SetBool(val == "on")
	case reflect.Float32:
		if val == "" {
			val = "0.0"
		}
		floatVal, err := strconv.ParseFloat(val, 32)
		if err != nil {
			errors.Fields[nameInTag] = BindingFloatTypeError
		} else {
			structField.SetFloat(floatVal)
		}
	case reflect.Float64:
		if val == "" {
			val = "0.0"
		}
		floatVal, err := strconv.ParseFloat(val, 64)
		if err != nil {
			errors.Fields[nameInTag] = BindingFloatTypeError
		} else {
			structField.SetFloat(floatVal)
		}
	case reflect.String:
		structField.SetString(val)
	}
}

// Don't pass in pointers to bind to. Can lead to bugs. See:
// https://github.com/codegangsta/martini-contrib/issues/40
// https://github.com/codegangsta/martini-contrib/pull/34#issuecomment-29683659
func ensureNotPointer(obj interface{}) {
	if reflect.TypeOf(obj).Kind() == reflect.Ptr {
		panic("Pointers are not accepted as binding models")
	}
}

// Performs validation and combines errors from validation
// with errors from deserialization, then maps both the
// resulting struct and the errors to the context.
func validateAndMap(obj reflect.Value, context martini.Context, errors *Errors, ifacePtr ...interface{}) {
	context.Invoke(Validate(obj.Interface()))
	errors.Combine(getErrors(context))
	context.Map(*errors)
	context.Map(obj.Elem().Interface())
	if len(ifacePtr) > 0 {
		context.MapTo(obj.Elem().Interface(), ifacePtr[0])
	}
}

func newErrors() *Errors {
	return &Errors{make(map[string]string), make(map[string]string)}
}

func getErrors(context martini.Context) Errors {
	return context.Get(reflect.TypeOf(Errors{})).Interface().(Errors)
}

type (
	// Implement the Validator interface to define your own input
	// validation before the request even gets to your application.
	// The Validate method will be executed during the validation phase.
	Validator interface {
		Validate(*Errors, *http.Request, martini.Context)
	}
)

var (
	// Maximum amount of memory to use when parsing a multipart form.
	// Set this to whatever value you prefer; default is 10 MB.
	MaxMemory = int64(1024 * 1024 * 10)
)

// Errors represents the contract of the response body when the
// binding step fails before getting to the application.
type Errors struct {
	Overall map[string]string `json:"overall"`
	Fields  map[string]string `json:"fields"`
}

// Total errors is the sum of errors with the request overall
// and errors on individual fields.
func (err Errors) Count() int {
	return len(err.Overall) + len(err.Fields)
}

func (this *Errors) Combine(other Errors) {
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
	BindingAlphaDashDotError    string = "AlphaDashDot"
	BindingMinSizeError         string = "MinSize"
	BindingMaxSizeError         string = "MaxSize"
	BindingEmailError           string = "Email"
	BindingUrlError             string = "Url"
	BindingDeserializationError string = "DeserializationError"
	BindingIntegerTypeError     string = "IntegerTypeError"
	BindingBooleanTypeError     string = "BooleanTypeError"
	BindingFloatTypeError       string = "FloatTypeError"
)
