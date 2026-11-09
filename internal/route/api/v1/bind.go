package v1

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/flamego/validator"
	"github.com/unknwon/com"
	"gopkg.in/yaml.v3"
)

var (
	alphaDashDotInvalid = regexp.MustCompile(`[^\d\w\-_\.]`)
	// emailPattern is intentionally loose (unanchored) to accept the same input
	// as the API did before migrating to Flamego.
	emailPattern = regexp.MustCompile("[\\w!#$%&'*+/=?^_`{|}~-]+(?:\\.[\\w!#$%&'*+/=?^_`{|}~-]+)*@(?:[\\w](?:[\\w-]*[\\w])?\\.)+[a-zA-Z0-9](?:[\\w-]*[\\w])?")
)

var requestValidator = func() *validator.Validate {
	v := validator.New()
	_ = v.RegisterValidation("alphadashdot", func(fl validator.FieldLevel) bool {
		return !alphaDashDotInvalid.MatchString(fl.Field().String())
	})
	_ = v.RegisterValidation("email", func(fl validator.FieldLevel) bool {
		return emailPattern.MatchString(fl.Field().String())
	})
	return v
}()

// bind returns a handler that decodes the request body into a new instance of
// the model based on the Content-Type of the request, validates it against the
// "validate" struct tags, and maps it for subsequent handlers. It responds with
// 415 for an unsupported Content-Type, 400 for a malformed body, and 422 for a
// body that fails validation.
func bind(model any) func(c *apiContext) {
	typ := reflect.TypeOf(model)
	return func(c *apiContext) {
		obj := reflect.New(typ)
		status, err := decodeBody(c.Request().Request, obj)
		if err != nil {
			c.ErrorStatus(status, err)
			return
		}

		err = requestValidator.Struct(obj.Interface())
		if err != nil {
			c.ErrorStatus(http.StatusUnprocessableEntity, err)
			return
		}
		c.Map(obj.Elem().Interface())
	}
}

// decodeBody decodes the request body into obj, which must be a pointer to a
// struct. It returns the HTTP status code to respond with alongside the error.
func decodeBody(r *http.Request, obj reflect.Value) (int, error) {
	contentType := r.Header.Get("Content-Type")
	switch {
	case strings.Contains(contentType, "form-urlencoded"):
		err := r.ParseForm()
		if err != nil {
			return http.StatusBadRequest, errors.Wrap(err, "parse form")
		}
		return http.StatusUnprocessableEntity, mapForm(obj.Elem(), r.Form)

	case strings.Contains(contentType, "multipart/form-data"):
		if r.MultipartForm == nil {
			err := r.ParseMultipartForm(10 << 20) // 10 MiB
			if err != nil {
				return http.StatusBadRequest, errors.Wrap(err, "parse multipart form")
			}
		}
		return http.StatusUnprocessableEntity, mapForm(obj.Elem(), r.MultipartForm.Value)

	case strings.Contains(contentType, "json"):
		if r.Body == nil {
			return 0, nil
		}
		err := json.NewDecoder(r.Body).Decode(obj.Interface())
		if err != nil && err != io.EOF {
			return http.StatusBadRequest, err
		}
		return 0, nil

	case strings.Contains(contentType, "yaml"):
		if r.Body == nil {
			return 0, nil
		}
		err := yaml.NewDecoder(r.Body).Decode(obj.Interface())
		if err != nil && err != io.EOF {
			return http.StatusBadRequest, err
		}
		return 0, nil

	case contentType == "":
		return http.StatusUnsupportedMediaType, errors.New("Empty Content-Type")
	default:
		return http.StatusUnsupportedMediaType, errors.New("Unsupported Content-Type")
	}
}

// mapForm sets fields of the struct from form values, using the snake case of
// the field name as the form key. Only string, bool and int fields, and slices
// of them are supported.
func mapForm(obj reflect.Value, form url.Values) error {
	typ := obj.Type()
	for i := 0; i < typ.NumField(); i++ {
		name := com.ToSnakeCase(typ.Field(i).Name)
		values, ok := form[name]
		if !ok {
			continue
		}

		field := obj.Field(i)
		if field.Kind() != reflect.Slice {
			err := setFormValue(field, values[0])
			if err != nil {
				return errors.Wrapf(err, "field %q", name)
			}
			continue
		}

		slice := reflect.MakeSlice(field.Type(), len(values), len(values))
		for j, value := range values {
			err := setFormValue(slice.Index(j), value)
			if err != nil {
				return errors.Wrapf(err, "field %q", name)
			}
		}
		field.Set(slice)
	}
	return nil
}

func setFormValue(field reflect.Value, value string) error {
	switch field.Kind() {
	case reflect.String:
		field.SetString(value)

	case reflect.Bool:
		if value == "on" {
			field.SetBool(true)
			return nil
		}
		if value == "" {
			return nil
		}
		v, err := strconv.ParseBool(value)
		if err != nil {
			return err
		}
		field.SetBool(v)

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if value == "" {
			return nil
		}
		v, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return err
		}
		field.SetInt(v)
	}
	return nil
}
