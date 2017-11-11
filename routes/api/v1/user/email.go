// Copyright 2015 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package user

import (
	api "github.com/gogits/go-gogs-client"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/pkg/context"
	"github.com/gogits/gogs/pkg/setting"
	"github.com/gogits/gogs/routes/api/v1/convert"
)

// https://github.com/gogits/go-gogs-client/wiki/Users-Emails#list-email-addresses-for-a-user
func ListEmails(c *context.APIContext) {
	emails, err := models.GetEmailAddresses(c.User.ID)
	if err != nil {
		c.Error(500, "GetEmailAddresses", err)
		return
	}
	apiEmails := make([]*api.Email, len(emails))
	for i := range emails {
		apiEmails[i] = convert.ToEmail(emails[i])
	}
	c.JSON(200, &apiEmails)
}

// https://github.com/gogits/go-gogs-client/wiki/Users-Emails#add-email-addresses
func AddEmail(c *context.APIContext, form api.CreateEmailOption) {
	if len(form.Emails) == 0 {
		c.Status(422)
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
			c.Error(422, "", "Email address has been used: "+err.(models.ErrEmailAlreadyUsed).Email)
		} else {
			c.Error(500, "AddEmailAddresses", err)
		}
		return
	}

	apiEmails := make([]*api.Email, len(emails))
	for i := range emails {
		apiEmails[i] = convert.ToEmail(emails[i])
	}
	c.JSON(201, &apiEmails)
}

// https://github.com/gogits/go-gogs-client/wiki/Users-Emails#delete-email-addresses
func DeleteEmail(c *context.APIContext, form api.CreateEmailOption) {
	if len(form.Emails) == 0 {
		c.Status(204)
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
		c.Error(500, "DeleteEmailAddresses", err)
		return
	}
	c.Status(204)
}
