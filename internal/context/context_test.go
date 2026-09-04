package context

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/macaron.v1"
)

func TestContext_JSON(t *testing.T) {
	newContext := func(t *testing.T, handler func(c *Context)) (*httptest.ResponseRecorder, *http.Request) {
		t.Helper()

		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/users/testuser/tokens", nil)

		m := macaron.New()
		m.Use(func(ctx *macaron.Context) {
			handler(&Context{Context: ctx})
		})
		m.ServeHTTP(recorder, req)
		return recorder, req
	}

	t.Run("content type must not have charset parameter", func(t *testing.T) {
		recorder, _ := newContext(t, func(c *Context) {
			c.JSON(http.StatusCreated, map[string]string{
				"name": "ExampleToken",
			})
		})

		// The "charset" parameter is not defined for the "application/json"
		// media type and adding one makes the media type invalid (RFC 8259,
		// IANA Media Types registry).
		require.Equal(t, "application/json", recorder.Header().Get("Content-Type"))
		require.Equal(t, http.StatusCreated, recorder.Code)

		var got map[string]string
		require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &got))
		require.Equal(t, "ExampleToken", got["name"])
	})

	t.Run("JSONSuccess uses status 200", func(t *testing.T) {
		recorder, _ := newContext(t, func(c *Context) {
			c.JSONSuccess(map[string]string{
				"name": "ExampleToken",
			})
		})

		require.Equal(t, "application/json", recorder.Header().Get("Content-Type"))
		require.Equal(t, http.StatusOK, recorder.Code)
	})

	t.Run("indentation follows environment", func(t *testing.T) {
		require.Equal(t, macaron.DEV, macaron.Env)

		recorder, _ := newContext(t, func(c *Context) {
			c.JSON(http.StatusOK, map[string]string{
				"name": "ExampleToken",
			})
		})
		require.Contains(t, recorder.Body.String(), "\n")

		macaron.Env = macaron.PROD
		t.Cleanup(func() { macaron.Env = macaron.DEV })

		recorder, _ = newContext(t, func(c *Context) {
			c.JSON(http.StatusOK, map[string]string{
				"name": "ExampleToken",
			})
		})
		require.NotContains(t, recorder.Body.String(), "\n")
	})
}
