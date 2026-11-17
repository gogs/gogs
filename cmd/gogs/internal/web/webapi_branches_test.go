package web

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"gogs.io/gogs/internal/git"
)

func TestBranchesFilter_SelectBranches(t *testing.T) {
	now := time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)
	day := 24 * time.Hour
	branch := func(name, email string, age time.Duration) *git.Branch {
		return &git.Branch{Name: name, Committer: &git.Signature{Email: email, When: now.Add(-age)}}
	}
	// Newest first, as Git lists them.
	branches := []*git.Branch{
		branch("main", "alice@example.com", 0),
		branch("Feature/Search", "Alice@Example.com", day),
		branch("fix-typo", "bob@example.com", 2*day),
		branch("almost-stale", "alice-old@example.com", 30*day-time.Second),
		branch("just-not-active", "alice@example.com", 30*day),
		branch("just-not-stale", "carol@example.com", 90*day),
		branch("release/1.0", "alice-unverified@example.com", 90*day+time.Second),
	}
	numbered := func(n int) []*git.Branch {
		list := make([]*git.Branch, n)
		for i := range list {
			list[i] = branch(fmt.Sprintf("b%02d", i+1), "bob@example.com", time.Duration(i)*time.Hour)
		}
		return list
	}
	names := func(branches []*git.Branch) []string {
		list := []string{}
		for _, b := range branches {
			list = append(list, b.Name)
		}
		return list
	}
	aliceEmails := map[string]bool{"alice@example.com": true, "alice-old@example.com": true}

	tests := []struct {
		name         string
		branches     []*git.Branch
		view         string
		query        string
		viewerEmails map[string]bool
		offset       int
		limit        int
		wantNames    []string
		wantHasMore  bool
	}{
		{
			name:      "all includes the default branch",
			branches:  branches,
			view:      "all",
			limit:     branchesPageSize,
			wantNames: []string{"main", "Feature/Search", "fix-typo", "almost-stale", "just-not-active", "just-not-stale", "release/1.0"},
		},
		{
			name:      "query matches names case-insensitively",
			branches:  branches,
			view:      "all",
			query:     "fEAT",
			limit:     branchesPageSize,
			wantNames: []string{"Feature/Search"},
		},
		{
			name:      "query matches the middle of names with slashes",
			branches:  branches,
			view:      "all",
			query:     "e/1",
			limit:     branchesPageSize,
			wantNames: []string{"release/1.0"},
		},
		{
			name:      "query that matches nothing",
			branches:  branches,
			view:      "all",
			query:     "nope",
			limit:     branchesPageSize,
			wantNames: []string{},
		},
		{
			name:      "default view is only the default branch",
			branches:  branches,
			view:      "default",
			limit:     branchesSectionSize,
			wantNames: []string{"main"},
		},
		{
			name:      "active is updated less than 30 days ago, without the default branch",
			branches:  branches,
			view:      "active",
			limit:     branchesPageSize,
			wantNames: []string{"Feature/Search", "fix-typo", "almost-stale"},
		},
		{
			name:      "stale is not updated for more than 90 days",
			branches:  branches,
			view:      "stale",
			limit:     branchesPageSize,
			wantNames: []string{"release/1.0"},
		},
		{
			name:      "query narrows a view",
			branches:  branches,
			view:      "active",
			query:     "FIX",
			limit:     branchesPageSize,
			wantNames: []string{"fix-typo"},
		},
		{
			name:         "yours matches any activated email in any case, without the default branch",
			branches:     branches,
			view:         "yours",
			viewerEmails: aliceEmails,
			limit:        branchesPageSize,
			wantNames:    []string{"Feature/Search", "almost-stale", "just-not-active"},
		},
		{
			name:         "yours of another user",
			branches:     branches,
			view:         "yours",
			viewerEmails: map[string]bool{"carol@example.com": true},
			limit:        branchesPageSize,
			wantNames:    []string{"just-not-stale"},
		},
		{
			name:      "yours of a signed-out viewer",
			branches:  branches,
			view:      "yours",
			limit:     branchesPageSize,
			wantNames: []string{},
		},
		{
			name:      "unknown view",
			branches:  branches,
			view:      "overview",
			limit:     branchesPageSize,
			wantNames: []string{},
		},
		{
			name:      "exactly a page has no next page",
			branches:  numbered(20),
			view:      "all",
			limit:     branchesPageSize,
			wantNames: names(numbered(20)),
		},
		{
			name:        "one more than a page has a next page",
			branches:    numbered(21),
			view:        "all",
			limit:       branchesPageSize,
			wantNames:   names(numbered(20)),
			wantHasMore: true,
		},
		{
			name:      "second page has the rest",
			branches:  numbered(21),
			view:      "all",
			offset:    branchesPageSize,
			limit:     branchesPageSize,
			wantNames: []string{"b21"},
		},
		{
			name:      "page past the end is empty",
			branches:  numbered(21),
			view:      "all",
			offset:    2 * branchesPageSize,
			limit:     branchesPageSize,
			wantNames: []string{},
		},
		{
			name:        "pages count only matching branches",
			branches:    append(numbered(21), branch("b-extra", "bob@example.com", 0)),
			view:        "all",
			query:       "B0",
			offset:      5,
			limit:       3,
			wantNames:   []string{"b06", "b07", "b08"},
			wantHasMore: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			f := branchesFilter{
				DefaultBranch: "main",
				Query:         test.query,
				Now:           now,
				ViewerEmails:  test.viewerEmails,
			}
			got, hasMore := f.selectBranches(test.branches, test.view, test.offset, test.limit)
			assert.Equal(t, test.wantNames, names(got))
			assert.Equal(t, test.wantHasMore, hasMore)
		})
	}
}
