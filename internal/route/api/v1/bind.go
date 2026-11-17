package v1

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"

	"github.com/go-macaron/binding"
	"github.com/unknwon/com"
	"gopkg.in/yaml.v3"
)

// maxMultipartMemory is the maximum amount of memory used to parse a multipart
// form.
const maxMultipartMemory = 10 << 20 // 10 MiB

// bind returns a handler that decodes the request body into a new instance of
// the model based on the Content-Type of the request, validates it against the
// "binding" struct tags, and maps it for subsequent handlers.
//
// It mirrors how github.com/go-macaron/binding binds request bodies, which API
// v1 used before migrating to Flamego, so that clients get the same responses:
// a JSON list of binding errors with 415 for an unsupported Content-Type, 400
// for a malformed body, and 422 for a body that fails validation.
func bind(model any) func(c *apiContext) {
	typ := reflect.TypeOf(model)
	return func(c *apiContext) {
		obj := reflect.New(typ)
		errs := decodeBody(c.Request().Request, obj)
		if len(errs) > 0 {
			writeBindingErrors(c.ResponseWriter(), errs)
			return
		}
		c.Map(obj.Elem().Interface())
	}
}

// decodeBody decodes the request body into obj, which must be a pointer to a
// struct, and validates it. It returns all errors found.
func decodeBody(r *http.Request, obj reflect.Value) binding.Errors {
	var errs binding.Errors
	contentType := r.Header.Get("Content-Type")
	switch {
	case strings.Contains(contentType, "form-urlencoded"):
		err := r.ParseForm()
		if err != nil {
			errs.Add([]string{}, binding.ERR_DESERIALIZATION, err.Error())
		}
		errs = mapForm(obj.Elem(), r.Form, errs)

	case strings.Contains(contentType, "multipart/form-data"):
		if r.MultipartForm == nil {
			reader, err := r.MultipartReader()
			if err != nil {
				errs.Add([]string{}, binding.ERR_DESERIALIZATION, err.Error())
				return errs
			}
			form, err := reader.ReadForm(maxMultipartMemory)
			if err != nil {
				errs.Add([]string{}, binding.ERR_DESERIALIZATION, err.Error())
				return errs
			}
			r.MultipartForm = form
		}
		errs = mapForm(obj.Elem(), r.MultipartForm.Value, errs)

	case strings.Contains(contentType, "json"):
		if r.Body != nil {
			err := json.NewDecoder(r.Body).Decode(obj.Interface())
			if err != nil && err != io.EOF {
				errs.Add([]string{}, binding.ERR_DESERIALIZATION, err.Error())
				return errs
			}
		}

	case strings.Contains(contentType, "yaml"):
		if r.Body != nil {
			err := yaml.NewDecoder(r.Body).Decode(obj.Interface())
			if err != nil && err != io.EOF {
				errs.Add([]string{}, binding.ERR_DESERIALIZATION, err.Error())
				return errs
			}
		}

	case contentType == "":
		errs.Add([]string{}, binding.ERR_CONTENT_TYPE, "Empty Content-Type")
		return errs
	default:
		errs.Add([]string{}, binding.ERR_CONTENT_TYPE, "Unsupported Content-Type")
		return errs
	}
	return append(errs, binding.RawValidate(obj.Interface())...)
}

// writeBindingErrors responds with errs in the format of
// github.com/go-macaron/binding.
func writeBindingErrors(w http.ResponseWriter, errs binding.Errors) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	switch {
	case errs.Has(binding.ERR_DESERIALIZATION):
		w.WriteHeader(http.StatusBadRequest)
	case errs.Has(binding.ERR_CONTENT_TYPE):
		w.WriteHeader(http.StatusUnsupportedMediaType)
	default:
		w.WriteHeader(http.StatusUnprocessableEntity)
	}
	p, _ := json.Marshal(errs)
	_, _ = w.Write(p)
}

// mapForm sets fields of the struct from form values, using the snake case of
// the field name as the form key, and returns errs with an error added for each
// value that does not parse.
func mapForm(obj reflect.Value, form url.Values, errs binding.Errors) binding.Errors {
	typ := obj.Type()
	for i := 0; i < typ.NumField(); i++ {
		field := obj.Field(i)
		if typ.Field(i).Type.Kind() == reflect.Struct {
			errs = mapForm(field, form, errs)
		}
		if !field.CanSet() {
			continue
		}

		name := com.ToSnakeCase(typ.Field(i).Name)
		values, ok := form[name]
		if !ok {
			continue
		}

		if field.Kind() == reflect.Slice && len(values) > 0 {
			slice := reflect.MakeSlice(field.Type(), len(values), len(values))
			for j, value := range values {
				errs = setFormValue(slice.Index(j), value, name, errs)
			}
			field.Set(slice)
			continue
		}
		errs = setFormValue(field, values[0], name, errs)
	}
	return errs
}

func setFormValue(field reflect.Value, value, name string, errs binding.Errors) binding.Errors {
	switch field.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if value == "" {
			value = "0"
		}
		v, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			errs.Add([]string{name}, binding.ERR_INTERGER_TYPE, "Value could not be parsed as integer")
		} else {
			field.SetInt(v)
		}

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if value == "" {
			value = "0"
		}
		v, err := strconv.ParseUint(value, 10, 64)
		if err != nil {
			errs.Add([]string{name}, binding.ERR_INTERGER_TYPE, "Value could not be parsed as unsigned integer")
		} else {
			field.SetUint(v)
		}

	case reflect.Bool:
		if value == "on" {
			field.SetBool(true)
			break
		}
		if value == "" {
			value = "false"
		}
		v, err := strconv.ParseBool(value)
		if err != nil {
			errs.Add([]string{name}, binding.ERR_BOOLEAN_TYPE, "Value could not be parsed as boolean")
		} else if v {
			field.SetBool(true)
		}

	case reflect.Float32, reflect.Float64:
		if value == "" {
			value = "0.0"
		}
		bitSize, class := 64, "Value could not be parsed as 64-bit float"
		if field.Kind() == reflect.Float32 {
			bitSize, class = 32, "Value could not be parsed as 32-bit float"
		}
		v, err := strconv.ParseFloat(value, bitSize)
		if err != nil {
			errs.Add([]string{name}, binding.ERR_FLOAT_TYPE, class)
		} else {
			field.SetFloat(v)
		}

	case reflect.String:
		field.SetString(value)
	}
	return errs
}
