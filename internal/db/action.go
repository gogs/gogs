// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package db

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/gogs/git-module"
	api "github.com/gogs/go-gogs-client"
	"github.com/json-iterator/go"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/lazyregexp"
)

var (
	// Same as Github. See https://help.github.com/articles/closing-issues-via-commit-messages
	issueCloseKeywords  = []string{"close", "closes", "closed", "fix", "fixes", "fixed", "resolve", "resolves", "resolved"}
	issueReopenKeywords = []string{"reopen", "reopens", "reopened"}

	issueCloseKeywordsPat  = lazyregexp.New(assembleKeywordsPattern(issueCloseKeywords))
	issueReopenKeywordsPat = lazyregexp.New(assembleKeywordsPattern(issueReopenKeywords))
	issueReferencePattern  = lazyregexp.New(`(?i)(?:)(^| )\S*#\d+`)
)

func assembleKeywordsPattern(words []string) string {
	return fmt.Sprintf(`(?i)(?:%s) \S+`, strings.Join(words, "|"))
}

func issueIndexTrimRight(c rune) bool {
	return !unicode.IsDigit(c)
}

// UpdateIssuesCommit checks if issues are manipulated by commit message.
func UpdateIssuesCommit(doer *User, repo *Repository, commits []*PushCommit) error {
	// Commits are appended in the reverse order.
	for i := len(commits) - 1; i >= 0; i-- {
		c := commits[i]

		refMarked := make(map[int64]bool)
		for _, ref := range issueReferencePattern.FindAllString(c.Message, -1) {
			ref = strings.TrimSpace(ref)
			ref = strings.TrimRightFunc(ref, issueIndexTrimRight)

			if len(ref) == 0 {
				continue
			}

			// Add repo name if missing
			if ref[0] == '#' {
				ref = fmt.Sprintf("%s%s", repo.FullName(), ref)
			} else if !strings.Contains(ref, "/") {
				// FIXME: We don't support User#ID syntax yet
				continue
			}

			issue, err := GetIssueByRef(ref)
			if err != nil {
				if IsErrIssueNotExist(err) {
					continue
				}
				return err
			}

			if refMarked[issue.ID] {
				continue
			}
			refMarked[issue.ID] = true

			msgLines := strings.Split(c.Message, "\n")
			shortMsg := msgLines[0]
			if len(msgLines) > 2 {
				shortMsg += "..."
			}
			message := fmt.Sprintf(`<a href="%s/commit/%s">%s</a>`, repo.Link(), c.Sha1, shortMsg)
			if err = CreateRefComment(doer, repo, issue, message, c.Sha1); err != nil {
				return err
			}
		}

		refMarked = make(map[int64]bool)
		// FIXME: can merge this one and next one to a common function.
		for _, ref := range issueCloseKeywordsPat.FindAllString(c.Message, -1) {
			ref = ref[strings.IndexByte(ref, byte(' '))+1:]
			ref = strings.TrimRightFunc(ref, issueIndexTrimRight)

			if len(ref) == 0 {
				continue
			}

			// Add repo name if missing
			if ref[0] == '#' {
				ref = fmt.Sprintf("%s%s", repo.FullName(), ref)
			} else if !strings.Contains(ref, "/") {
				// FIXME: We don't support User#ID syntax yet
				continue
			}

			issue, err := GetIssueByRef(ref)
			if err != nil {
				if IsErrIssueNotExist(err) {
					continue
				}
				return err
			}

			if refMarked[issue.ID] {
				continue
			}
			refMarked[issue.ID] = true

			if issue.RepoID != repo.ID || issue.IsClosed {
				continue
			}

			if err = issue.ChangeStatus(doer, repo, true); err != nil {
				return err
			}
		}

		// It is conflict to have close and reopen at same time, so refsMarkd doesn't need to reinit here.
		for _, ref := range issueReopenKeywordsPat.FindAllString(c.Message, -1) {
			ref = ref[strings.IndexByte(ref, byte(' '))+1:]
			ref = strings.TrimRightFunc(ref, issueIndexTrimRight)

			if len(ref) == 0 {
				continue
			}

			// Add repo name if missing
			if ref[0] == '#' {
				ref = fmt.Sprintf("%s%s", repo.FullName(), ref)
			} else if !strings.Contains(ref, "/") {
				// We don't support User#ID syntax yet
				// return ErrNotImplemented
				continue
			}

			issue, err := GetIssueByRef(ref)
			if err != nil {
				if IsErrIssueNotExist(err) {
					continue
				}
				return err
			}

			if refMarked[issue.ID] {
				continue
			}
			refMarked[issue.ID] = true

			if issue.RepoID != repo.ID || !issue.IsClosed {
				continue
			}

			if err = issue.ChangeStatus(doer, repo, false); err != nil {
				return err
			}
		}
	}
	return nil
}

type CommitRepoActionOptions struct {
	PusherName  string
	RepoOwnerID int64
	RepoName    string
	RefFullName string
	OldCommitID string
	NewCommitID string
	Commits     *PushCommits
}

// CommitRepoAction adds new commit action to the repository, and prepare corresponding webhooks.
func CommitRepoAction(opts CommitRepoActionOptions) error {
	pusher, err := GetUserByName(opts.PusherName)
	if err != nil {
		return fmt.Errorf("GetUserByName [%s]: %v", opts.PusherName, err)
	}

	repo, err := GetRepositoryByName(opts.RepoOwnerID, opts.RepoName)
	if err != nil {
		return fmt.Errorf("GetRepositoryByName [owner_id: %d, name: %s]: %v", opts.RepoOwnerID, opts.RepoName, err)
	}

	// Change repository bare status and update last updated time.
	repo.IsBare = false
	if err = UpdateRepository(repo, false); err != nil {
		return fmt.Errorf("UpdateRepository: %v", err)
	}

	isNewRef := opts.OldCommitID == git.EmptyID
	isDelRef := opts.NewCommitID == git.EmptyID

	opType := ActionCommitRepo
	// Check if it's tag push or branch.
	if strings.HasPrefix(opts.RefFullName, git.RefsTags) {
		opType = ActionPushTag
	} else {
		// if not the first commit, set the compare URL.
		if !isNewRef && !isDelRef {
			opts.Commits.CompareURL = repo.ComposeCompareURL(opts.OldCommitID, opts.NewCommitID)
		}

		// Only update issues via commits when internal issue tracker is enabled
		if repo.EnableIssues && !repo.EnableExternalTracker {
			if err = UpdateIssuesCommit(pusher, repo, opts.Commits.Commits); err != nil {
				log.Error("UpdateIssuesCommit: %v", err)
			}
		}
	}

	if len(opts.Commits.Commits) > conf.UI.FeedMaxCommitNum {
		opts.Commits.Commits = opts.Commits.Commits[:conf.UI.FeedMaxCommitNum]
	}

	data, err := jsoniter.Marshal(opts.Commits)
	if err != nil {
		return fmt.Errorf("Marshal: %v", err)
	}

	refName := git.RefShortName(opts.RefFullName)
	action := &Action{
		ActUserID:    pusher.ID,
		ActUserName:  pusher.Name,
		Content:      string(data),
		RepoID:       repo.ID,
		RepoUserName: repo.MustOwner().Name,
		RepoName:     repo.Name,
		RefName:      refName,
		IsPrivate:    repo.IsPrivate || repo.IsUnlisted,
	}

	apiRepo := repo.APIFormat(nil)
	apiPusher := pusher.APIFormat()
	switch opType {
	case ActionCommitRepo: // Push
		if isDelRef {
			if err = PrepareWebhooks(repo, HOOK_EVENT_DELETE, &api.DeletePayload{
				Ref:        refName,
				RefType:    "branch",
				PusherType: api.PUSHER_TYPE_USER,
				Repo:       apiRepo,
				Sender:     apiPusher,
			}); err != nil {
				return fmt.Errorf("PrepareWebhooks.(delete branch): %v", err)
			}

			action.OpType = ActionDeleteBranch
			if err = NotifyWatchers(action); err != nil {
				return fmt.Errorf("NotifyWatchers.(delete branch): %v", err)
			}

			// Delete branch doesn't have anything to push or compare
			return nil
		}

		compareURL := conf.Server.ExternalURL + opts.Commits.CompareURL
		if isNewRef {
			compareURL = ""
			if err = PrepareWebhooks(repo, HOOK_EVENT_CREATE, &api.CreatePayload{
				Ref:           refName,
				RefType:       "branch",
				DefaultBranch: repo.DefaultBranch,
				Repo:          apiRepo,
				Sender:        apiPusher,
			}); err != nil {
				return fmt.Errorf("PrepareWebhooks.(new branch): %v", err)
			}

			action.OpType = ActionCreateBranch
			if err = NotifyWatchers(action); err != nil {
				return fmt.Errorf("NotifyWatchers.(new branch): %v", err)
			}
		}

		commits, err := opts.Commits.ToApiPayloadCommits(repo.RepoPath(), repo.HTMLURL())
		if err != nil {
			return fmt.Errorf("ToApiPayloadCommits: %v", err)
		}

		if err = PrepareWebhooks(repo, HOOK_EVENT_PUSH, &api.PushPayload{
			Ref:        opts.RefFullName,
			Before:     opts.OldCommitID,
			After:      opts.NewCommitID,
			CompareURL: compareURL,
			Commits:    commits,
			Repo:       apiRepo,
			Pusher:     apiPusher,
			Sender:     apiPusher,
		}); err != nil {
			return fmt.Errorf("PrepareWebhooks.(new commit): %v", err)
		}

		action.OpType = ActionCommitRepo
		if err = NotifyWatchers(action); err != nil {
			return fmt.Errorf("NotifyWatchers.(new commit): %v", err)
		}

	case ActionPushTag: // Tag
		if isDelRef {
			if err = PrepareWebhooks(repo, HOOK_EVENT_DELETE, &api.DeletePayload{
				Ref:        refName,
				RefType:    "tag",
				PusherType: api.PUSHER_TYPE_USER,
				Repo:       apiRepo,
				Sender:     apiPusher,
			}); err != nil {
				return fmt.Errorf("PrepareWebhooks.(delete tag): %v", err)
			}

			action.OpType = ActionDeleteTag
			if err = NotifyWatchers(action); err != nil {
				return fmt.Errorf("NotifyWatchers.(delete tag): %v", err)
			}
			return nil
		}

		if err = PrepareWebhooks(repo, HOOK_EVENT_CREATE, &api.CreatePayload{
			Ref:           refName,
			RefType:       "tag",
			Sha:           opts.NewCommitID,
			DefaultBranch: repo.DefaultBranch,
			Repo:          apiRepo,
			Sender:        apiPusher,
		}); err != nil {
			return fmt.Errorf("PrepareWebhooks.(new tag): %v", err)
		}

		action.OpType = ActionPushTag
		if err = NotifyWatchers(action); err != nil {
			return fmt.Errorf("NotifyWatchers.(new tag): %v", err)
		}
	}

	return nil
}
