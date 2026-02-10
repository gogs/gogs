package v1

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/gogs/git-module"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/markup"
	"gogs.io/gogs/internal/route/api/v1/types"
)

// toAllowedPageSize makes sure page size is in allowed range.
func toAllowedPageSize(size int) int {
	if size <= 0 {
		size = 10
	} else if size > conf.API.MaxResponseItems {
		size = conf.API.MaxResponseItems
	}
	return size
}

// toUser converts a database user to an API user with the full name sanitized
// for safe HTML rendering.
func toUser(u *database.User) *types.User {
	return &types.User{
		ID:        u.ID,
		UserName:  u.Name,
		Login:     u.Name,
		FullName:  markup.Sanitize(u.FullName),
		Email:     u.Email,
		AvatarURL: u.AvatarURL(),
	}
}

func toUserEmail(email *database.EmailAddress) *types.UserEmail {
	return &types.UserEmail{
		Email:    email.Email,
		Verified: email.IsActivated,
		Primary:  email.IsPrimary,
	}
}

func toBranch(b *database.Branch, c *git.Commit) *types.RepositoryBranch {
	return &types.RepositoryBranch{
		Name:   b.Name,
		Commit: toPayloadCommit(c),
	}
}

func toTag(b *database.Tag, c *git.Commit) *tag {
	return &tag{
		Name:   b.Name,
		Commit: toPayloadCommit(c),
	}
}

func toPayloadCommit(c *git.Commit) *types.WebhookPayloadCommit {
	authorUsername := ""
	author, err := database.Handle.Users().GetByEmail(context.TODO(), c.Author.Email)
	if err == nil {
		authorUsername = author.Name
	}
	committerUsername := ""
	committer, err := database.Handle.Users().GetByEmail(context.TODO(), c.Committer.Email)
	if err == nil {
		committerUsername = committer.Name
	}
	return &types.WebhookPayloadCommit{
		ID:      c.ID.String(),
		Message: c.Message,
		URL:     "Not implemented",
		Author: &types.WebhookPayloadUser{
			Name:     c.Author.Name,
			Email:    c.Author.Email,
			UserName: authorUsername,
		},
		Committer: &types.WebhookPayloadUser{
			Name:     c.Committer.Name,
			Email:    c.Committer.Email,
			UserName: committerUsername,
		},
		Timestamp: c.Author.When,
	}
}

func toUserPublicKey(apiLink string, key *database.PublicKey) *types.UserPublicKey {
	return &types.UserPublicKey{
		ID:      key.ID,
		Key:     key.Content,
		URL:     apiLink + strconv.FormatInt(key.ID, 10),
		Title:   key.Name,
		Created: key.Created,
	}
}

func toRepositoryHook(repoLink string, w *database.Webhook) *types.RepositoryHook {
	config := map[string]string{
		"url":          w.URL,
		"content_type": w.ContentType.Name(),
	}
	if w.HookTaskType == database.SLACK {
		s := w.SlackMeta()
		config["channel"] = s.Channel
		config["username"] = s.Username
		config["icon_url"] = s.IconURL
		config["color"] = s.Color
	}

	return &types.RepositoryHook{
		ID:      w.ID,
		Type:    w.HookTaskType.Name(),
		URL:     fmt.Sprintf("%s/settings/hooks/%d", repoLink, w.ID),
		Active:  w.IsActive,
		Config:  config,
		Events:  w.EventsArray(),
		Updated: w.Updated,
		Created: w.Created,
	}
}

func toDeployKey(apiLink string, key *database.DeployKey) *types.RepositoryDeployKey {
	return &types.RepositoryDeployKey{
		ID:       key.ID,
		Key:      key.Content,
		URL:      apiLink + strconv.FormatInt(key.ID, 10),
		Title:    key.Name,
		Created:  key.Created,
		ReadOnly: true, // All deploy keys are read-only.
	}
}

func toOrganization(org *database.User) *types.Organization {
	return &types.Organization{
		ID:          org.ID,
		AvatarURL:   org.AvatarURL(),
		UserName:    org.Name,
		FullName:    org.FullName,
		Description: org.Description,
		Website:     org.Website,
		Location:    org.Location,
	}
}

func toOrganizationTeam(team *database.Team) *types.OrganizationTeam {
	return &types.OrganizationTeam{
		ID:          team.ID,
		Name:        team.Name,
		Description: team.Description,
		Permission:  team.Authorize.String(),
	}
}

func toIssueLabel(l *database.Label) *types.IssueLabel {
	return &types.IssueLabel{
		ID:    l.ID,
		Name:  l.Name,
		Color: strings.TrimLeft(l.Color, "#"),
	}
}

func issueState(isClosed bool) types.IssueStateType {
	if isClosed {
		return types.IssueStateClosed
	}
	return types.IssueStateOpen
}

// toIssue converts a database issue to an API issue.
// It assumes the following fields have been assigned with valid values:
// Required - Poster, Labels
// Optional - Milestone, Assignee, PullRequest
func toIssue(issue *database.Issue) *types.Issue {
	labels := make([]*types.IssueLabel, len(issue.Labels))
	for i := range issue.Labels {
		labels[i] = toIssueLabel(issue.Labels[i])
	}

	apiIssue := &types.Issue{
		ID:       issue.ID,
		Index:    issue.Index,
		Poster:   toUser(issue.Poster),
		Title:    issue.Title,
		Body:     issue.Content,
		Labels:   labels,
		State:    issueState(issue.IsClosed),
		Comments: issue.NumComments,
		Created:  issue.Created,
		Updated:  issue.Updated,
	}

	if issue.Milestone != nil {
		apiIssue.Milestone = toIssueMilestone(issue.Milestone)
	}
	if issue.Assignee != nil {
		apiIssue.Assignee = toUser(issue.Assignee)
	}
	if issue.IsPull {
		apiIssue.PullRequest = &types.PullRequestMeta{
			HasMerged: issue.PullRequest.HasMerged,
		}
		if issue.PullRequest.HasMerged {
			apiIssue.PullRequest.Merged = &issue.PullRequest.Merged
		}
	}

	return apiIssue
}

func toIssueComment(c *database.Comment) *types.IssueComment {
	return &types.IssueComment{
		ID:      c.ID,
		HTMLURL: c.HTMLURL(),
		Poster:  toUser(c.Poster),
		Body:    c.Content,
		Created: c.Created,
		Updated: c.Updated,
	}
}

func toIssueMilestone(m *database.Milestone) *types.IssueMilestone {
	ms := &types.IssueMilestone{
		ID:           m.ID,
		State:        issueState(m.IsClosed),
		Title:        m.Name,
		Description:  m.Content,
		OpenIssues:   m.NumOpenIssues,
		ClosedIssues: m.NumClosedIssues,
	}
	if m.IsClosed {
		ms.Closed = &m.ClosedDate
	}
	if m.Deadline.Year() < 9999 {
		ms.Deadline = &m.Deadline
	}
	return ms
}

// toRelease converts a database release to an API release.
// It assumes the Publisher field has been assigned.
func toRelease(r *database.Release) *types.RepositoryRelease {
	return &types.RepositoryRelease{
		ID:              r.ID,
		TagName:         r.TagName,
		TargetCommitish: r.Target,
		Name:            r.Title,
		Body:            r.Note,
		Draft:           r.IsDraft,
		Prerelease:      r.IsPrerelease,
		Author:          toUser(r.Publisher),
		Created:         r.Created,
	}
}

func toRepositoryCollaborator(c *database.Collaborator) *types.RepositoryCollaborator {
	return &types.RepositoryCollaborator{
		User: toUser(c.User),
		Permissions: types.RepositoryPermission{
			Admin: c.Collaboration.Mode >= database.AccessModeAdmin,
			Push:  c.Collaboration.Mode >= database.AccessModeWrite,
			Pull:  c.Collaboration.Mode >= database.AccessModeRead,
		},
	}
}

// toRepository converts a database repository to an API repository.
// It assumes the Owner field has been loaded on the repo.
func toRepository(repo *database.Repository, perm *types.RepositoryPermission) *types.Repository {
	cloneLink := repo.CloneLink()
	apiRepo := &types.Repository{
		ID:            repo.ID,
		Owner:         toUser(repo.Owner),
		Name:          repo.Name,
		FullName:      repo.FullName(),
		Description:   repo.Description,
		Private:       repo.IsPrivate,
		Fork:          repo.IsFork,
		Empty:         repo.IsBare,
		Mirror:        repo.IsMirror,
		Size:          repo.Size,
		HTMLURL:       repo.HTMLURL(),
		SSHURL:        cloneLink.SSH,
		CloneURL:      cloneLink.HTTPS,
		Website:       repo.Website,
		Stars:         repo.NumStars,
		Forks:         repo.NumForks,
		Watchers:      repo.NumWatches,
		OpenIssues:    repo.NumOpenIssues,
		DefaultBranch: repo.DefaultBranch,
		Created:       repo.Created,
		Updated:       repo.Updated,
		Permissions:   perm,
	}
	if repo.IsFork {
		p := &types.RepositoryPermission{Pull: true}
		apiRepo.Parent = toRepository(repo.BaseRepo, p)
	}
	return apiRepo
}
