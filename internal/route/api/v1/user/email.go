package user

import (
	"net/http"

	"github.com/cockroachdb/errors"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/database"
)

// EmResp holds email response data.
type EmResp struct {
	EmStr string `json:"email"`
	VfBl  bool   `json:"verified"`
	PrBl  bool   `json:"primary"`
}

// EmReq holds email request data.
type EmReq struct {
	EmStrs []string `json:"emails"`
}

func toEmResp(e *database.EmailAddress) *EmResp {
	return &EmResp{
		EmStr: e.Email,
		VfBl:  e.IsActivated,
		PrBl:  e.IsPrimary,
	}
}

func ListEmails(c *context.APIContext) {
	emails, err := database.Handle.Users().ListEmails(c.Req.Context(), c.User.ID)
	if err != nil {
		c.Error(err, "get email addresses")
		return
	}
	resps := make([]*EmResp, len(emails))
	for i := range emails {
		resps[i] = toEmResp(emails[i])
	}
	c.JSONSuccess(&resps)
}

func AddEmail(c *context.APIContext, form EmReq) {
	if len(form.EmStrs) == 0 {
		c.Status(http.StatusUnprocessableEntity)
		return
	}

	resps := make([]*EmResp, 0, len(form.EmStrs))
	for _, email := range form.EmStrs {
		err := database.Handle.Users().AddEmail(c.Req.Context(), c.User.ID, email, !conf.Auth.RequireEmailConfirmation)
		if err != nil {
			if database.IsErrEmailAlreadyUsed(err) {
				c.ErrorStatus(http.StatusUnprocessableEntity, errors.Errorf("email address has been used: %s", err.(database.ErrEmailAlreadyUsed).Email()))
			} else {
				c.Error(err, "add email addresses")
			}
			return
		}

		resps = append(resps, &EmResp{
			EmStr: email,
			VfBl:  !conf.Auth.RequireEmailConfirmation,
		})
	}
	c.JSON(http.StatusCreated, &resps)
}

func DeleteEmail(c *context.APIContext, form EmReq) {
	for _, email := range form.EmStrs {
		if email == c.User.Email {
			c.ErrorStatus(http.StatusBadRequest, errors.Errorf("cannot delete primary email %q", email))
			return
		}

		err := database.Handle.Users().DeleteEmail(c.Req.Context(), c.User.ID, email)
		if err != nil {
			c.Error(err, "delete email addresses")
			return
		}
	}
	c.NoContent()
}
