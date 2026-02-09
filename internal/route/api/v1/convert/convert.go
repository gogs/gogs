package convert

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/gogs/git-module"

	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/markup"
	"gogs.io/gogs/internal/route/api/v1/types"
)

func ToUser(u *database.User) *types.User {
	return &types.User{
		ID:        u.ID,
		UserName:  u.Name,
		Login:     u.Name,
		FullName:  u.FullName,
		Email:     u.Email,
		AvatarURL: u.AvatarURL(),
	}
}

// ToUserSanitized returns a user with the full name sanitized for safe HTML rendering.
func ToUserSanitized(u *database.User) *types.User {
	r := ToUser(u)
	r.FullName = markup.Sanitize(u.FullName)
	return r
}

func ToEmail(email *database.EmailAddress) *types.Email {
	return &types.Email{
		Email:    email.Email,
		Verified: email.IsActivated,
		Primary:  email.IsPrimary,
	}
}

func ToBranch(b *database.Branch, c *git.Commit) *types.Branch {
	return &types.Branch{
		Name:   b.Name,
		Commit: ToPayloadCommit(c),
	}
}

type Tag struct {
	Name   string               `json:"name"`
	Commit *types.PayloadCommit `json:"commit"`
}

func ToTag(b *database.Tag, c *git.Commit) *Tag {
	return &Tag{
		Name:   b.Name,
		Commit: ToPayloadCommit(c),
	}
}

func ToPayloadCommit(c *git.Commit) *types.PayloadCommit {
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
	return &types.PayloadCommit{
		ID:      c.ID.String(),
		Message: c.Message,
		URL:     "Not implemented",
		Author: &types.PayloadUser{
			Name:     c.Author.Name,
			Email:    c.Author.Email,
			UserName: authorUsername,
		},
		Committer: &types.PayloadUser{
			Name:     c.Committer.Name,
			Email:    c.Committer.Email,
			UserName: committerUsername,
		},
		Timestamp: c.Author.When,
	}
}

func ToPublicKey(apiLink string, key *database.PublicKey) *types.PublicKey {
	return &types.PublicKey{
		ID:      key.ID,
		Key:     key.Content,
		URL:     apiLink + strconv.FormatInt(key.ID, 10),
		Title:   key.Name,
		Created: key.Created,
	}
}

func ToHook(repoLink string, w *database.Webhook) *types.Hook {
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

	return &types.Hook{
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

func ToDeployKey(apiLink string, key *database.DeployKey) *types.DeployKey {
	return &types.DeployKey{
		ID:       key.ID,
		Key:      key.Content,
		URL:      apiLink + strconv.FormatInt(key.ID, 10),
		Title:    key.Name,
		Created:  key.Created,
		ReadOnly: true, // All deploy keys are read-only.
	}
}

func ToOrganization(org *database.User) *types.Organization {
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

func ToTeam(team *database.Team) *types.Team {
	return &types.Team{
		ID:          team.ID,
		Name:        team.Name,
		Description: team.Description,
		Permission:  team.Authorize.String(),
	}
}

func ToLabel(l *database.Label) *types.Label {
	return &types.Label{
		ID:    l.ID,
		Name:  l.Name,
		Color: strings.TrimLeft(l.Color, "#"),
	}
}

func issueState(isClosed bool) types.StateType {
	if isClosed {
		return types.StateClosed
	}
	return types.StateOpen
}

// ToIssue converts a database issue to an API issue.
// It assumes the following fields have been assigned with valid values:
// Required - Poster, Labels
// Optional - Milestone, Assignee, PullRequest
func ToIssue(issue *database.Issue) *types.Issue {
	labels := make([]*types.Label, len(issue.Labels))
	for i := range issue.Labels {
		labels[i] = ToLabel(issue.Labels[i])
	}

	apiIssue := &types.Issue{
		ID:       issue.ID,
		Index:    issue.Index,
		Poster:   ToUser(issue.Poster),
		Title:    issue.Title,
		Body:     issue.Content,
		Labels:   labels,
		State:    issueState(issue.IsClosed),
		Comments: issue.NumComments,
		Created:  issue.Created,
		Updated:  issue.Updated,
	}

	if issue.Milestone != nil {
		apiIssue.Milestone = ToMilestone(issue.Milestone)
	}
	if issue.Assignee != nil {
		apiIssue.Assignee = ToUser(issue.Assignee)
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

func ToComment(c *database.Comment) *types.Comment {
	return &types.Comment{
		ID:      c.ID,
		HTMLURL: c.HTMLURL(),
		Poster:  ToUser(c.Poster),
		Body:    c.Content,
		Created: c.Created,
		Updated: c.Updated,
	}
}

func ToMilestone(m *database.Milestone) *types.Milestone {
	ms := &types.Milestone{
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

// ToRelease converts a database release to an API release.
// It assumes the Publisher field has been assigned.
func ToRelease(r *database.Release) *types.Release {
	return &types.Release{
		ID:              r.ID,
		TagName:         r.TagName,
		TargetCommitish: r.Target,
		Name:            r.Title,
		Body:            r.Note,
		Draft:           r.IsDraft,
		Prerelease:      r.IsPrerelease,
		Author:          ToUser(r.Publisher),
		Created:         r.Created,
	}
}

func ToCollaborator(c *database.Collaborator) *types.Collaborator {
	return &types.Collaborator{
		User: ToUser(c.User),
		Permissions: types.Permission{
			Admin: c.Collaboration.Mode >= database.AccessModeAdmin,
			Push:  c.Collaboration.Mode >= database.AccessModeWrite,
			Pull:  c.Collaboration.Mode >= database.AccessModeRead,
		},
	}
}

// ToRepository converts a database repository to an API repository.
// It assumes the Owner field has been loaded on the repo.
func ToRepository(repo *database.Repository, perm *types.Permission, user ...*database.User) *types.Repository {
	cloneLink := repo.CloneLink()
	apiRepo := &types.Repository{
		ID:            repo.ID,
		Owner:         ToUser(repo.Owner),
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
		p := &types.Permission{Pull: true}
		if len(user) != 0 {
			accessMode := database.Handle.Permissions().AccessMode(
				context.TODO(),
				user[0].ID,
				repo.ID,
				database.AccessModeOptions{
					OwnerID: repo.OwnerID,
					Private: repo.IsPrivate,
				},
			)
			p.Admin = accessMode >= database.AccessModeAdmin
			p.Push = accessMode >= database.AccessModeWrite
		}
		apiRepo.Parent = ToRepository(repo.BaseRepo, p)
	}
	return apiRepo
}

// ToPullRequest converts a database pull request to an API pull request.
// It assumes the following fields have been assigned with valid values:
// Required - Issue, BaseRepo
// Optional - HeadRepo, Merger
func ToPullRequest(pr *database.PullRequest) *types.PullRequest {
	var apiHeadRepo *types.Repository
	if pr.HeadRepo == nil {
		apiHeadRepo = &types.Repository{
			Name: "deleted",
		}
	} else {
		apiHeadRepo = ToRepository(pr.HeadRepo, nil)
	}

	apiIssue := ToIssue(pr.Issue)
	apiPullRequest := &types.PullRequest{
		ID:         pr.ID,
		Index:      pr.Index,
		Poster:     apiIssue.Poster,
		Title:      apiIssue.Title,
		Body:       apiIssue.Body,
		Labels:     apiIssue.Labels,
		Milestone:  apiIssue.Milestone,
		Assignee:   apiIssue.Assignee,
		State:      apiIssue.State,
		Comments:   apiIssue.Comments,
		HeadBranch: pr.HeadBranch,
		HeadRepo:   apiHeadRepo,
		BaseBranch: pr.BaseBranch,
		BaseRepo:   ToRepository(pr.BaseRepo, nil),
		HTMLURL:    pr.Issue.HTMLURL(),
		HasMerged:  pr.HasMerged,
	}

	if pr.Status != database.PullRequestStatusChecking {
		mergeable := pr.Status != database.PullRequestStatusConflict
		apiPullRequest.Mergeable = &mergeable
	}
	if pr.HasMerged {
		apiPullRequest.Merged = &pr.Merged
		apiPullRequest.MergedCommitID = &pr.MergedCommitID
		apiPullRequest.MergedBy = ToUser(pr.Merger)
	}

	return apiPullRequest
}
