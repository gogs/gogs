package user

import (
	"bytes"
	"net/http"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/database"
)

func ShowSSHKeys(c *context.Context, uid int64) {
	keys, err := database.ListPublicKeys(uid)
	if err != nil {
		c.Error(err, "list public keys")
		return
	}

	var buf bytes.Buffer
	for i := range keys {
		buf.WriteString(keys[i].OmitEmail())
		buf.WriteString("\n")
	}
	c.PlainText(http.StatusOK, buf.String())
}

func Email2User(c *context.Context) {
	u, err := database.Handle.Users().GetByEmail(c.Req.Context(), c.Query("email"))
	if err != nil {
		c.NotFoundOrError(err, "get user by email")
		return
	}
	c.Redirect(conf.Server.Subpath + "/user/" + u.Name)
}
