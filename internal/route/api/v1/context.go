package v1

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/flamego/flamego"
	"github.com/unknwon/paginater"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/errx"
)

// docURL is the API documentation link included in every error response.
const docURL = "https://github.com/gogs/docs-api"

// RequestInfo describes the request as received by the upstream middleware,
// before its path was normalized for routing.
type RequestInfo struct {
	IsBasicAuth bool
	IsTokenAuth bool
	// Path is the unescaped request path.
	Path string
}

type apiRepository struct {
	AccessMode database.AccessMode
	Owner      *database.User
	Repository *database.Repository
}

// IsOwner returns true if current user is the owner of repository.
func (r *apiRepository) IsOwner() bool {
	return r.AccessMode >= database.AccessModeOwner
}

// IsAdmin returns true if current user has admin or higher access of repository.
func (r *apiRepository) IsAdmin() bool {
	return r.AccessMode >= database.AccessModeAdmin
}

// IsWriter returns true if current user has write or higher access of repository.
func (r *apiRepository) IsWriter() bool {
	return r.AccessMode >= database.AccessModeWrite
}

// HasAccess returns true if the current user has at least read access for this repository
func (r *apiRepository) HasAccess() bool {
	return r.AccessMode >= database.AccessModeRead
}

type apiOrganization struct {
	Organization *database.User
	Team         *database.Team
}

// apiContext is the request context of API v1 handlers.
type apiContext struct {
	flamego.Context

	User        *database.User
	IsLogged    bool
	IsBasicAuth bool
	IsTokenAuth bool

	// BaseURL is the base URL of API v1 endpoints, e.g. https://try.gogs.io/api/v1
	BaseURL string
	// Link is the current request path without the trailing slash.
	Link string

	// path is the unescaped request path.
	path string

	Repo *apiRepository
	Org  *apiOrganization
}

func apiContexter(c flamego.Context, user *database.User, info RequestInfo) {
	c.Map(&apiContext{
		Context:     c,
		User:        user,
		IsLogged:    user != nil,
		IsBasicAuth: info.IsBasicAuth,
		IsTokenAuth: info.IsTokenAuth,
		BaseURL:     conf.Server.ExternalURL + "api/v1",
		Link:        conf.Server.Subpath + strings.TrimSuffix(info.Path, "/"),
		path:        info.Path,
		Repo:        &apiRepository{},
	})
}

// UserID returns ID of current logged in user. It returns 0 if visitor is
// anonymous.
func (c *apiContext) UserID() int64 {
	if !c.IsLogged {
		return 0
	}
	return c.User.ID
}

// Written returns true if the response has been written.
func (c *apiContext) Written() bool {
	return c.ResponseWriter().Written()
}

// Status writes the response header with given status code.
func (c *apiContext) Status(status int) {
	c.ResponseWriter().WriteHeader(status)
}

// JSON renders v as the JSON response with given status code.
func (c *apiContext) JSON(status int, v any) {
	var (
		body []byte
		err  error
	)
	if conf.IsProdMode() {
		body, err = json.Marshal(v)
	} else {
		body, err = json.MarshalIndent(v, "", "  ")
	}
	if err != nil {
		http.Error(c.ResponseWriter(), err.Error(), http.StatusInternalServerError)
		return
	}

	w := c.ResponseWriter()
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

// JSONSuccess renders v as the JSON response with status http.StatusOK.
func (c *apiContext) JSONSuccess(v any) {
	c.JSON(http.StatusOK, v)
}

// PlainText renders msg as the plain text response with given status code.
func (c *apiContext) PlainText(status int, msg string) {
	w := c.ResponseWriter()
	if w.Header().Get("Content-Type") == "" {
		w.Header().Set("Content-Type", "text/plain")
	}
	w.WriteHeader(status)
	_, _ = w.Write([]byte(msg))
}

// NoContent renders the 204 response.
func (c *apiContext) NoContent() {
	c.Status(http.StatusNoContent)
}

// NotFound renders the 404 response.
func (c *apiContext) NotFound() {
	c.Status(http.StatusNotFound)
}

// ErrorStatus renders error with given status code.
func (c *apiContext) ErrorStatus(status int, err error) {
	c.JSON(status, map[string]string{
		"message": err.Error(),
		"url":     docURL,
	})
}

// Error renders the 500 response.
func (c *apiContext) Error(err error, msg string) {
	log.ErrorDepth(4, "%s: %v", msg, err)
	c.ErrorStatus(
		http.StatusInternalServerError,
		errors.New("Something went wrong, please check the server logs for more information."),
	)
}

// Errorf renders the 500 response with formatted message.
func (c *apiContext) Errorf(err error, format string, args ...any) {
	c.Error(err, fmt.Sprintf(format, args...))
}

// NotFoundOrError use error check function to determine if the error
// is about not found. It responses with 404 status code for not found error,
// or error context description for logging purpose of 500 server error.
func (c *apiContext) NotFoundOrError(err error, msg string) {
	if errx.IsNotFound(err) {
		c.NotFound()
		return
	}
	c.Error(err, msg)
}

// SetLinkHeader sets pagination link header by given total number and page size.
func (c *apiContext) SetLinkHeader(total, pageSize int) {
	path := c.path[1:]
	page := paginater.New(total, pageSize, c.QueryInt("page"), 0)
	links := make([]string, 0, 4)
	if page.HasNext() {
		links = append(links, fmt.Sprintf("<%s%s?page=%d>; rel=\"next\"", conf.Server.ExternalURL, path, page.Next()))
	}
	if !page.IsLast() {
		links = append(links, fmt.Sprintf("<%s%s?page=%d>; rel=\"last\"", conf.Server.ExternalURL, path, page.TotalPages()))
	}
	if !page.IsFirst() {
		links = append(links, fmt.Sprintf("<%s%s?page=1>; rel=\"first\"", conf.Server.ExternalURL, path))
	}
	if page.HasPrevious() {
		links = append(links, fmt.Sprintf("<%s%s?page=%d>; rel=\"prev\"", conf.Server.ExternalURL, path, page.Previous()))
	}

	if len(links) > 0 {
		c.ResponseWriter().Header().Set("Link", strings.Join(links, ","))
	}
}
