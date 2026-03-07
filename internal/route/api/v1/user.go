package v1

import (
	"net/http"

	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/route/api/v1/types"
)

func searchUsers(c *context.APIContext) {
	pageSize := c.QueryInt("limit")
	if pageSize <= 0 {
		pageSize = 10
	}
	users, _, err := database.Handle.Users().SearchByName(c.Req.Context(), c.Query("q"), 1, pageSize, "")
	if err != nil {
		c.JSON(http.StatusInternalServerError, map[string]any{
			"ok":    false,
			"error": err.Error(),
		})
		return
	}

	results := make([]*types.User, len(users))
	for i := range users {
		results[i] = toUser(users[i])
		if !c.IsLogged {
			results[i].Email = ""
		}
	}

	c.JSONSuccess(map[string]any{
		"ok":   true,
		"data": results,
	})
}

func getUserProfile(c *context.APIContext) {
	u, err := database.Handle.Users().GetByUsername(c.Req.Context(), c.Params(":username"))
	if err != nil {
		c.NotFoundOrError(err, "get user by name")
		return
	}

	// Hide user e-mail when API caller isn't signed in.
	if !c.IsLogged {
		u.Email = ""
	}
	c.JSONSuccess(toUser(u))
}

func getAuthenticatedUser(c *context.APIContext) {
	c.JSONSuccess(toUser(c.User))
}
