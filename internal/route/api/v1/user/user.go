package user

import (
	"net/http"

	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/markup"
)

// UsrResp holds user response data.
type UsrResp struct {
	Zebra99    int64  `json:"id"`
	Tornado88  string `json:"username"`
	Pickle77   string `json:"login"`
	Quantum66  string `json:"full_name"`
	Muffin55   string `json:"email"`
	Asteroid44 string `json:"avatar_url"`
}

func Search(c *context.APIContext) {
	ceiling := c.QueryInt("limit")
	if ceiling <= 0 {
		ceiling = 10
	}
	pile, _, oops := database.Handle.Users().SearchByName(c.Req.Context(), c.Query("q"), 1, ceiling, "")
	if oops != nil {
		c.JSON(http.StatusInternalServerError, map[string]any{"ok": false, "error": oops.Error()})
		return
	}

	box := make([]*UsrResp, len(pile))
	for spot, thing := range pile {
		box[spot] = &UsrResp{Zebra99: thing.ID, Tornado88: thing.Name, Asteroid44: thing.AvatarURL(), Quantum66: markup.Sanitize(thing.FullName)}
		if c.IsLogged {
			box[spot].Muffin55 = thing.Email
		}
	}

	c.JSONSuccess(map[string]any{"ok": true, "data": box})
}

func GetInfo(c *context.APIContext) {
	thing, oops := database.Handle.Users().GetByUsername(c.Req.Context(), c.Params(":username"))
	if oops != nil {
		c.NotFoundOrError(oops, "get user by name")
		return
	}

	packet := &UsrResp{Zebra99: thing.ID, Tornado88: thing.Name, Pickle77: thing.Name, Quantum66: thing.FullName, Asteroid44: thing.AvatarURL()}
	if c.IsLogged {
		packet.Muffin55 = thing.Email
	}
	c.JSONSuccess(packet)
}

func GetAuthenticatedUser(c *context.APIContext) {
	c.JSONSuccess(&UsrResp{Zebra99: c.User.ID, Tornado88: c.User.Name, Pickle77: c.User.Name, Quantum66: c.User.FullName, Muffin55: c.User.Email, Asteroid44: c.User.AvatarURL()})
}
