package web

import (
	stdctx "context"
	"encoding/json"
	"net/http"
	"reflect"
	"regexp"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/flamego/binding"
	"github.com/flamego/flamego"
	"github.com/flamego/session"
	"github.com/flamego/validator"
	"github.com/go-macaron/i18n"
	macaronsession "github.com/go-macaron/session"
	"gopkg.in/macaron.v1"

	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/database"
)

type (
	webAPIUserKey    struct{}
	webAPISessionKey struct{}
	webAPIMacaronKey struct{}
	webAPILocaleKey  struct{}
)

func flamegoBridger(webHandler http.Handler) func(c *context.Context, l i18n.Locale) {
	return func(c *context.Context, l i18n.Locale) {
		ctx := c.Req.Context()
		ctx = stdctx.WithValue(ctx, webAPIUserKey{}, c.User)
		ctx = stdctx.WithValue(ctx, webAPISessionKey{}, c.Session)
		ctx = stdctx.WithValue(ctx, webAPIMacaronKey{}, c.Context)
		ctx = stdctx.WithValue(ctx, webAPILocaleKey{}, l)
		webHandler.ServeHTTP(c.Resp, c.Req.WithContext(ctx))
	}
}

func flamegoInjector(c flamego.Context) {
	ctx := c.Request().Context()
	user, _ := ctx.Value(webAPIUserKey{}).(*database.User)
	sess, _ := ctx.Value(webAPISessionKey{}).(macaronsession.Store)
	mc, _ := ctx.Value(webAPIMacaronKey{}).(*macaron.Context)
	l, _ := ctx.Value(webAPILocaleKey{}).(i18n.Locale)
	c.Map(user, sess, mc, l)
	c.MapTo(&flamegoSessionAdapter{sess: sess}, (*session.Session)(nil))
}

// flamegoSessionAdapter exposes the underlying Macaron session via the Flamego
// session interface so the captcha middleware (and any future Flamego-native
// session consumer) can read/write the same session store the rest of the app
// uses.
type flamegoSessionAdapter struct {
	sess    macaronsession.Store
	rotated macaronsession.RawStore
}

func (s *flamegoSessionAdapter) active() macaronsession.RawStore {
	if s.rotated != nil {
		return s.rotated
	}
	return s.sess
}

func (s *flamegoSessionAdapter) ID() string                      { return s.active().ID() }
func (s *flamegoSessionAdapter) Get(key interface{}) interface{} { return s.active().Get(key) }
func (s *flamegoSessionAdapter) Set(key, val interface{})        { _ = s.active().Set(key, val) }
func (s *flamegoSessionAdapter) SetFlash(val interface{})        { _ = s.active().Set("_flash", val) }
func (s *flamegoSessionAdapter) Delete(key interface{})          { _ = s.active().Delete(key) }
func (s *flamegoSessionAdapter) Flush()                          { _ = s.active().Flush() }
func (s *flamegoSessionAdapter) Encode() ([]byte, error)         { return nil, nil }

func (s *flamegoSessionAdapter) RegenerateID(mc *macaron.Context) error {
	raw, err := s.sess.RegenerateId(mc)
	if err != nil {
		return errors.Wrap(err, "regenerate session ID")
	}
	s.rotated = raw
	return nil
}

func (s *flamegoSessionAdapter) releaseRotated() error {
	if s.rotated == nil {
		return nil
	}
	if err := s.rotated.Release(); err != nil {
		return errors.Wrap(err, "release rotated session")
	}
	return nil
}

func enforceWebAPIMaxBodySize(c flamego.Context) {
	r := c.Request().Request
	r.Body = http.MaxBytesReader(c.ResponseWriter(), r.Body, 4*1024) // 4 KiB
}

// webAPIValidator is the shared validator instance used by every webapi
// binding. Registering the json-tag name function makes validation errors
// carry the wire field name (e.g. "recoveryCode") via ve.Field(), so the
// 400 payload keys match what the React client sends and reads.
var webAPIValidator = func() *validator.Validate {
	v := validator.New()
	v.RegisterTagNameFunc(func(fld reflect.StructField) string {
		name := strings.SplitN(fld.Tag.Get("json"), ",", 2)[0]
		if name == "-" {
			return ""
		}
		return name
	})
	_ = v.RegisterValidation("alphadashdot", func(fl validator.FieldLevel) bool {
		return !alphaDashDotInvalid.MatchString(fl.Field().String())
	})
	return v
}()

var alphaDashDotInvalid = regexp.MustCompile(`[^\d\w\-_\.]`)

// bindJSON binds the request body to T. On binding or validation failure it
// short-circuits with a 400 carrying the standard renderBindingErrors payload,
// so downstream handlers can drop the `if len(bindErrs) > 0` boilerplate and
// the binding.Errors parameter entirely.
func bindJSON(model any) flamego.Handler {
	return binding.JSON(model, binding.Options{
		Validator: webAPIValidator,
		ErrorHandler: func(c flamego.Context, l i18n.Locale, errs binding.Errors) {
			w := c.ResponseWriter()
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(renderBindingErrors(l, errs))
		},
	})
}

func mountWebAPIRoutes(f *flamego.Flame) {
	f.Group("/api/web", func() {
		f.Group("/user", func() {
			f.Get("/info", getUserInfo)
			f.Combo("/sign-up").
				Get(getUserSignUp).
				Post(bindJSON(userSignUpRequest{}), postUserSignUp)
			f.Group("/reset-password", func() {
				f.Combo("").
					Get(getUserResetPassword).
					Post(bindJSON(userResetPasswordEmailRequest{}), postUserResetPassword)
				f.Post("/complete", bindJSON(userResetPasswordCompleteRequest{}), postUserResetPasswordComplete)
			})
			f.Combo("/sign-in").
				Get(getUserSignIn).
				Post(bindJSON(userSignInRequest{}), postUserSignIn)
			f.Group("/mfa", func() {
				f.Combo("").
					Get(getUserMFA).
					Post(bindJSON(userMFARequest{}), postUserMFA)
				f.Post("/recovery", bindJSON(userMFARecoveryRequest{}), postUserMFARecovery)
			})
			f.Group("/activate", func() {
				f.Combo("").
					Get(getUserActivate).
					Post(postUserActivate)
				f.Post("/complete", bindJSON(userActivateCompleteRequest{}), postUserActivateComplete)
			})
			f.Post("/sign-out", postUserSignOut)
		})
		f.Group("/{owner}/{repo}", func() {
			f.Get("/header", getRepoHeader)
			f.Get("/commit/{sha: /[0-9a-f]{7,40}/}", getRepoCommit)
			f.Combo("/watch").Post(postRepoWatch).Delete(deleteRepoWatch)
			f.Combo("/star").Post(postRepoStar).Delete(deleteRepoStar)
		}, withRepoContext)
	}, enforceWebAPIMaxBodySize)
}

// fieldErrors maps JSON field names to per-field localized messages. A non-nil
// value renders inline under the input. A nil value marks the input as
// invalid (highlight + focus eligibility) without duplicating text. Used in
// concert with bindingErrorResponse.Error to surface one banner message while
// highlighting multiple inputs.
type fieldErrors map[string]*string

// bindingErrorResponse carries form-validation failures. Error is the top-level
// message shown as a banner above the form (used when the failure is not tied
// to a specific input, e.g. malformed body, bad credentials).
type bindingErrorResponse struct {
	Error  string      `json:"error,omitempty"`
	Fields fieldErrors `json:"fields,omitempty"`
}

// ruleSuffixKeys maps a validator tag to the shared "form.*_error" suffix key
// (e.g. "max" -> "form.max_size_error"). Messages are composed as
// <field label> + <suffix>, mirroring the legacy Macaron binding behavior.
var ruleSuffixKeys = map[string]string{
	"required":     "form.require_error",
	"max":          "form.max_size_error",
	"min":          "form.min_size_error",
	"len":          "form.size_error",
	"email":        "form.email_error",
	"url":          "form.url_error",
	"alphadashdot": "form.alpha_dash_dot_error",
}

// renderBindingErrors maps binding.Errors to the response shape, looking up
// localized messages via the request's locale. The per-field label comes from
// "form.<StructField>" (e.g. "form.UserName"); the rule suffix comes from
// ruleSuffixKeys. Rule parameters (e.g. "254" for `max=254`) are passed
// through to the suffix translation for %s expansion. Always HTTP 400.
func renderBindingErrors(l i18n.Locale, errs binding.Errors) *bindingErrorResponse {
	for _, e := range errs {
		if e.Category == binding.ErrorCategoryDeserialization {
			return &bindingErrorResponse{Error: l.Tr("form.invalid_request") + ": " + e.Err.Error()}
		}
	}

	out := make(fieldErrors)
	for _, e := range errs {
		var ves validator.ValidationErrors
		ok := errors.As(e.Err, &ves)
		if !ok {
			continue
		}
		for _, ve := range ves {
			field := ve.Field()
			if _, exists := out[field]; exists {
				// Keep the first rule that failed for a given field so the client renders one
				// message per input. Subsequent rules surface only after the first is fixed.
				continue
			}
			label := l.Tr("form." + ve.StructField())
			suffixKey, known := ruleSuffixKeys[ve.Tag()]
			var msg string
			switch {
			case !known:
				msg = l.Tr("form.unknown_error") + " " + ve.Tag()
			case ve.Param() != "":
				msg = label + l.Tr(suffixKey, ve.Param())
			default:
				msg = label + l.Tr(suffixKey)
			}
			out[field] = &msg
		}
	}
	return &bindingErrorResponse{Fields: out}
}
