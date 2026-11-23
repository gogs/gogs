package v1

import (
	"net/http"

	"gogs.io/gogs/internal/api/v1types"
	"gogs.io/gogs/internal/database"
)

func searchUsers(c *apiContext, links database.LinkOptions) {
	pageSize := c.QueryInt("limit")
	if pageSize <= 0 {
		pageSize = 10
	}
	users, _, err := database.Handle.Users().SearchByName(c.Request().Context(), c.Query("q"), 1, pageSize, "")
	if err != nil {
		c.JSON(http.StatusInternalServerError, map[string]any{
			"ok":    false,
			"error": err.Error(),
		})
		return
	}

	results := make([]*v1types.User, len(users))
	for i := range users {
		results[i] = toUser(links, users[i])
		if !c.IsLogged {
			results[i].Email = ""
		}
	}

	c.JSONSuccess(map[string]any{
		"ok":   true,
		"data": results,
	})
}

func getUserProfile(c *apiContext, links database.LinkOptions) {
	u, err := database.Handle.Users().GetByUsername(c.Request().Context(), c.Param("username"))
	if err != nil {
		c.NotFoundOrError(err, "get user by name")
		return
	}

	// Hide user e-mail when API caller is unauthenticated.
	if !c.IsLogged {
		u.Email = ""
	}
	c.JSONSuccess(toUser(links, u))
}

func getAuthenticatedUser(c *apiContext, links database.LinkOptions) {
	c.JSONSuccess(toUser(links, c.User))
}
