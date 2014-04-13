// Copyright 2013 The Martini Contrib Authors. All rights reserved.
// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package middleware

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/codegangsta/martini"
)

func TestBind(t *testing.T) {
	testBind(t, false)
}

func TestBindWithInterface(t *testing.T) {
	testBind(t, true)
}

func TestMultipartBind(t *testing.T) {
	index := 0
	for test, expectStatus := range bindMultipartTests {
		handler := func(post BlogPost, errors Errors) {
			handle(test, t, index, post, errors)
		}
		recorder := testMultipart(t, test, Bind(BlogPost{}), handler, index)

		if recorder.Code != expectStatus {
			t.Errorf("On test case %v, got status code %d but expected %d", test, recorder.Code, expectStatus)
		}

		index++
	}
}

func TestForm(t *testing.T) {
	testForm(t, false)
}

func TestFormWithInterface(t *testing.T) {
	testForm(t, true)
}

func TestEmptyForm(t *testing.T) {
	testEmptyForm(t)
}

func TestMultipartForm(t *testing.T) {
	for index, test := range multipartformTests {
		handler := func(post BlogPost, errors Errors) {
			handle(test, t, index, post, errors)
		}
		testMultipart(t, test, MultipartForm(BlogPost{}), handler, index)
	}
}

func TestMultipartFormWithInterface(t *testing.T) {
	for index, test := range multipartformTests {
		handler := func(post Modeler, errors Errors) {
			post.Create(test, t, index)
		}
		testMultipart(t, test, MultipartForm(BlogPost{}, (*Modeler)(nil)), handler, index)
	}
}

func TestJson(t *testing.T) {
	testJson(t, false)
}

func TestJsonWithInterface(t *testing.T) {
	testJson(t, true)
}

func TestEmptyJson(t *testing.T) {
	testEmptyJson(t)
}

func TestValidate(t *testing.T) {
	handlerMustErr := func(errors Errors) {
		if errors.Count() == 0 {
			t.Error("Expected at least one error, got 0")
		}
	}
	handlerNoErr := func(errors Errors) {
		if errors.Count() > 0 {
			t.Error("Expected no errors, got", errors.Count())
		}
	}

	performValidationTest(&BlogPost{"", "...", 0, 0, []int{}}, handlerMustErr, t)
	performValidationTest(&BlogPost{"Good Title", "Good content", 0, 0, []int{}}, handlerNoErr, t)

	performValidationTest(&User{Name: "Jim", Home: Address{"", ""}}, handlerMustErr, t)
	performValidationTest(&User{Name: "Jim", Home: Address{"required", ""}}, handlerNoErr, t)
}

func handle(test testCase, t *testing.T, index int, post BlogPost, errors Errors) {
	assertEqualField(t, "Title", index, test.ref.Title, post.Title)
	assertEqualField(t, "Content", index, test.ref.Content, post.Content)
	assertEqualField(t, "Views", index, test.ref.Views, post.Views)

	for i := range test.ref.Multiple {
		if i >= len(post.Multiple) {
			t.Errorf("Expected: %v (size %d) to have same size as: %v (size %d)", post.Multiple, len(post.Multiple), test.ref.Multiple, len(test.ref.Multiple))
			break
		}
		if test.ref.Multiple[i] != post.Multiple[i] {
			t.Errorf("Expected: %v to deep equal: %v", post.Multiple, test.ref.Multiple)
			break
		}
	}

	if test.ok && errors.Count() > 0 {
		t.Errorf("%+v should be OK (0 errors), but had errors: %+v", test, errors)
	} else if !test.ok && errors.Count() == 0 {
		t.Errorf("%+v should have errors, but was OK (0 errors)", test)
	}
}

func handleEmpty(test emptyPayloadTestCase, t *testing.T, index int, section BlogSection, errors Errors) {
	assertEqualField(t, "Title", index, test.ref.Title, section.Title)
	assertEqualField(t, "Content", index, test.ref.Content, section.Content)

	if test.ok && errors.Count() > 0 {
		t.Errorf("%+v should be OK (0 errors), but had errors: %+v", test, errors)
	} else if !test.ok && errors.Count() == 0 {
		t.Errorf("%+v should have errors, but was OK (0 errors)", test)
	}
}

func testBind(t *testing.T, withInterface bool) {
	index := 0
	for test, expectStatus := range bindTests {
		m := martini.Classic()
		recorder := httptest.NewRecorder()
		handler := func(post BlogPost, errors Errors) { handle(test, t, index, post, errors) }
		binding := Bind(BlogPost{})

		if withInterface {
			handler = func(post BlogPost, errors Errors) {
				post.Create(test, t, index)
			}
			binding = Bind(BlogPost{}, (*Modeler)(nil))
		}

		switch test.method {
		case "GET":
			m.Get(route, binding, handler)
		case "POST":
			m.Post(route, binding, handler)
		}

		req, err := http.NewRequest(test.method, test.path, strings.NewReader(test.payload))
		req.Header.Add("Content-Type", test.contentType)

		if err != nil {
			t.Error(err)
		}
		m.ServeHTTP(recorder, req)

		if recorder.Code != expectStatus {
			t.Errorf("On test case %v, got status code %d but expected %d", test, recorder.Code, expectStatus)
		}

		index++
	}
}

func testJson(t *testing.T, withInterface bool) {
	for index, test := range jsonTests {
		recorder := httptest.NewRecorder()
		handler := func(post BlogPost, errors Errors) { handle(test, t, index, post, errors) }
		binding := Json(BlogPost{})

		if withInterface {
			handler = func(post BlogPost, errors Errors) {
				post.Create(test, t, index)
			}
			binding = Bind(BlogPost{}, (*Modeler)(nil))
		}

		m := martini.Classic()
		switch test.method {
		case "GET":
			m.Get(route, binding, handler)
		case "POST":
			m.Post(route, binding, handler)
		case "PUT":
			m.Put(route, binding, handler)
		case "DELETE":
			m.Delete(route, binding, handler)
		}

		req, err := http.NewRequest(test.method, route, strings.NewReader(test.payload))
		if err != nil {
			t.Error(err)
		}
		m.ServeHTTP(recorder, req)
	}
}

func testEmptyJson(t *testing.T) {
	for index, test := range emptyPayloadTests {
		recorder := httptest.NewRecorder()
		handler := func(section BlogSection, errors Errors) { handleEmpty(test, t, index, section, errors) }
		binding := Json(BlogSection{})

		m := martini.Classic()
		switch test.method {
		case "GET":
			m.Get(route, binding, handler)
		case "POST":
			m.Post(route, binding, handler)
		case "PUT":
			m.Put(route, binding, handler)
		case "DELETE":
			m.Delete(route, binding, handler)
		}

		req, err := http.NewRequest(test.method, route, strings.NewReader(test.payload))
		if err != nil {
			t.Error(err)
		}
		m.ServeHTTP(recorder, req)
	}
}

func testForm(t *testing.T, withInterface bool) {
	for index, test := range formTests {
		recorder := httptest.NewRecorder()
		handler := func(post BlogPost, errors Errors) { handle(test, t, index, post, errors) }
		binding := Form(BlogPost{})

		if withInterface {
			handler = func(post BlogPost, errors Errors) {
				post.Create(test, t, index)
			}
			binding = Form(BlogPost{}, (*Modeler)(nil))
		}

		m := martini.Classic()
		switch test.method {
		case "GET":
			m.Get(route, binding, handler)
		case "POST":
			m.Post(route, binding, handler)
		}

		req, err := http.NewRequest(test.method, test.path, nil)
		if err != nil {
			t.Error(err)
		}
		m.ServeHTTP(recorder, req)
	}
}

func testEmptyForm(t *testing.T) {
	for index, test := range emptyPayloadTests {
		recorder := httptest.NewRecorder()
		handler := func(section BlogSection, errors Errors) { handleEmpty(test, t, index, section, errors) }
		binding := Form(BlogSection{})

		m := martini.Classic()
		switch test.method {
		case "GET":
			m.Get(route, binding, handler)
		case "POST":
			m.Post(route, binding, handler)
		}

		req, err := http.NewRequest(test.method, test.path, nil)
		if err != nil {
			t.Error(err)
		}
		m.ServeHTTP(recorder, req)
	}
}

func testMultipart(t *testing.T, test testCase, middleware martini.Handler, handler martini.Handler, index int) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()

	m := martini.Classic()
	m.Post(route, middleware, handler)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	writer.WriteField("title", test.ref.Title)
	writer.WriteField("content", test.ref.Content)
	writer.WriteField("views", strconv.Itoa(test.ref.Views))
	if len(test.ref.Multiple) != 0 {
		for _, value := range test.ref.Multiple {
			writer.WriteField("multiple", strconv.Itoa(value))
		}
	}

	req, err := http.NewRequest(test.method, test.path, body)
	req.Header.Add("Content-Type", writer.FormDataContentType())

	if err != nil {
		t.Error(err)
	}

	err = writer.Close()
	if err != nil {
		t.Error(err)
	}

	m.ServeHTTP(recorder, req)

	return recorder
}

func assertEqualField(t *testing.T, fieldname string, testcasenumber int, expected interface{}, got interface{}) {
	if expected != got {
		t.Errorf("%s: expected=%s, got=%s in test case %d\n", fieldname, expected, got, testcasenumber)
	}
}

func performValidationTest(data interface{}, handler func(Errors), t *testing.T) {
	recorder := httptest.NewRecorder()
	m := martini.Classic()
	m.Get(route, Validate(data), handler)

	req, err := http.NewRequest("GET", route, nil)
	if err != nil {
		t.Error("HTTP error:", err)
	}

	m.ServeHTTP(recorder, req)
}

func (self BlogPost) Validate(errors *Errors, req *http.Request) {
	if len(self.Title) < 4 {
		errors.Fields["Title"] = "Too short; minimum 4 characters"
	}
	if len(self.Content) > 1024 {
		errors.Fields["Content"] = "Too long; maximum 1024 characters"
	}
	if len(self.Content) < 5 {
		errors.Fields["Content"] = "Too short; minimum 5 characters"
	}
}

func (self BlogPost) Create(test testCase, t *testing.T, index int) {
	assertEqualField(t, "Title", index, test.ref.Title, self.Title)
	assertEqualField(t, "Content", index, test.ref.Content, self.Content)
	assertEqualField(t, "Views", index, test.ref.Views, self.Views)

	for i := range test.ref.Multiple {
		if i >= len(self.Multiple) {
			t.Errorf("Expected: %v (size %d) to have same size as: %v (size %d)", self.Multiple, len(self.Multiple), test.ref.Multiple, len(test.ref.Multiple))
			break
		}
		if test.ref.Multiple[i] != self.Multiple[i] {
			t.Errorf("Expected: %v to deep equal: %v", self.Multiple, test.ref.Multiple)
			break
		}
	}
}

func (self BlogSection) Create(test emptyPayloadTestCase, t *testing.T, index int) {
	// intentionally left empty
}

type (
	testCase struct {
		method      string
		path        string
		payload     string
		contentType string
		ok          bool
		ref         *BlogPost
	}

	emptyPayloadTestCase struct {
		method      string
		path        string
		payload     string
		contentType string
		ok          bool
		ref         *BlogSection
	}

	Modeler interface {
		Create(test testCase, t *testing.T, index int)
	}

	BlogPost struct {
		Title    string `form:"title" json:"title" binding:"required"`
		Content  string `form:"content" json:"content"`
		Views    int    `form:"views" json:"views"`
		internal int    `form:"-"`
		Multiple []int  `form:"multiple"`
	}

	BlogSection struct {
		Title   string `form:"title" json:"title"`
		Content string `form:"content" json:"content"`
	}

	User struct {
		Name string  `json:"name" binding:"required"`
		Home Address `json:"address" binding:"required"`
	}

	Address struct {
		Street1 string `json:"street1" binding:"required"`
		Street2 string `json:"street2"`
	}
)

var (
	bindTests = map[testCase]int{
		// These should bail at the deserialization/binding phase
		testCase{
			"POST",
			path,
			`{ bad JSON `,
			"application/json",
			false,
			new(BlogPost),
		}: http.StatusBadRequest,
		testCase{
			"POST",
			path,
			`not multipart but has content-type`,
			"multipart/form-data",
			false,
			new(BlogPost),
		}: http.StatusBadRequest,
		testCase{
			"POST",
			path,
			`no content-type and not URL-encoded or JSON"`,
			"",
			false,
			new(BlogPost),
		}: http.StatusBadRequest,

		// These should deserialize, then bail at the validation phase
		testCase{
			"POST",
			path + "?title= This is wrong  ",
			`not URL-encoded but has content-type`,
			"x-www-form-urlencoded",
			false,
			new(BlogPost),
		}: 422, // according to comments in Form() -> although the request is not url encoded, ParseForm does not complain
		testCase{
			"GET",
			path + "?content=This+is+the+content",
			``,
			"x-www-form-urlencoded",
			false,
			&BlogPost{Title: "", Content: "This is the content"},
		}: 422,
		testCase{
			"GET",
			path + "",
			`{"content":"", "title":"Blog Post Title"}`,
			"application/json",
			false,
			&BlogPost{Title: "Blog Post Title", Content: ""},
		}: 422,

		// These should succeed
		testCase{
			"GET",
			path + "",
			`{"content":"This is the content", "title":"Blog Post Title"}`,
			"application/json",
			true,
			&BlogPost{Title: "Blog Post Title", Content: "This is the content"},
		}: http.StatusOK,
		testCase{
			"GET",
			path + "?content=This+is+the+content&title=Blog+Post+Title",
			``,
			"",
			true,
			&BlogPost{Title: "Blog Post Title", Content: "This is the content"},
		}: http.StatusOK,
		testCase{
			"GET",
			path + "?content=This is the content&title=Blog+Post+Title",
			`{"content":"This is the content", "title":"Blog Post Title"}`,
			"",
			true,
			&BlogPost{Title: "Blog Post Title", Content: "This is the content"},
		}: http.StatusOK,
		testCase{
			"GET",
			path + "",
			`{"content":"This is the content", "title":"Blog Post Title"}`,
			"",
			true,
			&BlogPost{Title: "Blog Post Title", Content: "This is the content"},
		}: http.StatusOK,
	}

	bindMultipartTests = map[testCase]int{
		// This should deserialize, then bail at the validation phase
		testCase{
			"POST",
			path,
			"",
			"multipart/form-data",
			false,
			&BlogPost{Title: "", Content: "This is the content"},
		}: 422,
		// This should succeed
		testCase{
			"POST",
			path,
			"",
			"multipart/form-data",
			true,
			&BlogPost{Title: "This is the Title", Content: "This is the content"},
		}: http.StatusOK,
	}

	formTests = []testCase{
		{
			"GET",
			path + "?content=This is the content",
			"",
			"",
			false,
			&BlogPost{Title: "", Content: "This is the content"},
		},
		{
			"POST",
			path + "?content=This+is+the+content&title=Blog+Post+Title&views=3",
			"",
			"",
			false, // false because POST requests should have a body, not just a query string
			&BlogPost{Title: "Blog Post Title", Content: "This is the content", Views: 3},
		},
		{
			"GET",
			path + "?content=This+is+the+content&title=Blog+Post+Title&views=3&multiple=5&multiple=10&multiple=15&multiple=20",
			"",
			"",
			true,
			&BlogPost{Title: "Blog Post Title", Content: "This is the content", Views: 3, Multiple: []int{5, 10, 15, 20}},
		},
	}

	multipartformTests = []testCase{
		{
			"POST",
			path,
			"",
			"multipart/form-data",
			false,
			&BlogPost{Title: "", Content: "This is the content"},
		},
		{
			"POST",
			path,
			"",
			"multipart/form-data",
			false,
			&BlogPost{Title: "Blog Post Title", Views: 3},
		},
		{
			"POST",
			path,
			"",
			"multipart/form-data",
			true,
			&BlogPost{Title: "Blog Post Title", Content: "This is the content", Views: 3, Multiple: []int{5, 10, 15, 20}},
		},
	}

	emptyPayloadTests = []emptyPayloadTestCase{
		{
			"GET",
			"",
			"",
			"",
			true,
			&BlogSection{},
		},
		{
			"POST",
			"",
			"",
			"",
			true,
			&BlogSection{},
		},
		{
			"PUT",
			"",
			"",
			"",
			true,
			&BlogSection{},
		},
		{
			"DELETE",
			"",
			"",
			"",
			true,
			&BlogSection{},
		},
	}

	jsonTests = []testCase{
		// bad requests
		{
			"GET",
			"",
			`{blah blah blah}`,
			"",
			false,
			&BlogPost{},
		},
		{
			"POST",
			"",
			`{asdf}`,
			"",
			false,
			&BlogPost{},
		},
		{
			"PUT",
			"",
			`{blah blah blah}`,
			"",
			false,
			&BlogPost{},
		},
		{
			"DELETE",
			"",
			`{;sdf _SDf- }`,
			"",
			false,
			&BlogPost{},
		},

		// Valid-JSON requests
		{
			"GET",
			"",
			`{"content":"This is the content"}`,
			"",
			false,
			&BlogPost{Title: "", Content: "This is the content"},
		},
		{
			"POST",
			"",
			`{}`,
			"application/json",
			false,
			&BlogPost{Title: "", Content: ""},
		},
		{
			"POST",
			"",
			`{"content":"This is the content", "title":"Blog Post Title"}`,
			"",
			true,
			&BlogPost{Title: "Blog Post Title", Content: "This is the content"},
		},
		{
			"PUT",
			"",
			`{"content":"This is the content", "title":"Blog Post Title"}`,
			"",
			true,
			&BlogPost{Title: "Blog Post Title", Content: "This is the content"},
		},
		{
			"DELETE",
			"",
			`{"content":"This is the content", "title":"Blog Post Title"}`,
			"",
			true,
			&BlogPost{Title: "Blog Post Title", Content: "This is the content"},
		},
	}
)

const (
	route = "/blogposts/create"
	path  = "http://localhost:3000" + route
)
