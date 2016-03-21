// Copyright 2015 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package convert

import (
	"fmt"

	"github.com/Unknwon/com"

	"github.com/gogits/git-module"
	api "github.com/gogits/go-gogs-client"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/log"
	"github.com/gogits/gogs/modules/setting"
)

func ToUser(u *models.User) *api.User {
	if u == nil {
		return nil
	}

	return &api.User{
		ID:        u.Id,
		UserName:  u.Name,
		FullName:  u.FullName,
		Email:     u.Email,
		AvatarUrl: u.AvatarLink(),
	}
}

func ToEmail(email *models.EmailAddress) *api.Email {
	return &api.Email{
		Email:    email.Email,
		Verified: email.IsActivated,
		Primary:  email.IsPrimary,
	}
}

func ToRepository(owner *models.User, repo *models.Repository, permission api.Permission) *api.Repository {
	cl := repo.CloneLink()
	return &api.Repository{
		ID:          repo.ID,
		Owner:       ToUser(owner),
		FullName:    owner.Name + "/" + repo.Name,
		Private:     repo.IsPrivate,
		Fork:        repo.IsFork,
		HtmlUrl:     setting.AppUrl + owner.Name + "/" + repo.Name,
		CloneUrl:    cl.HTTPS,
		SshUrl:      cl.SSH,
		Permissions: permission,
	}
}

func ToBranch(b *models.Branch, c *git.Commit) *api.Branch {
	return &api.Branch{
		Name:   b.Name,
		Commit: ToCommit(c),
	}
}

func ToCommit(c *git.Commit) *api.PayloadCommit {
	return &api.PayloadCommit{
		ID:      c.ID.String(),
		Message: c.Message(),
		URL:     "Not implemented",
		Author: &api.PayloadAuthor{
			Name:  c.Committer.Name,
			Email: c.Committer.Email,
			/* UserName: c.Committer.UserName, */
		},
	}
}

func ToPublicKey(apiLink string, key *models.PublicKey) *api.PublicKey {
	return &api.PublicKey{
		ID:      key.ID,
		Key:     key.Content,
		URL:     apiLink + com.ToStr(key.ID),
		Title:   key.Name,
		Created: key.Created,
	}
}

func ToHook(repoLink string, w *models.Webhook) *api.Hook {
	config := map[string]string{
		"url":          w.URL,
		"content_type": w.ContentType.Name(),
	}
	if w.HookTaskType == models.SLACK {
		s := w.GetSlackHook()
		config["channel"] = s.Channel
		config["username"] = s.Username
		config["icon_url"] = s.IconURL
		config["color"] = s.Color
	}

	return &api.Hook{
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

func ToDeployKey(apiLink string, key *models.DeployKey) *api.DeployKey {
	return &api.DeployKey{
		ID:       key.ID,
		Key:      key.Content,
		URL:      apiLink + com.ToStr(key.ID),
		Title:    key.Name,
		Created:  key.Created,
		ReadOnly: true, // All deploy keys are read-only.
	}
}

func ToLabel(label *models.Label) *api.Label {
	return &api.Label{
		Name:  label.Name,
		Color: label.Color,
	}
}

func ToMilestone(milestone *models.Milestone) *api.Milestone {
	if milestone == nil {
		return nil
	}

	apiMilestone := &api.Milestone{
		ID:           milestone.ID,
		State:        milestone.State(),
		Title:        milestone.Name,
		Description:  milestone.Content,
		OpenIssues:   milestone.NumOpenIssues,
		ClosedIssues: milestone.NumClosedIssues,
	}
	if milestone.IsClosed {
		apiMilestone.Closed = &milestone.ClosedDate
	}
	if milestone.Deadline.Year() < 9999 {
		apiMilestone.Deadline = &milestone.Deadline
	}
	return apiMilestone
}

func ToIssue(issue *models.Issue) *api.Issue {
	apiLabels := make([]*api.Label, len(issue.Labels))
	for i := range issue.Labels {
		apiLabels[i] = ToLabel(issue.Labels[i])
	}

	apiIssue := &api.Issue{
		ID:        issue.ID,
		Index:     issue.Index,
		State:     issue.State(),
		Title:     issue.Name,
		Body:      issue.Content,
		User:      ToUser(issue.Poster),
		Labels:    apiLabels,
		Assignee:  ToUser(issue.Assignee),
		Milestone: ToMilestone(issue.Milestone),
		Comments:  issue.NumComments,
		Created:   issue.Created,
		Updated:   issue.Updated,
	}
	if issue.IsPull {
		if err := issue.GetPullRequest(); err != nil {
			log.Error(4, "GetPullRequest", err)
		} else {
			apiIssue.PullRequest = &api.PullRequestMeta{
				HasMerged: issue.PullRequest.HasMerged,
			}
			if issue.PullRequest.HasMerged {
				apiIssue.PullRequest.Merged = &issue.PullRequest.Merged
			}
		}
	}

	return apiIssue
}

func ToOrganization(org *models.User) *api.Organization {
	return &api.Organization{
		ID:          org.Id,
		AvatarUrl:   org.AvatarLink(),
		UserName:    org.Name,
		FullName:    org.FullName,
		Description: org.Description,
		Website:     org.Website,
		Location:    org.Location,
	}
}

func ToTeam(team *models.Team) *api.Team {
	return &api.Team{
		ID:          team.ID,
		Name:        team.Name,
		Description: team.Description,
		Permission:  team.Authorize.String(),
	}
}
