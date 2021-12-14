// Copyright 2021 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/route/api/v1/convert"
)

func ListTags(c *context.APIContext) {
	tags, err := c.Repo.Repository.GetTags()
	if err != nil {
		c.Error(err, "get tags")
		return
	}

	apiTags := make([]*convert.Tag, len(tags))
	for i := range tags {
		commit, err := tags[i].GetCommit()
		if err != nil {
			c.Error(err, "get commit")
			return
		}
		apiTags[i] = convert.ToTag(tags[i], commit)
	}

	c.JSONSuccess(&apiTags)
}
