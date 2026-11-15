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
