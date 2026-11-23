package database

import (
	"context"
	"fmt"
	"maps"

	"github.com/cockroachdb/errors"
	"gorm.io/gorm"
	"unknwon.dev/x/logx"

	"gogs.io/gogs/internal/email"
	"gogs.io/gogs/internal/markup"
	"gogs.io/gogs/internal/strx"
)

// NotifyOptions are the settings that telling people about changes to issues,
// pull requests and their comments reads, i.e., webhooks, mentions and emails.
type NotifyOptions struct {
	Links LinkOptions
	// RepositoryRoot is where repositories are stored, in which emails resolve
	// the commit IDs they mention.
	RepositoryRoot string
	// Markup finds the mentions.
	Markup markup.Options
	// EnableEmailNotification sends emails to watchers, participants and mentioned
	// users.
	EnableEmailNotification bool
	Email                   email.Options
}

func (issue *Issue) MailSubject() string {
	return fmt.Sprintf("[%s] %s (#%d)", issue.Repo.Name, issue.Title, issue.Index)
}

// mailerUser is a wrapper for satisfying mailer.User interface.
type mailerUser struct {
	user *User
}

func (mu mailerUser) ID() int64 {
	return mu.user.ID
}

func (mu mailerUser) DisplayName() string {
	return mu.user.DisplayName()
}

func (mu mailerUser) Email() string {
	return mu.user.Email
}

func NewMailerUser(u *User) email.User {
	return mailerUser{u}
}

// mailerRepo is a wrapper for satisfying mailer.Repository interface.
type mailerRepo struct {
	repo  *Repository
	links LinkOptions
	root  string
}

func (mr mailerRepo) FullName() string {
	return mr.repo.FullName()
}

func (mr mailerRepo) HTMLURL() string {
	return mr.repo.HTMLURL(mr.links.ExternalURL)
}

// ComposeMetas leaves out the repository to resolve commit IDs in when guests
// who cannot read the code take part in its issues, and so receive its mails,
// so that commit links do not reveal which commits it has.
func (mr mailerRepo) ComposeMetas() map[string]string {
	metas := mr.repo.ComposeMetas(mr.links.Subpath, mr.root)
	if !mr.repo.IsPrivate || !mr.repo.CanGuestViewIssues() {
		return metas
	}
	metas = maps.Clone(metas)
	delete(metas, "repoPath")
	return metas
}

// NewMailerRepo returns the repository for emails, whose links are made of
// given settings and whose path is under the root of the repositories.
func NewMailerRepo(links LinkOptions, root string, repo *Repository) email.Repository {
	return mailerRepo{repo: repo, links: links, root: root}
}

// mailerIssue is a wrapper for satisfying mailer.Issue interface.
type mailerIssue struct {
	issue       *Issue
	externalURL string
}

func (mi mailerIssue) MailSubject() string {
	return mi.issue.MailSubject()
}

func (mi mailerIssue) Content() string {
	return mi.issue.Content
}

func (mi mailerIssue) HTMLURL() string {
	return mi.issue.HTMLURL(mi.externalURL)
}

// NewMailerIssue returns the issue for emails, whose link starts with the
// external URL.
func NewMailerIssue(externalURL string, issue *Issue) email.Issue {
	return mailerIssue{issue: issue, externalURL: externalURL}
}

// mailIssueCommentToParticipants can be used for both new issue creation and comment.
// This function sends two lists of emails:
// 1. Repository watchers, users who participated in comments and the assignee.
// 2. Users who are not in 1. but get mentioned in current issue/comment.
func mailIssueCommentToParticipants(logger *logx.Logger, notify NotifyOptions, tx *gorm.DB, issue *Issue, doer *User, mentions []string) error {
	ctx := context.TODO()

	if !notify.EnableEmailNotification {
		return nil
	}

	watchers, err := newReposStore(tx).ListWatches(ctx, issue.RepoID)
	if err != nil {
		return errors.Wrap(err, "list watches")
	}
	participants, err := newIssuesStore(tx).listParticipants(ctx, issue.ID)
	if err != nil {
		return errors.Wrap(err, "list participants")
	}

	// Add the issue poster in case they are not watching the repository.
	// Duplicates of watchers are safely filtered out below.
	if issue.PosterID != doer.ID {
		participants = append(participants, issue.Poster)
	}

	tos := make([]string, 0, len(watchers)) // List of email addresses.
	names := make([]string, 0, len(watchers))
	for i := range watchers {
		if watchers[i].UserID == doer.ID {
			continue
		}

		to, err := Handle.Users().GetByID(ctx, watchers[i].UserID)
		if err != nil {
			return errors.Newf("GetUserByID [%d]: %v", watchers[i].UserID, err)
		}
		if to.IsOrganization() || !to.IsActive {
			continue
		}

		tos = append(tos, to.Email)
		names = append(names, to.Name)
	}
	for i := range participants {
		if participants[i].ID == doer.ID {
			continue
		} else if strx.ContainsFold(names, participants[i].Name) {
			continue
		}

		tos = append(tos, participants[i].Email)
		names = append(names, participants[i].Name)
	}
	if issue.Assignee != nil && issue.Assignee.ID != doer.ID {
		if !strx.ContainsFold(names, issue.Assignee.Name) {
			tos = append(tos, issue.Assignee.Email)
			names = append(names, issue.Assignee.Name)
		}
	}
	mailerIssue := NewMailerIssue(notify.Links.ExternalURL, issue)
	mailerRepo := NewMailerRepo(notify.Links, notify.RepositoryRoot, issue.Repo)
	if err = email.SendIssueCommentMail(logger, notify.Email, mailerIssue, mailerRepo, NewMailerUser(doer), tos); err != nil {
		return errors.Wrap(err, "send issue comment mail")
	}

	// Mail mentioned people and exclude watchers.
	names = append(names, doer.Name)
	toUsernames := make([]string, 0, len(mentions)) // List of user names.
	for i := range mentions {
		if strx.ContainsFold(names, mentions[i]) {
			continue
		}

		toUsernames = append(toUsernames, mentions[i])
	}

	tos, err = Handle.Users().GetMailableEmailsByUsernames(ctx, toUsernames)
	if err != nil {
		return errors.Wrap(err, "get mailable emails by usernames")
	}
	if err = email.SendIssueMentionMail(logger, notify.Email, mailerIssue, mailerRepo, NewMailerUser(doer), tos); err != nil {
		return errors.Wrap(err, "send issue mention mail")
	}
	return nil
}
