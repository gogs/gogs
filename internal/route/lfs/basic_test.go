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
	"github.com/stretchr/testify/require"
	"gopkg.in/macaron.v1"

	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/lfsutil"
)

var _ lfsutil.Storager = (*mockStorage)(nil)

// mockStorage is an in-memory storage for LFS objects.
type mockStorage struct {
	buf *bytes.Buffer
}

func (*mockStorage) Storage() lfsutil.Storage {
	return "memory"
}

func (s *mockStorage) Upload(_ lfsutil.OID, rc io.ReadCloser) (int64, error) {
	defer func() { _ = rc.Close() }()
	return io.Copy(s.buf, rc)
}

func (s *mockStorage) Download(_ lfsutil.OID, w io.Writer) error {
	_, err := io.Copy(w, s.buf)
	return err
}

func TestBasicHandler_serveDownload(t *testing.T) {
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
		c.Map(&database.Repository{Name: "repo"})
		c.Map(lfsutil.OID("ef797c8118f02dfb649607dd5d3f8c7623048c9c063d532cc95c5ed7a898a64f"))
	})
	m.Get("/", basic.serveDownload)

	tests := []struct {
		name          string
		content       string
		mockStore     func() *MockStore
		expStatusCode int
		expHeader     http.Header
		expBody       string
	}{
		{
			name: "object does not exist",
			mockStore: func() *MockStore {
				mockStore := NewMockStore()
				mockStore.GetLFSObjectByOIDFunc.SetDefaultReturn(nil, database.ErrLFSObjectNotExist{})
				return mockStore
			},
			expStatusCode: http.StatusNotFound,
			expHeader: http.Header{
				"Content-Type": []string{"application/vnd.git-lfs+json"},
			},
			expBody: `{"message":"Object does not exist"}` + "\n",
		},
		{
			name: "storage not found",
			mockStore: func() *MockStore {
				mockStore := NewMockStore()
				mockStore.GetLFSObjectByOIDFunc.SetDefaultReturn(&database.LFSObject{Storage: "bad_storage"}, nil)
				return mockStore
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
			mockStore: func() *MockStore {
				mockStore := NewMockStore()
				mockStore.GetLFSObjectByOIDFunc.SetDefaultReturn(
					&database.LFSObject{
						Size:    12,
						Storage: s.Storage(),
					},
					nil,
				)
				return mockStore
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
			basic.store = test.mockStore()

			s.buf = bytes.NewBufferString(test.content)

			r, err := http.NewRequest(http.MethodGet, "/", nil)
			require.NoError(t, err)

			rr := httptest.NewRecorder()
			m.ServeHTTP(rr, r)

			resp := rr.Result()
			assert.Equal(t, test.expStatusCode, resp.StatusCode)
			assert.Equal(t, test.expHeader, resp.Header)

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			assert.Equal(t, test.expBody, string(body))
		})
	}
}

func TestBasicHandler_serveUpload(t *testing.T) {
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
		c.Map(&database.Repository{Name: "repo"})
		c.Map(lfsutil.OID("ef797c8118f02dfb649607dd5d3f8c7623048c9c063d532cc95c5ed7a898a64f"))
	})
	m.Put("/", basic.serveUpload)

	tests := []struct {
		name          string
		mockStore     func() *MockStore
		expStatusCode int
		expBody       string
	}{
		{
			name: "object already exists",
			mockStore: func() *MockStore {
				mockStore := NewMockStore()
				mockStore.GetLFSObjectByOIDFunc.SetDefaultReturn(&database.LFSObject{}, nil)
				return mockStore
			},
			expStatusCode: http.StatusOK,
		},
		{
			name: "new object",
			mockStore: func() *MockStore {
				mockStore := NewMockStore()
				mockStore.GetLFSObjectByOIDFunc.SetDefaultReturn(nil, database.ErrLFSObjectNotExist{})
				return mockStore
			},
			expStatusCode: http.StatusOK,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			basic.store = test.mockStore()

			r, err := http.NewRequest("PUT", "/", strings.NewReader("Hello world!"))
			require.NoError(t, err)

			rr := httptest.NewRecorder()
			m.ServeHTTP(rr, r)

			resp := rr.Result()
			assert.Equal(t, test.expStatusCode, resp.StatusCode)

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			assert.Equal(t, test.expBody, string(body))
		})
	}
}

func TestBasicHandler_serveVerify(t *testing.T) {
	basic := &basicHandler{}

	m := macaron.New()
	m.Use(macaron.Renderer())
	m.Use(func(c *macaron.Context) {
		c.Map(&database.Repository{Name: "repo"})
	})
	m.Post("/", basic.serveVerify)

	tests := []struct {
		name          string
		body          string
		mockStore     func() *MockStore
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
			mockStore: func() *MockStore {
				mockStore := NewMockStore()
				mockStore.GetLFSObjectByOIDFunc.SetDefaultReturn(nil, database.ErrLFSObjectNotExist{})
				return mockStore
			},
			expStatusCode: http.StatusNotFound,
			expBody:       `{"message":"Object does not exist"}` + "\n",
		},
		{
			name: "object size mismatch",
			body: `{"oid":"ef797c8118f02dfb649607dd5d3f8c7623048c9c063d532cc95c5ed7a898a64f"}`,
			mockStore: func() *MockStore {
				mockStore := NewMockStore()
				mockStore.GetLFSObjectByOIDFunc.SetDefaultReturn(&database.LFSObject{Size: 12}, nil)
				return mockStore
			},
			expStatusCode: http.StatusBadRequest,
			expBody:       `{"message":"Object size mismatch"}` + "\n",
		},

		{
			name: "object exists",
			body: `{"oid":"ef797c8118f02dfb649607dd5d3f8c7623048c9c063d532cc95c5ed7a898a64f", "size":12}`,
			mockStore: func() *MockStore {
				mockStore := NewMockStore()
				mockStore.GetLFSObjectByOIDFunc.SetDefaultReturn(&database.LFSObject{Size: 12}, nil)
				return mockStore
			},
			expStatusCode: http.StatusOK,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.mockStore != nil {
				basic.store = test.mockStore()
			}

			r, err := http.NewRequest("POST", "/", strings.NewReader(test.body))
			require.NoError(t, err)

			rr := httptest.NewRecorder()
			m.ServeHTTP(rr, r)

			resp := rr.Result()
			assert.Equal(t, test.expStatusCode, resp.StatusCode)

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			assert.Equal(t, test.expBody, string(body))
		})
	}
}
