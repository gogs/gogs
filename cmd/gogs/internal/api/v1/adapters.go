package v1

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"gogs.io/gogs/internal/api/v1types"
	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/git"
	"gogs.io/gogs/internal/markup"
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
func toUser(u *database.User) *v1types.User {
	return &v1types.User{
		ID:        u.ID,
		UserName:  u.Name,
		Login:     u.Name,
		FullName:  markup.Sanitize(u.FullName),
		Email:     u.Email,
		AvatarURL: u.AvatarURL(),
	}
}

func toUserEmail(email *database.EmailAddress) *v1types.UserEmail {
	return &v1types.UserEmail{
		Email:    email.Email,
		Verified: email.IsActivated,
		Primary:  email.IsPrimary,
	}
}

func toBranch(b *database.Branch, c *git.Commit) *v1types.RepositoryBranch {
	return &v1types.RepositoryBranch{
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

func toPayloadCommit(c *git.Commit) *v1types.WebhookPayloadCommit {
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
	return &v1types.WebhookPayloadCommit{
		ID:      c.ID.String(),
		Message: c.Message,
		URL:     "Not implemented",
		Author: &v1types.WebhookPayloadUser{
			Name:     c.Author.Name,
			Email:    c.Author.Email,
			UserName: authorUsername,
		},
		Committer: &v1types.WebhookPayloadUser{
			Name:     c.Committer.Name,
			Email:    c.Committer.Email,
			UserName: committerUsername,
		},
		Timestamp: c.Author.When,
	}
}

func toUserPublicKey(apiLink string, key *database.PublicKey) *v1types.UserPublicKey {
	return &v1types.UserPublicKey{
		ID:      key.ID,
		Key:     key.Content,
		URL:     apiLink + strconv.FormatInt(key.ID, 10),
		Title:   key.Name,
		Created: key.Created,
	}
}

func toRepositoryHook(w *database.Webhook) *v1types.RepositoryHook {
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

	return &v1types.RepositoryHook{
		ID:   w.ID,
		Type: w.HookTaskType.Name(),
		// The URL has always missed the repository link prefix, keep it as-is for
		// compatibility.
		URL:     fmt.Sprintf("/settings/hooks/%d", w.ID),
		Active:  w.IsActive,
		Config:  config,
		Events:  w.EventsArray(),
		Updated: w.Updated,
		Created: w.Created,
	}
}

func toDeployKey(apiLink string, key *database.DeployKey) *v1types.RepositoryDeployKey {
	return &v1types.RepositoryDeployKey{
		ID:       key.ID,
		Key:      key.Content,
		URL:      apiLink + strconv.FormatInt(key.ID, 10),
		Title:    key.Name,
		Created:  key.Created,
		ReadOnly: true, // All deploy keys are read-only.
	}
}

func toOrganization(org *database.User) *v1types.Organization {
	return &v1types.Organization{
		ID:          org.ID,
		AvatarURL:   org.AvatarURL(),
		UserName:    org.Name,
		FullName:    org.FullName,
		Description: org.Description,
		Website:     org.Website,
		Location:    org.Location,
	}
}

func toOrganizationTeam(team *database.Team) *v1types.OrganizationTeam {
	return &v1types.OrganizationTeam{
		ID:          team.ID,
		Name:        team.Name,
		Description: team.Description,
		Permission:  team.Authorize.String(),
	}
}

func toIssueLabel(l *database.Label) *v1types.IssueLabel {
	return &v1types.IssueLabel{
		ID:    l.ID,
		Name:  l.Name,
		Color: strings.TrimLeft(l.Color, "#"),
	}
}

func issueState(isClosed bool) v1types.IssueStateType {
	if isClosed {
		return v1types.IssueStateClosed
	}
	return v1types.IssueStateOpen
}

// toIssue converts a database issue to an API issue.
// It assumes the following fields have been assigned with valid values:
// Required - Poster, Labels
// Optional - Milestone, Assignee, PullRequest
func toIssue(issue *database.Issue) *v1types.Issue {
	labels := make([]*v1types.IssueLabel, len(issue.Labels))
	for i := range issue.Labels {
		labels[i] = toIssueLabel(issue.Labels[i])
	}

	apiIssue := &v1types.Issue{
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
		apiIssue.PullRequest = &v1types.PullRequestMeta{
			HasMerged: issue.PullRequest.HasMerged,
		}
		if issue.PullRequest.HasMerged {
			apiIssue.PullRequest.Merged = &issue.PullRequest.Merged
		}
	}

	return apiIssue
}

func toIssueComment(c *database.Comment) *v1types.IssueComment {
	return &v1types.IssueComment{
		ID:      c.ID,
		HTMLURL: c.HTMLURL(),
		Poster:  toUser(c.Poster),
		Body:    c.Content,
		Created: c.Created,
		Updated: c.Updated,
	}
}

func toIssueMilestone(m *database.Milestone) *v1types.IssueMilestone {
	ms := &v1types.IssueMilestone{
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
func toRelease(r *database.Release) *v1types.RepositoryRelease {
	return &v1types.RepositoryRelease{
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

func toRepositoryCollaborator(c *database.Collaborator) *v1types.RepositoryCollaborator {
	return &v1types.RepositoryCollaborator{
		User: toUser(c.User),
		Permissions: v1types.RepositoryPermission{
			Admin: c.Collaboration.Mode >= database.AccessModeAdmin,
			Push:  c.Collaboration.Mode >= database.AccessModeWrite,
			Pull:  c.Collaboration.Mode >= database.AccessModeRead,
		},
	}
}

// toRepository converts a database repository to an API repository. It
// assumes the Owner field has been loaded on the repo, and the BaseRepo field
// with its Owner field if the repo is a fork. The parent is included with
// basePerm, the permission of the viewer on the base repository, and left out
// when basePerm is nil, e.g., when the viewer cannot read it.
func toRepository(repo *database.Repository, perm, basePerm *v1types.RepositoryPermission) *v1types.Repository {
	apiRepo := toRepositoryWithoutParent(repo, perm)
	if repo.IsFork && basePerm != nil {
		// The parent does not include its own parent.
		apiRepo.Parent = toRepositoryWithoutParent(repo.BaseRepo, basePerm)
	}
	return apiRepo
}

func toRepositoryWithoutParent(repo *database.Repository, perm *v1types.RepositoryPermission) *v1types.Repository {
	cloneLink := repo.CloneLink()
	return &v1types.Repository{
		ID:            repo.ID,
		Owner:         toUser(repo.Owner),
		Name:          repo.Name,
		FullName:      repo.FullName(),
		Description:   repo.Description,
		Private:       repo.IsPrivate,
		Fork:          repo.IsFork,
		Empty:         repo.IsEmpty,
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
}
