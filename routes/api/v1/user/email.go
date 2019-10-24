// Copyright 2015 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package user

import (
	"net/http"

	api "github.com/gogs/go-gogs-client"

	"gogs.io/gogs/models"
	"gogs.io/gogs/pkg/context"
	"gogs.io/gogs/pkg/setting"
	"gogs.io/gogs/routes/api/v1/convert"
)

func ListEmails(c *context.APIContext) {
	emails, err := models.GetEmailAddresses(c.User.ID)
	if err != nil {
		c.ServerError("GetEmailAddresses", err)
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

	emails := make([]*models.EmailAddress, len(form.Emails))
	for i := range form.Emails {
		emails[i] = &models.EmailAddress{
			UID:         c.User.ID,
			Email:       form.Emails[i],
			IsActivated: !setting.Service.RegisterEmailConfirm,
		}
	}

	if err := models.AddEmailAddresses(emails); err != nil {
		if models.IsErrEmailAlreadyUsed(err) {
			c.Error(http.StatusUnprocessableEntity, "", "email address has been used: "+err.(models.ErrEmailAlreadyUsed).Email)
		} else {
			c.Error(http.StatusInternalServerError, "AddEmailAddresses", err)
		}
		return
	}

	apiEmails := make([]*api.Email, len(emails))
	for i := range emails {
		apiEmails[i] = convert.ToEmail(emails[i])
	}
	c.JSON(http.StatusCreated, &apiEmails)
}

func DeleteEmail(c *context.APIContext, form api.CreateEmailOption) {
	if len(form.Emails) == 0 {
		c.NoContent()
		return
	}

	emails := make([]*models.EmailAddress, len(form.Emails))
	for i := range form.Emails {
		emails[i] = &models.EmailAddress{
			UID:   c.User.ID,
			Email: form.Emails[i],
		}
	}

	if err := models.DeleteEmailAddresses(emails); err != nil {
		c.Error(http.StatusInternalServerError, "DeleteEmailAddresses", err)
		return
	}
	c.NoContent()
}
