package database

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/cockroachdb/errors"
	"gorm.io/gorm"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/git"
)

// PullRequestParent records that a pull request depends on another pull
// request. The changes of all parents together form the base the pull request
// is reviewed against, so a pull request can depend on several others.
type PullRequestParent struct {
	ID       int64 `gorm:"primaryKey"`
	PullID   int64 `gorm:"uniqueIndex:pull_request_parent_pull_parent_unique;not null"`
	ParentID int64 `gorm:"uniqueIndex:pull_request_parent_pull_parent_unique;index;not null"`
}

// ErrPullRequestParentCycle is returned when a parent depends on the pull
// request itself, directly or through other pull requests.
var ErrPullRequestParentCycle = errors.New("a parent depends on the pull request itself")

// PullRequestParentsStore is the storage layer for dependencies between pull
// requests.
type PullRequestParentsStore struct {
	db *gorm.DB
}

func newPullRequestParentsStore(db *gorm.DB) *PullRequestParentsStore {
	return &PullRequestParentsStore{db: db}
}

// ListParentIDs returns the IDs of the parents of the pull request in
// ascending order.
func (s *PullRequestParentsStore) ListParentIDs(ctx context.Context, pullID int64) ([]int64, error) {
	return listParentIDs(s.db.WithContext(ctx), pullID)
}

func listParentIDs(db *gorm.DB, pullID int64) ([]int64, error) {
	var ids []int64
	return ids, db.Model(&PullRequestParent{}).
		Where("pull_id = ?", pullID).
		Order("parent_id").
		Pluck("parent_id", &ids).
		Error
}

// ListChildIDs returns the IDs of the pull requests that depend on the pull
// request in ascending order.
func (s *PullRequestParentsStore) ListChildIDs(ctx context.Context, parentID int64) ([]int64, error) {
	var ids []int64
	return ids, s.db.WithContext(ctx).Model(&PullRequestParent{}).
		Where("parent_id = ?", parentID).
		Order("pull_id").
		Pluck("pull_id", &ids).
		Error
}

// Set replaces the parents of the pull request. It returns
// ErrPullRequestParentCycle when a parent depends on the pull request.
func (s *PullRequestParentsStore) Set(ctx context.Context, pullID int64, parentIDs []int64) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Walk up from the new parents. Reaching the pull request means the
		// new edges would close a cycle.
		seen := make(map[int64]bool)
		pending := append([]int64(nil), parentIDs...)
		for len(pending) > 0 {
			id := pending[len(pending)-1]
			pending = pending[:len(pending)-1]
			if id == pullID {
				return ErrPullRequestParentCycle
			} else if seen[id] {
				continue
			}
			seen[id] = true

			ids, err := listParentIDs(tx, id)
			if err != nil {
				return errors.Wrap(err, "list parent IDs")
			}
			pending = append(pending, ids...)
		}

		err := tx.Where("pull_id = ?", pullID).Delete(&PullRequestParent{}).Error
		if err != nil {
			return errors.Wrap(err, "delete parents")
		}
		parents := make([]*PullRequestParent, 0, len(parentIDs))
		added := make(map[int64]bool, len(parentIDs))
		for _, id := range parentIDs {
			if added[id] {
				continue
			}
			added[id] = true
			parents = append(parents, &PullRequestParent{PullID: pullID, ParentID: id})
		}
		if len(parents) == 0 {
			return nil
		}
		return errors.Wrap(tx.Create(parents).Error, "create parents")
	})
}

var (
	// ErrPullRequestParentsConflict is returned when the changes of the
	// parents of a pull request conflict with each other, so they have no
	// common base.
	ErrPullRequestParentsConflict = errors.New("parents have conflicting changes")
	// ErrPullRequestParentsNotMerged is returned when merging a pull request
	// before the pull requests it depends on.
	ErrPullRequestParentsNotMerged = errors.New("parents are not merged yet")
	// ErrPullRequestStackNotMergeable is returned when the pull requests a pull
	// request depends on cannot all be merged with it.
	ErrPullRequestStackNotMergeable = errors.New("the stack cannot be merged")
	// ErrPullRequestUpdateConflict is returned when the changes of a parent
	// conflict with the head branch of a pull request.
	ErrPullRequestUpdateConflict = errors.New("the changes of a parent conflict with the branch")
	// ErrPullRequestBranchProtected is returned when updating a protected
	// branch, which only takes changes through pushes that follow its rules.
	ErrPullRequestBranchProtected = errors.New("the branch is protected")
	// ErrPullRequestParentBaseMismatch is returned when a parent merges into
	// another branch than the pull request, so they cannot land together.
	ErrPullRequestParentBaseMismatch = errors.New("the parent targets another branch")
)

// loadParents returns the parents of the pull request in ascending order of
// their IDs, with their issues.
func (pr *PullRequest) loadParents(ctx context.Context) ([]*PullRequest, error) {
	ids, err := Handle.PullRequestParents().ListParentIDs(ctx, pr.ID)
	if err != nil {
		return nil, errors.Wrap(err, "list parent IDs")
	}
	parents := make([]*PullRequest, 0, len(ids))
	for _, id := range ids {
		parent, err := GetPullRequestByID(id)
		if err != nil {
			return nil, errors.Wrapf(err, "get parent %d", id)
		} else if err = parent.LoadIssue(); err != nil {
			return nil, errors.Wrapf(err, "load issue of parent %d", id)
		}
		parents = append(parents, parent)
	}
	return parents, nil
}

// UnmergedParents returns the parents of the pull request that are not merged
// yet, including closed ones.
func (pr *PullRequest) UnmergedParents(ctx context.Context) ([]*PullRequest, error) {
	parents, err := pr.loadParents(ctx)
	if err != nil {
		return nil, err
	}
	unmerged := parents[:0]
	for _, parent := range parents {
		if !parent.HasMerged {
			unmerged = append(unmerged, parent)
		}
	}
	return unmerged, nil
}

// parentHeadID returns the commit of the parent that its children build on:
// the commit it was merged at, or the head of its branch.
func parentHeadID(ctx context.Context, gitRepo *git.Repository, parent *PullRequest) (string, error) {
	if parent.HasMerged && parent.MergedCommitID != "" {
		return parent.MergedCommitID, nil
	}
	id, err := gitRepo.BranchCommitID(ctx, parent.HeadBranch)
	if err != nil {
		return "", errors.Wrapf(err, "get head of parent #%d", parent.Index)
	}
	return id, nil
}

// updateSyntheticBase merges the heads of the parents into the commit that the
// pull request is reviewed against, pins it at "refs/pull/<index>/base" so it
// is not garbage collected, and returns its ID. With one parent, the base is
// the head of that parent. Merged parents count with the commit they were
// merged at, so the pull request keeps showing only its own changes while its
// parents land one by one. The head branches of the pull request and its
// parents must be in the given repository.
func (pr *PullRequest) updateSyntheticBase(ctx context.Context, gitRepo *git.Repository, parents []*PullRequest) (string, error) {
	// A fixed signature keeps the commit ID the same for the same parents, so
	// the base only changes when a parent does.
	sig := &git.Signature{Name: "Gogs", Email: "noreply@" + conf.Server.Domain, When: time.Unix(0, 0).UTC()}

	var baseID string
	for _, parent := range parents {
		headID, err := parentHeadID(ctx, gitRepo, parent)
		if err != nil {
			return "", err
		}
		if baseID == "" {
			baseID = headID
			continue
		}

		tree, err := gitRepo.MergeTree(ctx, baseID, headID)
		if errors.Is(err, git.ErrMergeConflict) {
			return "", errors.Wrapf(ErrPullRequestParentsConflict, "merge parent #%d", parent.Index)
		} else if err != nil {
			return "", errors.Wrapf(err, "merge parent #%d", parent.Index)
		}
		message := fmt.Sprintf("Synthetic base of pull request #%d\n", pr.Index)
		baseID, err = gitRepo.CommitTree(ctx, tree, message, git.CommitTreeOptions{
			Parents:   []string{baseID, headID},
			Author:    sig,
			Committer: sig,
		})
		if err != nil {
			return "", errors.Wrap(err, "commit tree")
		}
	}

	err := gitRepo.UpdateRef(ctx, fmt.Sprintf("refs/pull/%d/base", pr.Index), baseID)
	if err != nil {
		return "", errors.Wrap(err, "update reference")
	}
	return baseID, nil
}

// BehindParents returns the parents that are not merged yet and have changes
// the head branch of the pull request does not have.
func (pr *PullRequest) BehindParents(ctx context.Context, gitRepo *git.Repository, parents []*PullRequest) ([]*PullRequest, error) {
	headID, err := gitRepo.BranchCommitID(ctx, pr.HeadBranch)
	if err != nil {
		return nil, errors.Wrap(err, "get head commit")
	}
	var behind []*PullRequest
	for _, parent := range parents {
		if parent.HasMerged {
			continue
		}
		parentID, err := parentHeadID(ctx, gitRepo, parent)
		if err != nil {
			return nil, err
		}
		contained, err := gitRepo.IsAncestor(ctx, parentID, headID)
		if err != nil {
			return nil, errors.Wrapf(err, "check if parent #%d is contained", parent.Index)
		} else if !contained {
			behind = append(behind, parent)
		}
	}
	return behind, nil
}

// mergeParentsInto merges the heads of the parents the head commit does not
// contain yet into it, with a merge commit for each, and returns the new head.
// It returns the head itself when it already contains every parent.
func mergeParentsInto(ctx context.Context, gitRepo *git.Repository, headID, branch string, parents []*PullRequest, sig *git.Signature) (string, error) {
	current := headID
	for _, parent := range parents {
		parentID, err := parentHeadID(ctx, gitRepo, parent)
		if err != nil {
			return "", err
		}
		contained, err := gitRepo.IsAncestor(ctx, parentID, current)
		if err != nil {
			return "", errors.Wrapf(err, "check if parent #%d is contained", parent.Index)
		} else if contained {
			continue
		}

		tree, err := gitRepo.MergeTree(ctx, current, parentID)
		if errors.Is(err, git.ErrMergeConflict) {
			return "", errors.Wrapf(ErrPullRequestUpdateConflict, "merge #%d", parent.Index)
		} else if err != nil {
			return "", errors.Wrapf(err, "merge #%d", parent.Index)
		}
		message := fmt.Sprintf("Merge #%d (%s) into %s\n", parent.Index, parent.HeadBranch, branch)
		current, err = gitRepo.CommitTree(ctx, tree, message, git.CommitTreeOptions{
			Parents:   []string{current, parentID},
			Author:    sig,
			Committer: sig,
		})
		if err != nil {
			return "", errors.Wrap(err, "commit tree")
		}
	}
	return current, nil
}

// UpdateBranch brings the changes of the parents that are not merged yet into
// the head branch of the pull request with merge commits, so it is reviewed
// against their latest changes. It returns false when the branch already has
// them. The head branches must be in the base repository.
func (pr *PullRequest) UpdateBranch(ctx context.Context, doer *User) (updated bool, err error) {
	// Stopping midway, e.g., when the client disconnects, would leave a
	// partial change behind.
	ctx = context.WithoutCancel(ctx)

	if err = pr.LoadAttributes(); err != nil {
		return false, errors.Wrap(err, "load attributes")
	}
	protected, err := GetProtectBranchOfRepoByName(pr.HeadRepoID, pr.HeadBranch)
	if err != nil && !IsErrBranchNotExist(err) {
		return false, errors.Wrap(err, "get protected branch")
	} else if protected != nil && protected.Protected {
		return false, ErrPullRequestBranchProtected
	}
	parents, err := pr.UnmergedParents(ctx)
	if err != nil {
		return false, errors.Wrap(err, "list unmerged parents")
	}
	gitRepo, err := git.Open(pr.HeadRepo.RepoPath())
	if err != nil {
		return false, errors.Wrap(err, "open repository")
	}
	headID, err := gitRepo.BranchCommitID(ctx, pr.HeadBranch)
	if err != nil {
		return false, errors.Wrap(err, "get head commit")
	}

	sig := &git.Signature{Name: doer.DisplayName(), Email: doer.Email}
	newID, err := mergeParentsInto(ctx, gitRepo, headID, pr.HeadBranch, parents, sig)
	if err != nil {
		return false, err
	} else if newID == headID {
		return false, nil
	}

	// Only move the branch if nobody pushed to it meanwhile.
	err = gitRepo.UpdateRef(ctx, git.RefsHeads+pr.HeadBranch, newID, git.UpdateRefOptions{OldValue: headID})
	if err != nil {
		return false, errors.Wrap(err, "update branch")
	}
	go HookQueue.Add(pr.HeadRepoID)
	// Rebuild the base and patch now, so the pull request shows the update
	// right away, and the bases of its children after it.
	AddTestPullRequestTask(doer, pr.HeadRepoID, pr.HeadBranch, true)
	return true, nil
}

// PullRequestStack is the pull requests connected to one another by
// dependencies.
type PullRequestStack struct {
	// Pulls are in topological order, parents first, and by number among pull
	// requests that do not depend on one another. Their issues are loaded.
	Pulls []*PullRequest
	// Parents maps the ID of each pull request to the IDs of its parents.
	Parents map[int64][]int64
}

// maxStackSize limits how many pull requests a stack loads, so a runaway graph
// cannot make a page load slow.
const maxStackSize = 50

// LoadPullRequestStack returns the pull requests connected to the pull request
// by dependencies, in either direction, including the pull request itself.
func LoadPullRequestStack(ctx context.Context, pr *PullRequest) (*PullRequestStack, error) {
	store := Handle.PullRequestParents()
	stack := &PullRequestStack{Parents: make(map[int64][]int64)}
	byID := map[int64]*PullRequest{pr.ID: pr}
	pending := []int64{pr.ID}
	for len(pending) > 0 && len(byID) <= maxStackSize {
		id := pending[0]
		pending = pending[1:]

		parentIDs, err := store.ListParentIDs(ctx, id)
		if err != nil {
			return nil, errors.Wrap(err, "list parent IDs")
		}
		childIDs, err := store.ListChildIDs(ctx, id)
		if err != nil {
			return nil, errors.Wrap(err, "list child IDs")
		}
		stack.Parents[id] = parentIDs
		for _, next := range append(parentIDs, childIDs...) {
			if byID[next] != nil {
				continue
			}
			p, err := GetPullRequestByID(next)
			if err != nil {
				return nil, errors.Wrapf(err, "get pull request %d", next)
			}
			byID[next] = p
			pending = append(pending, next)
		}
	}

	pulls := make([]*PullRequest, 0, len(byID))
	for id, p := range byID {
		if _, ok := stack.Parents[id]; !ok {
			// Beyond the size limit, so its edges are not loaded.
			continue
		}
		if p.Issue == nil {
			if err := p.LoadIssue(); err != nil {
				return nil, errors.Wrapf(err, "load issue of pull request %d", id)
			}
		}
		pulls = append(pulls, p)
	}
	stack.Pulls = sortPullRequestStack(pulls, stack.Parents)
	return stack, nil
}

// sortPullRequestStack orders the pull requests so that every one comes after
// its parents, and by number among those that do not depend on one another.
// Parents outside of the given pull requests are ignored.
func sortPullRequestStack(pulls []*PullRequest, parents map[int64][]int64) []*PullRequest {
	in := make(map[int64]bool, len(pulls))
	for _, p := range pulls {
		in[p.ID] = true
	}
	waiting := make(map[int64]int, len(pulls))
	children := make(map[int64][]*PullRequest)
	for _, p := range pulls {
		for _, parentID := range parents[p.ID] {
			if in[parentID] {
				waiting[p.ID]++
				children[parentID] = append(children[parentID], p)
			}
		}
	}

	var ready, sorted []*PullRequest
	for _, p := range pulls {
		if waiting[p.ID] == 0 {
			ready = append(ready, p)
		}
	}
	for len(ready) > 0 {
		sort.Slice(ready, func(i, j int) bool { return ready[i].Index < ready[j].Index })
		p := ready[0]
		ready = ready[1:]
		sorted = append(sorted, p)
		for _, child := range children[p.ID] {
			waiting[child.ID]--
			if waiting[child.ID] == 0 {
				ready = append(ready, child)
			}
		}
	}
	return sorted
}

// UnmergedAncestors returns the pull requests the pull request depends on,
// directly or through others, that are not merged yet, parents first.
func (s *PullRequestStack) UnmergedAncestors(pullID int64) []*PullRequest {
	ancestors := make(map[int64]bool)
	pending := append([]int64(nil), s.Parents[pullID]...)
	for len(pending) > 0 {
		id := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if ancestors[id] {
			continue
		}
		ancestors[id] = true
		pending = append(pending, s.Parents[id]...)
	}
	var unmerged []*PullRequest
	for _, p := range s.Pulls {
		if ancestors[p.ID] && !p.HasMerged {
			unmerged = append(unmerged, p)
		}
	}
	return unmerged
}

// Descendants returns the IDs of the pull requests that depend on the pull
// request, directly or through others.
func (s *PullRequestStack) Descendants(pullID int64) map[int64]bool {
	descendants := make(map[int64]bool)
	// Pulls are sorted parents first, so one pass finds every descendant.
	for _, p := range s.Pulls {
		for _, parentID := range s.Parents[p.ID] {
			if parentID == pullID || descendants[parentID] {
				descendants[p.ID] = true
				break
			}
		}
	}
	return descendants
}

// checkStackMerge returns an error naming the first of the pull requests, in
// order, whose head commit cannot be merged into the base commit together with
// the ones before it. The head commits are given in the same order.
func checkStackMerge(ctx context.Context, gitRepo *git.Repository, baseID string, pulls []*PullRequest, headIDs []string) error {
	current := baseID
	for i, p := range pulls {
		headID := headIDs[i]
		contained, err := gitRepo.IsAncestor(ctx, headID, current)
		if err != nil {
			return errors.Wrapf(err, "check if #%d is merged", p.Index)
		} else if contained {
			continue
		}
		tree, err := gitRepo.MergeTree(ctx, current, headID)
		if errors.Is(err, git.ErrMergeConflict) {
			return errors.Wrapf(ErrPullRequestStackNotMergeable, "#%d conflicts with the changes before it", p.Index)
		} else if err != nil {
			return errors.Wrapf(err, "merge #%d", p.Index)
		}
		current, err = gitRepo.CommitTree(ctx, tree, "Check stack merge\n", git.CommitTreeOptions{Parents: []string{current, headID}})
		if err != nil {
			return errors.Wrap(err, "commit tree")
		}
	}
	return nil
}

// MergePullRequestStack merges the pull requests the pull request depends on
// that are not merged yet, parents first, and then the pull request itself,
// each with a merge commit. It checks the whole stack merges cleanly before
// merging any of them, and returns the pull requests it merged. The pull
// requests must be in the given repository.
func MergePullRequestStack(ctx context.Context, doer *User, repo *Repository, gitRepo *git.Repository, pr *PullRequest) ([]*PullRequest, error) {
	ctx = context.WithoutCancel(ctx)

	stack, err := LoadPullRequestStack(ctx, pr)
	if err != nil {
		return nil, errors.Wrap(err, "load stack")
	}
	pulls := append(stack.UnmergedAncestors(pr.ID), pr)
	for _, p := range pulls {
		if p.Issue.IsClosed {
			return nil, errors.Wrapf(ErrPullRequestStackNotMergeable, "#%d is closed", p.Index)
		} else if p.BaseBranch != pr.BaseBranch {
			return nil, errors.Wrapf(ErrPullRequestStackNotMergeable, "#%d targets %s instead of %s", p.Index, p.BaseBranch, pr.BaseBranch)
		} else if p.HeadRepoID != repo.ID || p.BaseRepoID != repo.ID {
			return nil, errors.Wrapf(ErrPullRequestStackNotMergeable, "#%d is from another repository", p.Index)
		}
	}

	baseID, err := gitRepo.BranchCommitID(ctx, pr.BaseBranch)
	if err != nil {
		return nil, errors.Wrap(err, "get base commit")
	}
	headIDs := make([]string, len(pulls))
	for i, p := range pulls {
		if headIDs[i], err = gitRepo.BranchCommitID(ctx, p.HeadBranch); err != nil {
			return nil, errors.Wrapf(ErrPullRequestStackNotMergeable, "#%d has no head branch", p.Index)
		}
	}
	// The check writes its commits to a temporary repository that reads the
	// objects of the repository, so nothing is left behind in it.
	tmpRepo, cleanup, err := newTemporaryRepository(ctx, repo.RepoPath())
	if err != nil {
		return nil, errors.Wrap(err, "create temporary repository")
	}
	defer cleanup()
	if err = checkStackMerge(ctx, tmpRepo, baseID, pulls, headIDs); err != nil {
		return nil, err
	}

	merged := make([]*PullRequest, 0, len(pulls))
	for _, p := range pulls {
		if err = p.LoadAttributes(); err != nil {
			return merged, errors.Wrapf(err, "load attributes of #%d", p.Index)
		}
		p.Issue.Repo = repo
		if err = p.Merge(ctx, doer, gitRepo, MergeStyleRegular, ""); err != nil {
			return merged, errors.Wrapf(err, "merge #%d", p.Index)
		}
		merged = append(merged, p)
	}
	return merged, nil
}

// ListOpenLocalPullRequests returns the open pull requests of the repository
// whose head branches are in it, with their issues, by number.
func ListOpenLocalPullRequests(repoID int64) ([]*PullRequest, error) {
	prs := make([]*PullRequest, 0, 10)
	err := x.Where("pull_request.base_repo_id = ? AND pull_request.head_repo_id = ? AND pull_request.has_merged = ? AND issue.is_closed = ?",
		repoID, repoID, false, false).
		Join("INNER", "issue", "issue.id = pull_request.issue_id").
		Find(&prs)
	if err != nil {
		return nil, errors.Wrap(err, "find pull requests")
	}
	if err = PullRequestList(prs).LoadAttributes(); err != nil {
		return nil, errors.Wrap(err, "load attributes")
	}
	sort.Slice(prs, func(i, j int) bool { return prs[i].Index < prs[j].Index })
	return prs, nil
}

// SetPullRequestParents replaces the parents of the pull request and rebuilds
// the base it is reviewed against. The pull request and its parents must have
// their head branches in the base repository of the pull request.
func SetPullRequestParents(ctx context.Context, pr *PullRequest, parents []*PullRequest) error {
	for _, parent := range parents {
		if !parent.HasMerged && parent.BaseBranch != pr.BaseBranch {
			return errors.Wrapf(ErrPullRequestParentBaseMismatch, "#%d targets %s instead of %s", parent.Index, parent.BaseBranch, pr.BaseBranch)
		}
	}
	if len(parents) > 0 {
		gitRepo, err := git.Open(pr.BaseRepo.RepoPath())
		if err != nil {
			return errors.Wrap(err, "open repository")
		}
		// Fail before storing parents that cannot be combined.
		if _, err = pr.updateSyntheticBase(ctx, gitRepo, parents); err != nil {
			return err
		}
	}

	parentIDs := make([]int64, len(parents))
	for i := range parents {
		parentIDs[i] = parents[i].ID
	}
	if err := Handle.PullRequestParents().Set(ctx, pr.ID, parentIDs); err != nil {
		return err
	}

	if err := pr.UpdatePatch(ctx); err != nil {
		return errors.Wrap(err, "update patch")
	}
	pr.AddToTaskQueue()
	return nil
}

// addChildTasks rebuilds the bases of the open pull requests that depend on
// the given ones.
func addChildTasks(ctx context.Context, prs []*PullRequest) {
	var children []*PullRequest
	for _, pr := range prs {
		ids, err := Handle.PullRequestParents().ListChildIDs(ctx, pr.ID)
		if err != nil {
			log.Error("Failed to list children of pull request %d: %v", pr.ID, err)
			continue
		}
		for _, id := range ids {
			child, err := GetPullRequestByID(id)
			if err != nil {
				log.Error("Failed to get pull request %d: %v", id, err)
				continue
			} else if err = child.LoadIssue(); err != nil {
				log.Error("Failed to load issue of pull request %d: %v", id, err)
				continue
			}
			if !child.HasMerged && !child.Issue.IsClosed {
				children = append(children, child)
			}
		}
	}
	addHeadRepoTasks(children)
}
