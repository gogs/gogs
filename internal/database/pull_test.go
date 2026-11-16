package database

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"gogs.io/gogs/internal/git"
)

func TestPullRequest_DefaultMergeMessage(t *testing.T) {
	// Newest first, the way the commits of a pull request are listed.
	commits := []*git.Commit{
		{Message: "Change b\n"},
		{Message: "Add b\n"},
		{Message: "Change a\n\nKeep the second line of a.txt.\n"},
	}
	newPull := func(mergeMessage, squashMessage string) *PullRequest {
		return &PullRequest{
			Index:        7,
			HeadUserName: "bob",
			HeadBranch:   "feature",
			Issue:        &Issue{Title: "Add b and update a", Content: "Adds b.txt."},
			BaseRepo:     &Repository{PullsMergeMessage: mergeMessage, PullsSquashMessage: squashMessage},
		}
	}

	tests := []struct {
		name    string
		pull    *PullRequest
		style   MergeStyle
		commits []*git.Commit
		want    MergeMessage
	}{
		{
			name:    "merge commit default",
			pull:    newPull(MergeMessageDefault, ""),
			style:   MergeStyleRegular,
			commits: commits,
			want:    MergeMessage{Title: "Merge pull request #7 from bob/feature", Body: "Add b and update a"},
		},
		{
			name:    "merge commit with title and description",
			pull:    newPull(MergeMessageTitleDescription, ""),
			style:   MergeStyleRegular,
			commits: commits,
			want:    MergeMessage{Title: "Add b and update a (#7)", Body: "Adds b.txt."},
		},
		{
			name:    "squash default lists subjects oldest first",
			pull:    newPull("", MergeMessageDefault),
			style:   MergeStyleSquash,
			commits: commits,
			want:    MergeMessage{Title: "Add b and update a (#7)", Body: "* Change a\n* Add b\n* Change b"},
		},
		{
			name:    "squash default with one commit uses its message",
			pull:    newPull("", MergeMessageDefault),
			style:   MergeStyleSquash,
			commits: commits[2:],
			want:    MergeMessage{Title: "Change a (#7)", Body: "Keep the second line of a.txt."},
		},
		{
			name:    "squash with commit details keeps bodies",
			pull:    newPull("", MergeMessageTitleCommits),
			style:   MergeStyleSquash,
			commits: commits,
			want:    MergeMessage{Title: "Add b and update a (#7)", Body: "* Change a\n\nKeep the second line of a.txt.\n\n* Add b\n\n* Change b"},
		},
		{
			name:    "squash with title only",
			pull:    newPull("", MergeMessageTitle),
			style:   MergeStyleSquash,
			commits: commits,
			want:    MergeMessage{Title: "Add b and update a (#7)"},
		},
		{
			name:    "rebase has no message",
			pull:    newPull("", ""),
			style:   MergeStyleRebase,
			commits: commits,
			want:    MergeMessage{},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, test.pull.DefaultMergeMessage(test.style, test.commits))
		})
	}
}

func TestPullRequest_DefaultMergeMessage_CoAuthors(t *testing.T) {
	bob := &git.Signature{Name: "bob", Email: "bob@example.com"}
	carol := &git.Signature{Name: "Carol", Email: "carol@example.com"}
	// Newest first. bob opened the pull request, so he is not a co-author, and
	// Carol is credited once even though she wrote two commits.
	commits := []*git.Commit{
		{Author: carol, Message: "Change b\n"},
		{Author: bob, Message: "Add b\n\nCo-authored-by: Dave <DAVE@example.com>\nco-authored-by: Carol <carol@example.com>\n"},
		{Author: carol, Message: "Change a\n"},
	}
	pull := &PullRequest{
		Index:    7,
		Issue:    &Issue{Title: "Add b and update a", Poster: &User{ID: 2, Email: "Bob@example.com"}},
		BaseRepo: &Repository{},
	}

	got := pull.DefaultMergeMessage(MergeStyleSquash, commits)
	want := "* Change a\n* Add b\n* Change b\n\nCo-authored-by: Carol <carol@example.com>\nCo-authored-by: Dave <DAVE@example.com>"
	assert.Equal(t, want, got.Body)

	// Trailers copied into the body by a preset move to the end, and each
	// person is credited once.
	movedTests := []struct {
		name          string
		squashMessage string
		content       string
		commits       []*git.Commit
		want          string
	}{
		{
			name:          "commit details",
			squashMessage: MergeMessageTitleCommits,
			commits:       commits,
			want:          "* Change a\n\n* Add b\n\n* Change b\n\nCo-authored-by: Carol <carol@example.com>\nCo-authored-by: Dave <DAVE@example.com>",
		},
		{
			name:          "description",
			squashMessage: MergeMessageTitleDescription,
			content:       "Adds b.txt.\n\nCo-authored-by: Eve <eve@example.com>\nCo-authored-by: Carol <carol@example.com>\n",
			commits:       commits,
			want:          "Adds b.txt.\n\nCo-authored-by: Carol <carol@example.com>\nCo-authored-by: Dave <DAVE@example.com>\nCo-authored-by: Eve <eve@example.com>",
		},
		{
			name:          "one commit with a trailer before more text",
			squashMessage: MergeMessageDefault,
			commits:       []*git.Commit{{Author: bob, Message: "Add b\n\nCo-authored-by: Dave <dave@example.com>\n\nKeep it short.\n"}},
			want:          "Keep it short.\n\nCo-authored-by: Dave <dave@example.com>",
		},
	}
	for _, test := range movedTests {
		t.Run(test.name, func(t *testing.T) {
			pull := &PullRequest{
				Index:    7,
				Issue:    &Issue{Title: "Add b and update a", Content: test.content, Poster: &User{ID: 2, Email: "Bob@example.com"}},
				BaseRepo: &Repository{PullsSquashMessage: test.squashMessage},
			}
			assert.Equal(t, test.want, pull.DefaultMergeMessage(MergeStyleSquash, test.commits).Body)
		})
	}

	// Merge commits keep the authors of the commits, so they credit no one.
	assert.NotContains(t, pull.DefaultMergeMessage(MergeStyleRegular, commits).Body, "Co-authored-by")
}
