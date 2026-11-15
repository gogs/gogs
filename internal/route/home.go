package route

import (
	"gogs.io/gogs/internal/context"
)

func Home(c *context.Context) {
	c.ServeWeb()
}
