// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package lfs

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"gopkg.in/macaron.v1"

	"gogs.io/gogs/internal/db"
	"gogs.io/gogs/internal/lfsutil"
)

var _ lfsutil.Storager = (*mockStorage)(nil)

// mockStorage is a in-memory storage for LFS objects.
type mockStorage struct {
	buf *bytes.Buffer
}

func (*mockStorage) Storage() lfsutil.Storage {
	return "memory"
}

func (s *mockStorage) Upload(_ lfsutil.OID, rc io.ReadCloser) (int64, error) {
	defer rc.Close()
	return io.Copy(s.buf, rc)
}

func (s *mockStorage) Download(_ lfsutil.OID, w io.Writer) error {
	_, err := io.Copy(w, s.buf)
	return err
}

func Test_basicHandler_serveDownload(t *testing.T) {
	s := &mockStorage{}
	basic := &basicHandler{
		defaultStorage: s.Storage(),
		storagers: map[lfsutil.Storage]lfsutil.Storager{
			s.Storage(): s,
		},
	}

	m := macaron.New()
	m.Use(macaron.Renderer())
	m.Use(func(c *macaron.Context) {
		c.Map(&db.Repository{Name: "repo"})
		c.Map(lfsutil.OID("ef797c8118f02dfb649607dd5d3f8c7623048c9c063d532cc95c5ed7a898a64f"))
	})
	m.Get("/", basic.serveDownload)

	tests := []struct {
		name          string
		content       string
		mockLFSStore  func() db.LFSStore
		expStatusCode int
		expHeader     http.Header
		expBody       string
	}{
		{
			name: "object does not exist",
			mockLFSStore: func() db.LFSStore {
				mock := NewMockLFSStore()
				mock.GetObjectByOIDFunc.SetDefaultReturn(nil, db.ErrLFSObjectNotExist{})
				return mock
			},
			expStatusCode: http.StatusNotFound,
			expHeader: http.Header{
				"Content-Type": []string{"application/vnd.git-lfs+json"},
			},
			expBody: `{"message":"Object does not exist"}` + "\n",
		},
		{
			name: "storage not found",
			mockLFSStore: func() db.LFSStore {
				mock := NewMockLFSStore()
				mock.GetObjectByOIDFunc.SetDefaultReturn(&db.LFSObject{Storage: "bad_storage"}, nil)
				return mock
			},
			expStatusCode: http.StatusInternalServerError,
			expHeader: http.Header{
				"Content-Type": []string{"application/vnd.git-lfs+json"},
			},
			expBody: `{"message":"Internal server error"}` + "\n",
		},

		{
			name:    "object exists",
			content: "Hello world!",
			mockLFSStore: func() db.LFSStore {
				mock := NewMockLFSStore()
				mock.GetObjectByOIDFunc.SetDefaultReturn(
					&db.LFSObject{
						Size:    12,
						Storage: s.Storage(),
					},
					nil,
				)
				return mock
			},
			expStatusCode: http.StatusOK,
			expHeader: http.Header{
				"Content-Type":   []string{"application/octet-stream"},
				"Content-Length": []string{"12"},
			},
			expBody: "Hello world!",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db.SetMockLFSStore(t, test.mockLFSStore())

			s.buf = bytes.NewBufferString(test.content)

			r, err := http.NewRequest("GET", "/", nil)
			if err != nil {
				t.Fatal(err)
			}

			rr := httptest.NewRecorder()
			m.ServeHTTP(rr, r)

			resp := rr.Result()
			assert.Equal(t, test.expStatusCode, resp.StatusCode)
			assert.Equal(t, test.expHeader, resp.Header)

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatal(err)
			}
			assert.Equal(t, test.expBody, string(body))
		})
	}
}

func Test_basicHandler_serveUpload(t *testing.T) {
	s := &mockStorage{buf: &bytes.Buffer{}}
	basic := &basicHandler{
		defaultStorage: s.Storage(),
		storagers: map[lfsutil.Storage]lfsutil.Storager{
			s.Storage(): s,
		},
	}

	m := macaron.New()
	m.Use(macaron.Renderer())
	m.Use(func(c *macaron.Context) {
		c.Map(&db.Repository{Name: "repo"})
		c.Map(lfsutil.OID("ef797c8118f02dfb649607dd5d3f8c7623048c9c063d532cc95c5ed7a898a64f"))
	})
	m.Put("/", basic.serveUpload)

	tests := []struct {
		name          string
		mockLFSStore  func() db.LFSStore
		expStatusCode int
		expBody       string
	}{
		{
			name: "object already exists",
			mockLFSStore: func() db.LFSStore {
				mock := NewMockLFSStore()
				mock.GetObjectByOIDFunc.SetDefaultReturn(&db.LFSObject{}, nil)
				return mock
			},
			expStatusCode: http.StatusOK,
		},
		{
			name: "new object",
			mockLFSStore: func() db.LFSStore {
				mock := NewMockLFSStore()
				mock.GetObjectByOIDFunc.SetDefaultReturn(nil, db.ErrLFSObjectNotExist{})
				return mock
			},
			expStatusCode: http.StatusOK,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db.SetMockLFSStore(t, test.mockLFSStore())

			r, err := http.NewRequest("PUT", "/", strings.NewReader("Hello world!"))
			if err != nil {
				t.Fatal(err)
			}

			rr := httptest.NewRecorder()
			m.ServeHTTP(rr, r)

			resp := rr.Result()
			assert.Equal(t, test.expStatusCode, resp.StatusCode)

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatal(err)
			}
			assert.Equal(t, test.expBody, string(body))
		})
	}
}

func Test_basicHandler_serveVerify(t *testing.T) {
	m := macaron.New()
	m.Use(macaron.Renderer())
	m.Use(func(c *macaron.Context) {
		c.Map(&db.Repository{Name: "repo"})
	})
	m.Post("/", (&basicHandler{}).serveVerify)

	tests := []struct {
		name          string
		body          string
		mockLFSStore  func() db.LFSStore
		expStatusCode int
		expBody       string
	}{
		{
			name:          "invalid oid",
			body:          `{"oid": "bad_oid"}`,
			expStatusCode: http.StatusBadRequest,
			expBody:       `{"message":"Invalid oid"}` + "\n",
		},
		{
			name: "object does not exist",
			body: `{"oid":"ef797c8118f02dfb649607dd5d3f8c7623048c9c063d532cc95c5ed7a898a64f"}`,
			mockLFSStore: func() db.LFSStore {
				mock := NewMockLFSStore()
				mock.GetObjectByOIDFunc.SetDefaultReturn(nil, db.ErrLFSObjectNotExist{})
				return mock
			},
			expStatusCode: http.StatusNotFound,
			expBody:       `{"message":"Object does not exist"}` + "\n",
		},
		{
			name: "object size mismatch",
			body: `{"oid":"ef797c8118f02dfb649607dd5d3f8c7623048c9c063d532cc95c5ed7a898a64f"}`,
			mockLFSStore: func() db.LFSStore {
				mock := NewMockLFSStore()
				mock.GetObjectByOIDFunc.SetDefaultReturn(&db.LFSObject{Size: 12}, nil)
				return mock
			},
			expStatusCode: http.StatusBadRequest,
			expBody:       `{"message":"Object size mismatch"}` + "\n",
		},

		{
			name: "object exists",
			body: `{"oid":"ef797c8118f02dfb649607dd5d3f8c7623048c9c063d532cc95c5ed7a898a64f", "size":12}`,
			mockLFSStore: func() db.LFSStore {
				mock := NewMockLFSStore()
				mock.GetObjectByOIDFunc.SetDefaultReturn(&db.LFSObject{Size: 12}, nil)
				return mock
			},
			expStatusCode: http.StatusOK,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.mockLFSStore != nil {
				db.SetMockLFSStore(t, test.mockLFSStore())
			}

			r, err := http.NewRequest("POST", "/", strings.NewReader(test.body))
			if err != nil {
				t.Fatal(err)
			}

			rr := httptest.NewRecorder()
			m.ServeHTTP(rr, r)

			resp := rr.Result()
			assert.Equal(t, test.expStatusCode, resp.StatusCode)

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatal(err)
			}
			assert.Equal(t, test.expBody, string(body))
		})
	}
}
