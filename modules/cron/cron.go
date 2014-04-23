// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package cron

import (
	"github.com/robfig/cron"

	"github.com/gogits/gogs/models"
)

func NewCronContext() {
	c := cron.New()
	c.AddFunc("@every 1h", models.MirrorUpdate)
	c.Start()
}
