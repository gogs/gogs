// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package gitutil

import (
	"github.com/pkg/errors"
)

// TagsPage contains a list of tags and pagination information.
type TagsPage struct {
	// List of tags in the current page.
	Tags []string
	// Whether the results include the latest tag.
	HasLatest bool
	// When results do not include the latest tag, an indicator of 'after' to go back.
	PreviousAfter string
	// Whether there are more tags in the next page.
	HasNext bool
}

func (module) ListTagsAfter(repoPath, after string, limit int) (*TagsPage, error) {
	all, err := Module.RepoTags(repoPath)
	if err != nil {
		return nil, errors.Wrap(err, "get tags")
	}
	total := len(all)

	if limit < 0 {
		limit = 0
	}

	// Returns everything when no filter and no limit
	if after == "" && limit == 0 {
		return &TagsPage{
			Tags:      all,
			HasLatest: true,
		}, nil
	}

	// No filter but has a limit, returns first X tags
	if after == "" && limit > 0 {
		endIdx := limit
		if limit > total {
			endIdx = total
		}
		return &TagsPage{
			Tags:      all[:endIdx],
			HasLatest: true,
			HasNext:   limit < total,
		}, nil
	}

	// Loop over all tags see if we can find the filter
	previousAfter := ""
	found := false
	tags := make([]string, 0, len(all))
	for i := range all {
		if all[i] != after {
			continue
		}

		found = true
		if limit > 0 && i-limit >= 0 {
			previousAfter = all[i-limit]
		}

		// In case filter is the oldest one
		if i+1 < total {
			tags = all[i+1:]
		}
		break
	}

	if !found {
		tags = all
	}

	// If all tags after match is equal to the limit, it reaches the oldest tag as well.
	if limit == 0 || len(tags) <= limit {
		return &TagsPage{
			Tags:          tags,
			HasLatest:     !found,
			PreviousAfter: previousAfter,
		}, nil
	}

	return &TagsPage{
		Tags:          tags[:limit],
		HasLatest:     !found,
		PreviousAfter: previousAfter,
		HasNext:       true,
	}, nil
}
