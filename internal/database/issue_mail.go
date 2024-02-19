// Copyright 2016 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package database

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	"github.com/unknwon/com"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/email"
	"gogs.io/gogs/internal/markup"
	"gogs.io/gogs/internal/userutil"
)

func (issue *Issue) MailSubject() string {
	return fmt.Sprintf("[%s] %s (#%d)", issue.Repo.Name, issue.Title, issue.Index)
}

// mailerUser is a wrapper for satisfying mailer.User interface.
type mailerUser struct {
	user *User
}

func (this mailerUser) ID() int64 {
	return this.user.ID
}

func (this mailerUser) DisplayName() string {
	return this.user.DisplayName()
}

func (this mailerUser) Email() string {
	return this.user.Email
}

func (this mailerUser) GenerateEmailActivateCode(email string) string {
	return userutil.GenerateActivateCode(
		this.user.ID,
		email,
		this.user.Name,
		this.user.Password,
		this.user.Rands,
	)
}

func NewMailerUser(u *User) email.User {
	return mailerUser{u}
}

// mailerRepo is a wrapper for satisfying mailer.Repository interface.
type mailerRepo struct {
	repo *Repository
}

func (this mailerRepo) FullName() string {
	return this.repo.FullName()
}

func (this mailerRepo) HTMLURL() string {
	return this.repo.HTMLURL()
}

func (this mailerRepo) ComposeMetas() map[string]string {
	return this.repo.ComposeMetas()
}

func NewMailerRepo(repo *Repository) email.Repository {
	return mailerRepo{repo}
}

// mailerIssue is a wrapper for satisfying mailer.Issue interface.
type mailerIssue struct {
	issue *Issue
}

func (this mailerIssue) MailSubject() string {
	return this.issue.MailSubject()
}

func (this mailerIssue) Content() string {
	return this.issue.Content
}

func (this mailerIssue) HTMLURL() string {
	return this.issue.HTMLURL()
}

func NewMailerIssue(issue *Issue) email.Issue {
	return mailerIssue{issue}
}

// mailIssueCommentToParticipants can be used for both new issue creation and comment.
// This functions sends two list of emails:
// 1. Repository watchers, users who participated in comments and the assignee.
// 2. Users who are not in 1. but get mentioned in current issue/comment.
func mailIssueCommentToParticipants(issue *Issue, doer *User, mentions []string) error {
	ctx := context.TODO()

	if !conf.User.EnableEmailNotification {
		return nil
	}

	watchers, err := GetWatchers(issue.RepoID)
	if err != nil {
		return fmt.Errorf("GetWatchers [repo_id: %d]: %v", issue.RepoID, err)
	}
	participants, err := GetParticipantsByIssueID(issue.ID)
	if err != nil {
		return fmt.Errorf("GetParticipantsByIssueID [issue_id: %d]: %v", issue.ID, err)
	}

	// In case the issue poster is not watching the repository,
	// even if we have duplicated in watchers, can be safely filtered out.
	if issue.PosterID != doer.ID {
		participants = append(participants, issue.Poster)
	}

	tos := make([]string, 0, len(watchers)) // List of email addresses
	names := make([]string, 0, len(watchers))
	for i := range watchers {
		if watchers[i].UserID == doer.ID {
			continue
		}

		to, err := Users.GetByID(ctx, watchers[i].UserID)
		if err != nil {
			return fmt.Errorf("GetUserByID [%d]: %v", watchers[i].UserID, err)
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
		} else if com.IsSliceContainsStr(names, participants[i].Name) {
			continue
		}

		tos = append(tos, participants[i].Email)
		names = append(names, participants[i].Name)
	}
	if issue.Assignee != nil && issue.Assignee.ID != doer.ID {
		if !com.IsSliceContainsStr(names, issue.Assignee.Name) {
			tos = append(tos, issue.Assignee.Email)
			names = append(names, issue.Assignee.Name)
		}
	}
	email.SendIssueCommentMail(NewMailerIssue(issue), NewMailerRepo(issue.Repo), NewMailerUser(doer), tos)

	// Mail mentioned people and exclude watchers.
	names = append(names, doer.Name)
	toUsernames := make([]string, 0, len(mentions)) // list of user names.
	for i := range mentions {
		if com.IsSliceContainsStr(names, mentions[i]) {
			continue
		}

		toUsernames = append(toUsernames, mentions[i])
	}

	tos, err = Users.GetMailableEmailsByUsernames(ctx, toUsernames)
	if err != nil {
		return errors.Wrap(err, "get mailable emails by usernames")
	}
	email.SendIssueMentionMail(NewMailerIssue(issue), NewMailerRepo(issue.Repo), NewMailerUser(doer), tos)
	return nil
}

// MailParticipants sends new issue thread created emails to repository watchers
// and mentioned people.
func (issue *Issue) MailParticipants() (err error) {
	mentions := markup.FindAllMentions(issue.Content)
	if err = updateIssueMentions(x, issue.ID, mentions); err != nil {
		return fmt.Errorf("UpdateIssueMentions [%d]: %v", issue.ID, err)
	}

	if err = mailIssueCommentToParticipants(issue, issue.Poster, mentions); err != nil {
		log.Error("mailIssueCommentToParticipants: %v", err)
	}

	return nil
}
