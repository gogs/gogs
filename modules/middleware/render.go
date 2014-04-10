// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// foked from https://github.com/martini-contrib/render/blob/master/render.go
package middleware

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/go-martini/martini"

	"github.com/gogits/gogs/modules/base"
)

const (
	ContentType    = "Content-Type"
	ContentLength  = "Content-Length"
	ContentJSON    = "application/json"
	ContentHTML    = "text/html"
	ContentXHTML   = "application/xhtml+xml"
	defaultCharset = "UTF-8"
)

var helperFuncs = template.FuncMap{
	"yield": func() (string, error) {
		return "", fmt.Errorf("yield called with no layout defined")
	},
}

type Delims struct {
	Left string

	Right string
}

type RenderOptions struct {
	Directory string

	Layout string

	Extensions []string

	Funcs []template.FuncMap

	Delims Delims

	Charset string

	IndentJSON bool

	HTMLContentType string
}

type HTMLOptions struct {
	Layout string
}

func Renderer(options ...RenderOptions) martini.Handler {
	opt := prepareOptions(options)
	cs := prepareCharset(opt.Charset)
	t := compile(opt)
	return func(res http.ResponseWriter, req *http.Request, c martini.Context) {
		var tc *template.Template
		if martini.Env == martini.Dev {

			tc = compile(opt)
		} else {

			tc, _ = t.Clone()
		}

		rd := &Render{res, req, tc, opt, cs, base.TmplData{}, time.Time{}}

		rd.Data["TmplLoadTimes"] = func() string {
			if rd.startTime.IsZero() {
				return ""
			}
			return fmt.Sprint(time.Since(rd.startTime).Nanoseconds()/1e6) + "ms"
		}

		c.Map(rd.Data)
		c.Map(rd)
	}
}

func prepareCharset(charset string) string {
	if len(charset) != 0 {
		return "; charset=" + charset
	}

	return "; charset=" + defaultCharset
}

func prepareOptions(options []RenderOptions) RenderOptions {
	var opt RenderOptions
	if len(options) > 0 {
		opt = options[0]
	}

	if len(opt.Directory) == 0 {
		opt.Directory = "templates"
	}
	if len(opt.Extensions) == 0 {
		opt.Extensions = []string{".tmpl"}
	}
	if len(opt.HTMLContentType) == 0 {
		opt.HTMLContentType = ContentHTML
	}

	return opt
}

func compile(options RenderOptions) *template.Template {
	dir := options.Directory
	t := template.New(dir)
	t.Delims(options.Delims.Left, options.Delims.Right)

	template.Must(t.Parse("Martini"))

	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		r, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}

		ext := filepath.Ext(r)
		for _, extension := range options.Extensions {
			if ext == extension {

				buf, err := ioutil.ReadFile(path)
				if err != nil {
					panic(err)
				}

				name := (r[0 : len(r)-len(ext)])
				tmpl := t.New(filepath.ToSlash(name))

				for _, funcs := range options.Funcs {
					tmpl = tmpl.Funcs(funcs)
				}

				template.Must(tmpl.Funcs(helperFuncs).Parse(string(buf)))
				break
			}
		}

		return nil
	})

	return t
}

type Render struct {
	http.ResponseWriter
	req             *http.Request
	t               *template.Template
	opt             RenderOptions
	compiledCharset string

	Data base.TmplData

	startTime time.Time
}

func (r *Render) JSON(status int, v interface{}) {
	var result []byte
	var err error
	if r.opt.IndentJSON {
		result, err = json.MarshalIndent(v, "", "  ")
	} else {
		result, err = json.Marshal(v)
	}
	if err != nil {
		http.Error(r, err.Error(), 500)
		return
	}

	r.Header().Set(ContentType, ContentJSON+r.compiledCharset)
	r.WriteHeader(status)
	r.Write(result)
}

func (r *Render) JSONString(v interface{}) (string, error) {
	var result []byte
	var err error
	if r.opt.IndentJSON {
		result, err = json.MarshalIndent(v, "", "  ")
	} else {
		result, err = json.Marshal(v)
	}
	if err != nil {
		return "", err
	}
	return string(result), nil
}

func (r *Render) renderBytes(name string, binding interface{}, htmlOpt ...HTMLOptions) (*bytes.Buffer, error) {
	opt := r.prepareHTMLOptions(htmlOpt)

	if len(opt.Layout) > 0 {
		r.addYield(name, binding)
		name = opt.Layout
	}

	out, err := r.execute(name, binding)
	if err != nil {
		return nil, err
	}

	return out, nil
}

func (r *Render) HTML(status int, name string, binding interface{}, htmlOpt ...HTMLOptions) {
	r.startTime = time.Now()

	out, err := r.renderBytes(name, binding, htmlOpt...)
	if err != nil {
		http.Error(r, err.Error(), http.StatusInternalServerError)
		return
	}

	r.Header().Set(ContentType, r.opt.HTMLContentType+r.compiledCharset)
	r.WriteHeader(status)
	io.Copy(r, out)
}

func (r *Render) HTMLString(name string, binding interface{}, htmlOpt ...HTMLOptions) (string, error) {
	if out, err := r.renderBytes(name, binding, htmlOpt...); err != nil {
		return "", err
	} else {
		return out.String(), nil
	}
}

func (r *Render) Error(status int, message ...string) {
	r.WriteHeader(status)
	if len(message) > 0 {
		r.Write([]byte(message[0]))
	}
}

func (r *Render) Redirect(location string, status ...int) {
	code := http.StatusFound
	if len(status) == 1 {
		code = status[0]
	}

	http.Redirect(r, r.req, location, code)
}

func (r *Render) Template() *template.Template {
	return r.t
}

func (r *Render) execute(name string, binding interface{}) (*bytes.Buffer, error) {
	buf := new(bytes.Buffer)
	return buf, r.t.ExecuteTemplate(buf, name, binding)
}

func (r *Render) addYield(name string, binding interface{}) {
	funcs := template.FuncMap{
		"yield": func() (template.HTML, error) {
			buf, err := r.execute(name, binding)

			return template.HTML(buf.String()), err
		},
	}
	r.t.Funcs(funcs)
}

func (r *Render) prepareHTMLOptions(htmlOpt []HTMLOptions) HTMLOptions {
	if len(htmlOpt) > 0 {
		return htmlOpt[0]
	}

	return HTMLOptions{
		Layout: r.opt.Layout,
	}
}
