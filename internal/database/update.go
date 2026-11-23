package database

import (
	"context"
	"strings"

	"github.com/cockroachdb/errors"
	"unknwon.dev/x/logx"

	"gogs.io/gogs/internal/git"
)

// CommitToPushCommit transforms a git.Commit to PushCommit type.
func CommitToPushCommit(commit *git.Commit) *PushCommit {
	return &PushCommit{
		Sha1:           commit.ID.String(),
		Message:        commit.Message,
		AuthorEmail:    commit.Author.Email,
		AuthorName:     commit.Author.Name,
		CommitterEmail: commit.Committer.Email,
		CommitterName:  commit.Committer.Name,
		Timestamp:      commit.Committer.When,
	}
}

func CommitsToPushCommits(commits []*git.Commit) *PushCommits {
	if len(commits) == 0 {
		return &PushCommits{}
	}

	pcs := make([]*PushCommit, len(commits))
	for i := range commits {
		pcs[i] = CommitToPushCommit(commits[i])
	}
	return &PushCommits{len(pcs), pcs, "", nil}
}

// PushOptions are the settings that pushing to repositories and recording the
// pushes read.
type PushOptions struct {
	// FeedMaxCommitNum is the most commits kept in the action, or 0 for no limit.
	FeedMaxCommitNum int
	// AppDataPath is where the temporary repositories that pushes are made from
	// are created.
	AppDataPath string
	// Notify tells about the issues that the commits close or reopen, and makes
	// the webhook payloads.
	Notify NotifyOptions
}

// PushUpdates calls PushUpdate for each reference update of a push by the
// pusher to the repository, and then calls trigger with the short name of the
// reference, e.g., TriggerPushTasks. Failures are logged, because the push has
// already happened.
func PushUpdates(ctx context.Context, logger *logx.Logger, push PushOptions, pusher *User, ownerName, repoName string, updates []git.RefUpdate, trigger func(refName string)) {
	for _, update := range updates {
		err := PushUpdate(ctx, logger, PushUpdateOptions{
			OldCommitID:      update.OldID,
			NewCommitID:      update.NewID,
			FullRefspec:      update.RefName,
			PusherID:         pusher.ID,
			PusherName:       pusher.Name,
			RepoUserName:     ownerName,
			RepoName:         repoName,
			FeedMaxCommitNum: push.FeedMaxCommitNum,
			Notify:           push.Notify,
		})
		if err != nil {
			logger.Error("Failed to update for push", "ownerName", ownerName, "repoName", repoName, "refName", update.RefName, "error", err)
		}
		trigger(git.RefShortName(update.RefName))
	}
}

// TriggerPushTasks queues the webhook deliveries of the repository and the
// tests of pull requests affected by the push to the branch. The repositories
// are under root.
func TriggerPushTasks(logger *logx.Logger, links LinkOptions, root string, pusher *User, repoID int64, branch string) {
	go HookQueue.Add(repoID)
	go AddTestPullRequestTask(logger, links, root, pusher, repoID, branch, true)
}

type PushUpdateOptions struct {
	OldCommitID  string
	NewCommitID  string
	FullRefspec  string
	PusherID     int64
	PusherName   string
	RepoUserName string
	RepoName     string
	// FeedMaxCommitNum is the most commits kept in the action, or 0 for no limit.
	FeedMaxCommitNum int
	// Notify tells about the issues that the commits close or reopen, and makes
	// the webhook payloads.
	Notify NotifyOptions
}

// PushUpdate must be called for any push actions in order to generate necessary
// push action history feeds.
func PushUpdate(ctx context.Context, logger *logx.Logger, opts PushUpdateOptions) (err error) {
	isNewRef := git.IsEmptyID(opts.OldCommitID)
	isDelRef := git.IsEmptyID(opts.NewCommitID)
	if isNewRef && isDelRef {
		return errors.New("both old and new revisions are empty")
	}

	repoPath := RepoPath(opts.Notify.RepositoryRoot, opts.RepoUserName, opts.RepoName)

	owner, err := Handle.Users().GetByUsername(ctx, opts.RepoUserName)
	if err != nil {
		return errors.Newf("GetUserByName: %v", err)
	}

	repos := Handle.Repositories()
	repo, err := repos.GetByName(ctx, owner.ID, opts.RepoName)
	if err != nil {
		return errors.Wrap(err, "get repository")
	}
	repo.Owner = owner
	if err = repos.LoadAttributes(ctx, repo); err != nil {
		return errors.Wrap(err, "load repository attributes")
	}

	if err = repos.UpdateSize(ctx, opts.Notify.RepositoryRoot, repo); err != nil {
		return errors.Newf("UpdateSize: %v", err)
	}

	if strings.HasPrefix(opts.FullRefspec, git.RefsTags) {
		err := Handle.Actions().PushTag(ctx, logger,
			PushTagOptions{
				Owner:       owner,
				Repo:        repo,
				PusherName:  opts.PusherName,
				RefFullName: opts.FullRefspec,
				NewCommitID: opts.NewCommitID,
				Links:       opts.Notify.Links,
			},
		)
		if err != nil {
			return errors.Wrap(err, "create action for push tag")
		}
		return nil
	}

	if !isDelRef && strings.HasPrefix(opts.FullRefspec, git.RefsHeads) {
		err = repo.setDefaultBranchIfMissing(ctx, opts.Notify.RepositoryRoot, git.RefShortName(opts.FullRefspec))
		if err != nil {
			return errors.Wrap(err, "set default branch")
		}
	}

	var commits []*git.Commit
	// Deleting a branch has no commits to read.
	if !isDelRef {
		newCommit, err := git.RepoCatFileCommit(ctx, repoPath, opts.NewCommitID)
		if err != nil {
			return errors.Newf("GetCommit [commit_id: %s]: %v", opts.NewCommitID, err)
		}

		if isNewRef {
			commits, err = newCommit.Ancestors(ctx, git.LogOptions{MaxCount: 9})
			if err != nil {
				return errors.Newf("CommitsBeforeLimit [commit_id: %s]: %v", newCommit.ID, err)
			}
			commits = append([]*git.Commit{newCommit}, commits...)
		} else {
			commits, err = newCommit.CommitsAfter(ctx, opts.OldCommitID)
			if err != nil {
				return errors.Newf("CommitsBeforeUntil [commit_id: %s]: %v", opts.OldCommitID, err)
			}
		}
	}

	err = Handle.Actions().CommitRepo(ctx, logger,
		CommitRepoOptions{
			Owner:            owner,
			Repo:             repo,
			PusherName:       opts.PusherName,
			RefFullName:      opts.FullRefspec,
			OldCommitID:      opts.OldCommitID,
			NewCommitID:      opts.NewCommitID,
			Commits:          CommitsToPushCommits(commits),
			FeedMaxCommitNum: opts.FeedMaxCommitNum,
			Notify:           opts.Notify,
		},
	)
	if err != nil {
		return errors.Wrap(err, "create action for commit push")
	}
	return nil
}
