package database

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/cockroachdb/errors"
	"gorm.io/gorm"
	"unknwon.dev/x/logx"

	"gogs.io/gogs/internal/git"
	apiv1types "gogs.io/gogs/internal/route/api/v1/types"
)

// PullRequestsStore is the storage layer for pull requests.
type PullRequestsStore struct {
	db *gorm.DB
}

func newPullRequestsStore(db *gorm.DB) *PullRequestsStore {
	return &PullRequestsStore{db: db}
}

// Create creates the pull request, its issue with given labels and
// attachments, and its patch, tests whether the patch applies, notifies the
// watchers of the repository and the users it mentions, and prepares
// webhooks.
func (s *PullRequestsStore) Create(ctx context.Context, logger *logx.Logger, repo *Repository, pull *Issue, labelIDs []int64, uuids []string, pr *PullRequest, patch []byte) error {
	// Stopping midway, e.g., when the client disconnects, would leave a partial
	// change behind.
	ctx = context.WithoutCancel(ctx)

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		err := newIssuesStore(tx).create(ctx, NewIssueOptions{
			Repo:        repo,
			Issue:       pull,
			LableIDs:    labelIDs,
			Attachments: uuids,
			IsPull:      true,
		})
		if err != nil {
			return errors.Wrap(err, "create issue")
		}

		pr.Index = pull.Index
		if err = repo.SavePatch(pr.Index, patch); err != nil {
			return errors.Wrap(err, "save patch")
		}

		pr.BaseRepo = repo
		if err = newPullRequestsStore(tx).testPatch(ctx, logger, pr); err != nil {
			return errors.Wrap(err, "test patch")
		}
		// No conflict appears after test means mergeable.
		if pr.Status == PullRequestStatusChecking {
			pr.Status = PullRequestStatusMergeable
		}

		pr.IssueID = pull.ID
		return tx.Create(pr).Error
	})
	if err != nil {
		return err
	}

	err = newActionsStore(s.db).notifyWatchers(ctx, &Action{
		ActUserID:    pull.Poster.ID,
		ActUserName:  pull.Poster.Name,
		OpType:       ActionCreatePullRequest,
		Content:      fmt.Sprintf("%d|%s", pull.Index, pull.Title),
		RepoID:       repo.ID,
		RepoUserName: repo.Owner.Name,
		RepoName:     repo.Name,
		IsPrivate:    repo.IsPrivate,
	})
	if err != nil {
		logger.Error("Failed to notify watchers", "error", err)
	}
	if err = newIssuesStore(s.db).mailParticipants(ctx, logger, pull); err != nil {
		logger.Error("Failed to mail participants", "error", err)
	}

	pr.Issue = pull
	pull.PullRequest = pr
	err = newWebhooksStore(s.db).Prepare(ctx, logger, repo, HookEventTypePullRequest, &apiv1types.WebhookPullRequestPayload{
		Action:      apiv1types.WebhookIssueOpened,
		Index:       pull.Index,
		PullRequest: pr.APIFormat(logger),
		Repository:  repo.APIFormatLegacy(logger, nil),
		Sender:      pull.Poster.APIFormat(),
	})
	if err != nil {
		logger.Error("Failed to prepare webhooks", "error", err)
	}
	return nil
}

func (s *PullRequestsStore) get(ctx context.Context, args map[string]any, query string, values ...any) (*PullRequest, error) {
	pr := new(PullRequest)
	err := s.db.WithContext(ctx).Where(query, values...).First(pr).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrPullRequestNotExist{args: args}
		}
		return nil, err
	}
	return pr, s.LoadAttributes(ctx, pr)
}

// GetByID returns the pull request with given ID and its attributes. It
// returns ErrPullRequestNotExist when not found.
func (s *PullRequestsStore) GetByID(ctx context.Context, id int64) (*PullRequest, error) {
	return s.get(ctx, map[string]any{"pullRequestID": id}, "id = ?", id)
}

// GetByIssueID returns the pull request of the issue with given ID and its
// attributes. It returns ErrPullRequestNotExist when not found.
func (s *PullRequestsStore) GetByIssueID(ctx context.Context, issueID int64) (*PullRequest, error) {
	return s.get(ctx, map[string]any{"issueID": issueID}, "issue_id = ?", issueID)
}

// openUnmerged returns the query of pull requests that are open and not
// merged.
func (s *PullRequestsStore) openUnmerged(ctx context.Context) *gorm.DB {
	return s.db.WithContext(ctx).Select("pull_request.*").
		Joins("INNER JOIN issue ON issue.id = pull_request.issue_id").
		Where("pull_request.has_merged = ? AND issue.is_closed = ?", false, false)
}

// GetUnmerged returns the pull request that is open and not merged from given
// head branch to given base branch, without its attributes. It returns
// ErrPullRequestNotExist when not found.
func (s *PullRequestsStore) GetUnmerged(ctx context.Context, headRepoID, baseRepoID int64, headBranch, baseBranch string) (*PullRequest, error) {
	pr := new(PullRequest)
	err := s.openUnmerged(ctx).
		Where("pull_request.head_repo_id = ? AND pull_request.head_branch = ?", headRepoID, headBranch).
		Where("pull_request.base_repo_id = ? AND pull_request.base_branch = ?", baseRepoID, baseBranch).
		First(pr).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrPullRequestNotExist{args: map[string]any{
				"headRepoID": headRepoID,
				"baseRepoID": baseRepoID,
				"headBranch": headBranch,
				"baseBranch": baseBranch,
			}}
		}
		return nil, err
	}
	return pr, nil
}

// listUnmergedByHead returns the pull requests that are open and not merged
// from given branch of the repository with given ID, without their
// attributes.
func (s *PullRequestsStore) listUnmergedByHead(ctx context.Context, repoID int64, branch string) ([]*PullRequest, error) {
	prs := make([]*PullRequest, 0, 2)
	return prs, s.openUnmerged(ctx).
		Where("pull_request.head_repo_id = ? AND pull_request.head_branch = ?", repoID, branch).
		Order("pull_request.id ASC").
		Find(&prs).Error
}

// listUnmergedByBase returns the pull requests that are open and not merged
// into given branch of the repository with given ID, without their
// attributes.
func (s *PullRequestsStore) listUnmergedByBase(ctx context.Context, repoID int64, branch string) ([]*PullRequest, error) {
	prs := make([]*PullRequest, 0, 2)
	return prs, s.openUnmerged(ctx).
		Where("pull_request.base_repo_id = ? AND pull_request.base_branch = ?", repoID, branch).
		Order("pull_request.id ASC").
		Find(&prs).Error
}

// LoadAttributes sets the head and base repositories with their attributes,
// and the merger of merged ones, of given pull requests that are not set yet.
// A head repository that no longer exists is left unset, and a merger that no
// longer exists is the ghost user. Their issues are not loaded.
func (s *PullRequestsStore) LoadAttributes(ctx context.Context, prs ...*PullRequest) (err error) {
	db := s.db.WithContext(ctx)
	for _, pr := range prs {
		if pr.HeadRepo == nil {
			pr.HeadRepo, err = newReposStore(db).getWithAttributes(ctx, pr.HeadRepoID)
			if err != nil && !IsErrRepoNotExist(err) {
				return errors.Wrapf(err, "get head repository %d", pr.HeadRepoID)
			}
		}

		if pr.BaseRepo == nil {
			pr.BaseRepo, err = newReposStore(db).getWithAttributes(ctx, pr.BaseRepoID)
			if err != nil {
				return errors.Wrapf(err, "get base repository %d", pr.BaseRepoID)
			}
		}

		if pr.HasMerged && pr.Merger == nil {
			pr.Merger, err = newUsersStore(s.db).GetByID(ctx, pr.MergerID)
			if err != nil {
				if !IsErrUserNotExist(err) {
					return errors.Wrapf(err, "get merger %d", pr.MergerID)
				}
				pr.MergerID = -1
				pr.Merger = NewGhostUser()
			}
		}
	}
	return nil
}

// Update saves all fields of the pull request.
func (s *PullRequestsStore) Update(ctx context.Context, pr *PullRequest) error {
	return s.db.WithContext(ctx).Model(pr).Select("*").Updates(pr).Error
}

// updateStatus saves the status of the pull request.
func (s *PullRequestsStore) updateStatus(ctx context.Context, pr *PullRequest) error {
	return s.db.WithContext(ctx).Model(pr).Update("status", pr.Status).Error
}

// UpdatePatch saves the merge base and the patch of the pull request
// between its base and head branches.
func (s *PullRequestsStore) UpdatePatch(ctx context.Context, logger *logx.Logger, pr *PullRequest) (err error) {
	// Stopping midway, e.g., when the client disconnects, would leave a
	// partial change behind.
	ctx = context.WithoutCancel(ctx)

	headGitRepo, err := git.Open(pr.HeadRepo.RepoPath())
	if err != nil {
		return errors.Newf("open repository: %v", err)
	}

	// Add a temporary remote.
	tmpRemote := strconv.FormatInt(time.Now().UnixNano(), 10)
	baseRepoPath := RepoPath(pr.BaseRepo.MustOwner().Name, pr.BaseRepo.Name)
	err = headGitRepo.RemoteAdd(ctx, tmpRemote, baseRepoPath, git.RemoteAddOptions{Fetch: true})
	if err != nil {
		return errors.Newf("add remote %q [repo_id: %d]: %v", tmpRemote, pr.HeadRepoID, err)
	}
	defer func() {
		if err := headGitRepo.RemoteRemove(ctx, tmpRemote); err != nil {
			logger.Error("Failed to remove temporary remote", "remote", tmpRemote, "repoID", pr.HeadRepoID, "error", err)
		}
	}()

	remoteBranch := "remotes/" + tmpRemote + "/" + pr.BaseBranch
	pr.MergeBase, err = headGitRepo.MergeBase(ctx, remoteBranch, pr.HeadBranch)
	if err != nil {
		return errors.Newf("get merge base: %v", err)
	} else if err = s.Update(ctx, pr); err != nil {
		return errors.Wrap(err, "update pull request")
	}

	patch, err := headGitRepo.DiffBinary(ctx, pr.MergeBase, pr.HeadBranch)
	if err != nil {
		return errors.Newf("get binary patch: %v", err)
	}

	if err = pr.BaseRepo.SavePatch(pr.Index, patch); err != nil {
		return errors.Newf("save patch: %v", err)
	}

	logger.Debug("Patch saved", "pullRequestID", pr.ID)
	return nil
}

// AddToTaskQueue marks the pull request as being checked and queues testing
// its patch.
func (s *PullRequestsStore) AddToTaskQueue(ctx context.Context, logger *logx.Logger, pr *PullRequest) {
	ctx = context.WithoutCancel(ctx)
	go PullRequestQueue.AddFunc(pr.ID, func() {
		pr.Status = PullRequestStatusChecking
		if err := s.updateStatus(ctx, pr); err != nil {
			logger.Error("Failed to update pull request status", "pullRequestID", pr.ID, "error", err)
		}
	})
}

// addHeadRepoTasks updates the patches of given pull requests, pushes their
// head branches to their base repositories and queues testing them. Pull
// requests whose head repository is not loaded are skipped.
func (s *PullRequestsStore) addHeadRepoTasks(ctx context.Context, logger *logx.Logger, prs []*PullRequest) {
	for _, pr := range prs {
		if pr.HeadRepo == nil {
			logger.Debug("Skipped pull request with missing head repository", "pullRequestID", pr.ID)
			continue
		}

		logger.Debug("Composing new test task", "pullRequestID", pr.ID)
		if err := s.UpdatePatch(ctx, logger, pr); err != nil {
			logger.Error("Failed to update patch", "error", err)
			continue
		} else if err := pr.PushToBaseRepo(ctx, logger); err != nil {
			logger.Error("Failed to push to base repository", "error", err)
			continue
		}

		s.AddToTaskQueue(ctx, logger, pr)
	}
}

// checkAndUpdateStatus saves the status of the tested pull request, which is
// mergeable when the test found no conflict, unless it is queued to be
// tested again.
func (s *PullRequestsStore) checkAndUpdateStatus(ctx context.Context, logger *logx.Logger, pr *PullRequest) {
	if pr.Status == PullRequestStatusChecking {
		pr.Status = PullRequestStatusMergeable
	}

	if !PullRequestQueue.Exist(pr.ID) {
		if err := s.updateStatus(ctx, pr); err != nil {
			logger.Error("Failed to update pull request status", "pullRequestID", pr.ID, "error", err)
		}
	}
}
