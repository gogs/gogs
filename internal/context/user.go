package context

import (
	"github.com/flamego/flamego"

	"gogs.io/gogs/internal/database"
)

// ParamsUser is the wrapper type of the target user defined by URL parameter, namely '<username>'.
type ParamsUser struct {
	*database.User
}

// InjectParamsUser returns a handler that retrieves target user based on URL parameter '<username>',
// and injects it as *ParamsUser.
func InjectParamsUser() flamego.Handler {
	return func(c *Context) {
		user, err := database.Handle.Users().GetByUsername(c.Request.Context(), c.Param("username"))
		if err != nil {
			c.NotFoundOrError(err, "get user by name")
			return
		}
		c.Context.MapTo(&ParamsUser{user}, (*ParamsUser)(nil))
	}
}
