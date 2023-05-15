// Copyright 2015 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package user

import (
	"net/http"

	api "github.com/gogs/go-gogs-client"
	"github.com/pkg/errors"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/db"
	"gogs.io/gogs/internal/route/api/v1/convert"
)

func ListEmails(c *context.APIContext) {
	emails, err := db.Users.ListEmails(c.Req.Context(), c.User.ID)
	if err != nil {
		c.Error(err, "get email addresses")
		return
	}
	apiEmails := make([]*api.Email, len(emails))
	for i := range emails {
		apiEmails[i] = convert.ToEmail(emails[i])
	}
	c.JSONSuccess(&apiEmails)
}

func AddEmail(c *context.APIContext, form api.CreateEmailOption) {
	if len(form.Emails) == 0 {
		c.Status(http.StatusUnprocessableEntity)
		return
	}

	apiEmails := make([]*api.Email, 0, len(form.Emails))
	for _, email := range form.Emails {
		err := db.Users.AddEmail(c.Req.Context(), c.User.ID, email, !conf.Auth.RequireEmailConfirmation)
		if err != nil {
			if db.IsErrEmailAlreadyUsed(err) {
				c.ErrorStatus(http.StatusUnprocessableEntity, errors.Errorf("email address has been used: %s", err.(db.ErrEmailAlreadyUsed).Email()))
			} else {
				c.Error(err, "add email addresses")
			}
			return
		}

		apiEmails = append(apiEmails,
			&api.Email{
				Email:    email,
				Verified: !conf.Auth.RequireEmailConfirmation,
			},
		)
	}
	c.JSON(http.StatusCreated, &apiEmails)
}

func DeleteEmail(c *context.APIContext, form api.CreateEmailOption) {
	for _, email := range form.Emails {
		if email == c.User.Email {
			c.ErrorStatus(http.StatusBadRequest, errors.Errorf("cannot delete primary email %q", email))
			return
		}

		err := db.Users.DeleteEmail(c.Req.Context(), c.User.ID, email)
		if err != nil {
			c.Error(err, "delete email addresses")
			return
		}
	}
	c.NoContent()
}
