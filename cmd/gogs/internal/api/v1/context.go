package v1

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/flamego/flamego"
	"github.com/unknwon/paginater"
	"unknwon.dev/x/logx"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/errx"
	"gogs.io/gogs/internal/telemetry"
)

// docURL is the API documentation link included in every error response.
const docURL = "https://github.com/gogs/docs-api"

// RequestInfo describes the request as received by the upstream middleware,
// before its path was normalized for routing.
type RequestInfo struct {
	IsBasicAuth bool
	IsTokenAuth bool
	// AccessTokenID is the ID of the access token the request authenticated
	// with, 0 without token authentication.
	AccessTokenID int64
	// Path is the unescaped request path.
	Path string
	// WebNotFound and WebError respond with the 404 and 500 responses of web
	// pages. Endpoints that responded through the web pages before migrating to
	// Flamego, e.g., unknown routes, keep doing so.
	WebNotFound func()
	WebError    func(err error, msg string)
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

// HasAccess returns true if the current user has at least read access for this repository.
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

	Logger *logx.Logger

	User        *database.User
	IsLogged    bool
	IsBasicAuth bool
	IsTokenAuth bool
	// AccessTokenID is the ID of the access token the request authenticated
	// with, 0 without token authentication.
	AccessTokenID int64

	// BaseURL is the base URL of API v1 endpoints, e.g., https://try.gogs.io/api/v1.
	BaseURL string
	// Link is the current request path without the trailing slash.
	Link string

	// path is the unescaped request path.
	path string
	// externalURL is the external URL of the server, ending with "/".
	externalURL string
	// indentJSON is whether JSON responses are indented for reading, i.e.,
	// outside the production mode.
	indentJSON bool

	webNotFound func()
	webError    func(err error, msg string)

	Repo *apiRepository
	Org  *apiOrganization
}

func apiContexter(c flamego.Context, logger *logx.Logger, app conf.AppOpts, server conf.ServerOpts, viewer *database.User, info RequestInfo) {
	c.Map(&apiContext{
		Context:       c,
		Logger:        logger,
		User:          viewer,
		IsLogged:      viewer != nil,
		IsBasicAuth:   info.IsBasicAuth,
		IsTokenAuth:   info.IsTokenAuth,
		AccessTokenID: info.AccessTokenID,
		BaseURL:       server.ExternalURL + "api/v1",
		Link:          server.Subpath + strings.TrimSuffix(info.Path, "/"),
		path:          info.Path,
		externalURL:   server.ExternalURL,
		indentJSON:    !app.IsProdMode(),
		webNotFound:   info.WebNotFound,
		webError:      info.WebError,
		Repo:          &apiRepository{},
	})
}

// UserID returns ID of authenticated viewer. It returns 0 if visitor is
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
	if c.indentJSON {
		body, err = json.MarshalIndent(v, "", "  ")
	} else {
		body, err = json.Marshal(v)
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
	c.Logger.Error("Internal server error", "method", c.Request().Method, "path", c.path, "operation", msg, "error", err)
	telemetry.RecordFailure(c.Request().Context(), errors.Wrap(err, msg))
	c.ErrorStatus(
		http.StatusInternalServerError,
		errors.New("Something went wrong, please check the server logs for more information."),
	)
}

// Errorf renders the 500 response with formatted message.
func (c *apiContext) Errorf(err error, format string, args ...any) {
	c.Error(err, fmt.Sprintf(format, args...))
}

// NotFoundOrError responds with 404 when the error satisfies errx.IsNotFound.
// Otherwise, it responds with 500 and logs the error with msg as its context.
func (c *apiContext) NotFoundOrError(err error, msg string) {
	if errx.IsNotFound(err) {
		c.NotFound()
		return
	}
	c.Error(err, msg)
}

// webNotFoundOrError is like NotFoundOrError but responds with the 404 and 500
// responses of web pages.
func (c *apiContext) webNotFoundOrError(err error, msg string) {
	if errx.IsNotFound(err) {
		c.webNotFound()
		return
	}
	c.webError(err, msg)
}

// queryPage returns the requested page, at least 1. It caps the page so that
// page*pageSize, the end of the page, fits in an int. An offset that overflows
// to a negative number is silently dropped by the database, which returns the
// first page instead.
func (c *apiContext) queryPage(pageSize int) int {
	return min(max(c.QueryInt("page"), 1), math.MaxInt/max(pageSize, 1))
}

// SetLinkHeader sets pagination link header by given total number and page size.
func (c *apiContext) SetLinkHeader(total, pageSize int) {
	// The paginater clamps the page to the last one, but the list it describes
	// does not, e.g., page 10 of 3 pages is empty and comes after page 3.
	current := c.queryPage(pageSize)
	page := paginater.New(total, pageSize, current, 0)
	// Links keep the other query parameters, e.g., the search keyword and the
	// page size, so that every page belongs to the same list.
	query := c.Request().URL.Query()
	link := func(n int, rel string) string {
		query.Set("page", strconv.Itoa(n))
		return fmt.Sprintf("<%s%s?%s>; rel=%q", c.externalURL, c.path[1:], query.Encode(), rel)
	}
	links := make([]string, 0, 4)
	if page.HasNext() {
		links = append(links, link(page.Next(), "next"))
	}
	if current != page.TotalPages() {
		links = append(links, link(page.TotalPages(), "last"))
	}
	if current > 1 {
		links = append(links, link(1, "first"))
		links = append(links, link(min(current-1, page.TotalPages()), "prev"))
	}

	if len(links) > 0 {
		c.ResponseWriter().Header().Set("Link", strings.Join(links, ","))
	}
}
