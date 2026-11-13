package repo

import (
	gocontext "context"

	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/git"
)

// tryGetUserByEmail returns a non-nil value if the email is corresponding to an
// existing user.
func tryGetUserByEmail(ctx gocontext.Context, email string) *database.User {
	user, _ := database.Handle.Users().GetByEmail(ctx, email)
	return user
}

type userCommit struct {
	User *database.User
	*git.Commit
}

// matchUsersWithCommitEmails matches existing users using commit author emails.
func matchUsersWithCommitEmails(ctx gocontext.Context, oldCommits []*git.Commit) []*userCommit {
	emailToUsers := make(map[string]*database.User)
	newCommits := make([]*userCommit, len(oldCommits))
	usersStore := database.Handle.Users()
	for i := range oldCommits {
		var u *database.User
		if v, ok := emailToUsers[oldCommits[i].Author.Email]; !ok {
			u, _ = usersStore.GetByEmail(ctx, oldCommits[i].Author.Email)
			emailToUsers[oldCommits[i].Author.Email] = u
		} else {
			u = v
		}

		newCommits[i] = &userCommit{
			User:   u,
			Commit: oldCommits[i],
		}
	}
	return newCommits
}
