// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package cron

import (
	"fmt"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/setting"
)

var c = New()

func NewCronContext() {
	c.AddFunc("Update mirrors", "@every 1h", models.MirrorUpdate)
	c.AddFunc("Deliver hooks", fmt.Sprintf("@every %dm", setting.WebhookTaskInterval), models.DeliverHooks)
	c.Start()
}

func ListEntries() []*Entry {
	return c.Entries()
}
