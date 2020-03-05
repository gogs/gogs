// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package gitutil

import (
	"fmt"

	"github.com/gogs/git-module"
)

type TagsResult struct {
	// Indicates whether results include the latest tag.
	HasLatest bool
	// If results do not include the latest tag, a indicator 'after' to go back.
	PreviousAfter string
	// Indicates whether results include the oldest tag.
	ReachEnd bool
	// List of returned tags.
	Tags []string
}

// ListTagsAfter returns list of tags 'after' (exlusive) given tag.
func ListTagsAfter(repoPath, after string, limit int) (*TagsResult, error) {
	allTags, err := git.RepoTags(repoPath)
	if err != nil {
		return nil, fmt.Errorf("GetTags: %v", err)
	}

	if limit < 0 {
		limit = 0
	}

	numAllTags := len(allTags)
	if len(after) == 0 && limit == 0 {
		return &TagsResult{
			HasLatest: true,
			ReachEnd:  true,
			Tags:      allTags,
		}, nil
	} else if len(after) == 0 && limit > 0 {
		endIdx := limit
		if limit >= numAllTags {
			endIdx = numAllTags
		}
		return &TagsResult{
			HasLatest: true,
			ReachEnd:  limit >= numAllTags,
			Tags:      allTags[:endIdx],
		}, nil
	}

	previousAfter := ""
	hasMatch := false
	tags := make([]string, 0, len(allTags))
	for i := range allTags {
		if hasMatch {
			tags = allTags[i:]
			break
		}
		if allTags[i] == after {
			hasMatch = true
			if limit > 0 && i-limit >= 0 {
				previousAfter = allTags[i-limit]
			}
			continue
		}
	}

	if !hasMatch {
		tags = allTags
	}

	// If all tags after match is equal to the limit, it reaches the oldest tag as well.
	if limit == 0 || len(tags) <= limit {
		return &TagsResult{
			HasLatest:     !hasMatch,
			PreviousAfter: previousAfter,
			ReachEnd:      true,
			Tags:          tags,
		}, nil
	}
	return &TagsResult{
		HasLatest:     !hasMatch,
		PreviousAfter: previousAfter,
		Tags:          tags[:limit],
	}, nil
}
