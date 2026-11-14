package route

import (
	gocontext "context"

	"github.com/unknwon/paginater"

	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/route/user"
)

func Home(c *context.Context) {
	if c.IsLogged {
		user.Dashboard(c)
		return
	}

	c.ServeWeb()
}

type UserSearchOptions struct {
	Type     database.UserType
	Counter  func(ctx gocontext.Context) int64
	Ranger   func(ctx gocontext.Context, page, pageSize int) ([]*database.User, error)
	PageSize int
	OrderBy  string
	TplName  string
}

func RenderUserSearch(c *context.Context, opts *UserSearchOptions) {
	page := max(c.QueryInt("page"), 1)

	var (
		users []*database.User
		count int64
		err   error
	)

	keyword := c.Query("q")
	if keyword == "" {
		users, err = opts.Ranger(c.Req.Context(), page, opts.PageSize)
		if err != nil {
			c.Error(err, "ranger")
			return
		}
		count = opts.Counter(c.Req.Context())
	} else {
		search := database.Handle.Users().SearchByName
		if opts.Type == database.UserTypeOrganization {
			search = database.Handle.Organizations().SearchByName
		}
		users, count, err = search(c.Req.Context(), keyword, page, opts.PageSize, opts.OrderBy)
		if err != nil {
			c.Error(err, "search by name")
			return
		}
	}
	c.Data["Keyword"] = keyword
	c.Data["Total"] = count
	c.Data["Page"] = paginater.New(int(count), opts.PageSize, page, 5)
	c.Data["Users"] = users

	c.Success(opts.TplName)
}
