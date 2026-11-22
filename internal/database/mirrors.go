package database

import (
	"context"
	"fmt"
	"math"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"unknwon.dev/ini/v2"
	"unknwon.dev/x/logx"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/git"
	"gogs.io/gogs/internal/netx"
	"gogs.io/gogs/internal/sync"
)

var MirrorQueue = sync.NewUniqueQueue(1000)

// MirrorNotExist represents an error when mirror does not exist.
type MirrorNotExist struct {
	RepoID int64
}

// IsMirrorNotExist returns true if the error is MirrorNotExist.
func IsMirrorNotExist(err error) bool {
	_, ok := err.(MirrorNotExist)
	return ok
}

func (err MirrorNotExist) Error() string {
	return fmt.Sprintf("mirror does not exist [repo_id: %d]", err.RepoID)
}

// Mirror represents mirror information of a repository.
type Mirror struct {
	ID          int64
	RepoID      int64
	Repo        *Repository `json:"-" gorm:"-"`
	Interval    int         // Hour.
	EnablePrune bool        `gorm:"not null;default:TRUE"`

	// Last and next sync time of Git data from upstream.
	LastSync     time.Time `json:"-" gorm:"-"`
	LastSyncUnix int64     `gorm:"column:updated_unix"`
	NextSync     time.Time `json:"-" gorm:"-"`
	NextSyncUnix int64     `gorm:"column:next_update_unix"`

	address string
}

// BeforeCreate implements the GORM create hook.
func (m *Mirror) BeforeCreate(_ *gorm.DB) error {
	if m.NextSyncUnix == 0 {
		m.NextSyncUnix = m.NextSync.Unix()
	}
	return nil
}

// BeforeUpdate implements the GORM update hook.
func (m *Mirror) BeforeUpdate(_ *gorm.DB) error {
	m.LastSyncUnix = m.LastSync.Unix()
	m.NextSyncUnix = m.NextSync.Unix()
	return nil
}

// AfterFind implements the GORM query hook.
func (m *Mirror) AfterFind(_ *gorm.DB) error {
	m.LastSync = time.Unix(m.LastSyncUnix, 0).Local()
	m.NextSync = time.Unix(m.NextSyncUnix, 0).Local()
	return nil
}

func (m *Mirror) readAddress(logger *logx.Logger) {
	if len(m.address) > 0 {
		return
	}

	cfg, err := ini.Load(m.Repo.GitConfigPath(), ini.IgnoreInlineComment(true))
	if err != nil {
		logger.Error("Failed to load Git config", "error", err)
		return
	}
	m.address = cfg.Subsection("remote", "origin").Key("url").Value()
}

// HandleMirrorCredentials replaces user credentials in the URL with the
// placeholder <credentials> when mosaics is true, and removes them otherwise.
// It returns the original string if the URL has no scheme or no "@".
//
// TODO(unknwon): Use url.Parse.
func HandleMirrorCredentials(url string, mosaics bool) string {
	i := strings.Index(url, "@")
	if i == -1 {
		return url
	}
	start := strings.Index(url, "://")
	if start == -1 {
		return url
	}
	if mosaics {
		return url[:start+3] + "<credentials>" + url[i:]
	}
	return url[:start+3] + url[i+1:]
}

// Address returns mirror address from Git repository config without credentials.
func (m *Mirror) Address() string {
	m.readAddress(handleLogger())
	return HandleMirrorCredentials(m.address, false)
}

// MosaicsAddress returns mirror address from Git repository config with credentials under mosaics.
func (m *Mirror) MosaicsAddress() string {
	m.readAddress(handleLogger())
	return HandleMirrorCredentials(m.address, true)
}

// RawAddress returns raw mirror address directly from Git repository config.
func (m *Mirror) RawAddress() string {
	m.readAddress(handleLogger())
	return m.address
}

// SaveAddress writes new address to Git repository config.
func (m *Mirror) SaveAddress(ctx context.Context, addr string) error {
	gitRepo, err := git.Open(m.Repo.RepoPath())
	if err != nil {
		return errors.Wrap(err, "open repository")
	}

	addrURL, err := url.Parse(addr)
	if err != nil {
		return err
	}

	// Replace the URL in a single config write, so an interrupted request can never
	// leave the mirror without an "origin" remote.
	err = gitRepo.RemoteSetURL(ctx, "origin", addrURL.String())
	if errors.Is(err, git.ErrRemoteNotExist) {
		// Add a missing "origin" back, so that a mirror that lost it can be repaired.
		err = gitRepo.RemoteAdd(ctx, "origin", addrURL.String(), git.RemoteAddOptions{MirrorFetch: true})
	}
	if err != nil {
		return errors.Wrap(err, "set URL of remote 'origin'")
	}
	return nil
}

// MirrorsStore is the storage layer for the sync settings of mirror
// repositories.
type MirrorsStore struct {
	db *gorm.DB
}

func newMirrorsStore(db *gorm.DB) *MirrorsStore {
	return &MirrorsStore{db: db}
}

// GetByRepoID returns the mirror of the repository with given ID, with the
// repository and its attributes. It returns MirrorNotExist when not found.
func (s *MirrorsStore) GetByRepoID(ctx context.Context, repoID int64) (*Mirror, error) {
	m := new(Mirror)
	err := s.db.WithContext(ctx).Where("repo_id = ?", repoID).First(m).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, MirrorNotExist{RepoID: repoID}
		}
		return nil, err
	}

	m.Repo, err = getRepositoryByID(s.db.WithContext(ctx), repoID)
	if err != nil {
		return nil, errors.Wrap(err, "get repository")
	}
	return m, nil
}

// LoadForRepos sets the mirrors of given repositories that are mirrors.
// Repositories without a mirror are left with none.
func (s *MirrorsStore) LoadForRepos(ctx context.Context, repos ...*Repository) error {
	repoIDs := make([]int64, 0, len(repos))
	for _, repo := range repos {
		if repo.IsMirror {
			repoIDs = append(repoIDs, repo.ID)
		}
	}
	if len(repoIDs) == 0 {
		return nil
	}

	var mirrors []*Mirror
	err := s.db.WithContext(ctx).Where("repo_id IN ?", repoIDs).Find(&mirrors).Error
	if err != nil {
		return errors.Wrap(err, "list mirrors")
	}
	byRepoID := make(map[int64]*Mirror, len(mirrors))
	for _, m := range mirrors {
		byRepoID[m.RepoID] = m
	}
	for _, repo := range repos {
		if !repo.IsMirror {
			continue
		}
		m := byRepoID[repo.ID]
		if m != nil {
			m.Repo = repo
		}
		repo.Mirror = m
	}
	return nil
}

// Update saves all fields of the mirror.
func (s *MirrorsStore) Update(ctx context.Context, m *Mirror) error {
	return s.db.WithContext(ctx).Model(m).Select("*").Updates(m).Error
}

// DeleteByRepoID deletes the mirror of the repository with given ID, which
// makes it a regular repository. Deleting a mirror that does not exist is not
// an error.
func (s *MirrorsStore) DeleteByRepoID(ctx context.Context, repoID int64) error {
	return s.db.WithContext(ctx).Where("repo_id = ?", repoID).Delete(new(Mirror)).Error
}

// listDueRepoIDs returns the IDs of the repositories whose mirrors are due to
// sync at given time. Mirrors whose repository is gone are logged and skipped.
func (s *MirrorsStore) listDueRepoIDs(ctx context.Context, logger *logx.Logger, now time.Time) ([]int64, error) {
	db := s.db.WithContext(ctx)
	var mirrors []*Mirror
	err := db.Where("next_update_unix <= ?", now.Unix()).Order("next_update_unix ASC").Find(&mirrors).Error
	if err != nil {
		return nil, err
	}
	if len(mirrors) == 0 {
		return nil, nil
	}

	// A subquery, because there can be more due mirrors than a statement can
	// bind parameters. It selects all mirrors, so that a mirror that finished
	// syncing since the query above is not taken for one without repository.
	var existing []int64
	err = db.Model(new(Repository)).Where("id IN (?)", db.Model(new(Mirror)).Select("repo_id")).Pluck("id", &existing).Error
	if err != nil {
		return nil, errors.Wrap(err, "list repositories")
	}
	exists := make(map[int64]bool, len(existing))
	for _, id := range existing {
		exists[id] = true
	}

	dueRepoIDs := make([]int64, 0, len(mirrors))
	for _, m := range mirrors {
		if !exists[m.RepoID] {
			logger.Error("Disconnected mirror repository found", "mirrorID", m.ID)
			continue
		}
		dueRepoIDs = append(dueRepoIDs, m.RepoID)
	}
	return dueRepoIDs, nil
}

// scheduleNextSync sets the next sync of the mirror of the repository with
// given ID to one interval after given time. Only that column is written, so
// settings saved while the mirror synced are kept. When the interval changed
// since it was read, the save that changed it already set the next sync.
func (s *MirrorsStore) scheduleNextSync(ctx context.Context, repoID int64, after time.Time) error {
	db := s.db.WithContext(ctx)
	m := new(Mirror)
	err := db.Where("repo_id = ?", repoID).Take(m).Error
	if err != nil {
		return errors.Wrap(err, "get mirror")
	}
	// Computed in Go, because the interval column is 32-bit on PostgreSQL.
	nextSync := after.Unix() + int64(m.Interval)*60*60
	return db.Model(new(Mirror)).
		Where("repo_id = ? AND ? = ?", repoID, clause.Column{Name: "interval"}, m.Interval).
		UpdateColumn("next_update_unix", nextSync).
		Error
}

// markSynced records that the mirror of the repository with given ID synced at
// given time.
func (s *MirrorsStore) markSynced(ctx context.Context, repoID int64, at time.Time) error {
	return s.db.WithContext(ctx).
		Model(new(Mirror)).
		Where("repo_id = ?", repoID).
		UpdateColumn("updated_unix", at.Unix()).
		Error
}

const gitShortEmptyID = "0000000"

// mirrorSyncResult contains information of an updated reference.
// If the oldCommitID is "0000000", it means a new reference, the value of newCommitID is empty.
// If the newCommitID is "0000000", it means the reference is deleted, the value of oldCommitID is empty.
type mirrorSyncResult struct {
	refName     string
	oldCommitID string
	newCommitID string
}

// parseRemoteUpdateOutput detects create, update and delete operations of references from upstream.
func parseRemoteUpdateOutput(logger *logx.Logger, output string) []*mirrorSyncResult {
	results := make([]*mirrorSyncResult, 0, 3)
	lines := strings.Split(output, "\n")
	for i := range lines {
		// Make sure reference name is present before continuing.
		idx := strings.Index(lines[i], "-> ")
		if idx == -1 {
			continue
		}

		refName := lines[i][idx+3:]
		switch {
		case strings.HasPrefix(lines[i], " * "): // New reference
			results = append(results, &mirrorSyncResult{
				refName:     refName,
				oldCommitID: gitShortEmptyID,
			})
		case strings.HasPrefix(lines[i], " - "): // Delete reference
			results = append(results, &mirrorSyncResult{
				refName:     refName,
				newCommitID: gitShortEmptyID,
			})
		case strings.HasPrefix(lines[i], "   "): // New commits of a reference
			delimIdx := strings.Index(lines[i][3:], " ")
			if delimIdx == -1 {
				logger.Error("SHA delimiter not found", "line", lines[i])
				continue
			}
			shas := strings.Split(lines[i][3:delimIdx+3], "..")
			if len(shas) != 2 {
				logger.Error("Expected two SHAs in update line", "line", lines[i])
				continue
			}
			results = append(results, &mirrorSyncResult{
				refName:     refName,
				oldCommitID: shas[0],
				newCommitID: shas[1],
			})

		default:
			logger.Warn("Unexpected update line", "line", lines[i])
		}
	}
	return results
}

// mirrorGitEnv returns the environment variables used by every remote network
// operation against a mirror source.
func mirrorGitEnv() []string {
	// Append to the Git configuration that the server environment may already pass
	// through GIT_CONFIG_* (e.g., "http.proxy"), instead of replacing it.
	existingGitConfigCount := 0
	if count, err := strconv.Atoi(os.Getenv("GIT_CONFIG_COUNT")); err == nil && count > 0 {
		existingGitConfigCount = count
	}
	return []string{
		// Disabling HTTP redirects prevents an attacker-controlled public URL from
		// redirecting to an internal endpoint that the up-front clone address
		// validation would otherwise have blocked.
		"GIT_CONFIG_COUNT=" + strconv.Itoa(existingGitConfigCount+1),
		"GIT_CONFIG_KEY_" + strconv.Itoa(existingGitConfigCount) + "=http.followRedirects",
		"GIT_CONFIG_VALUE_" + strconv.Itoa(existingGitConfigCount) + "=false",

		// Keep git non-interactive during mirror operations. Without these, a
		// network failure or a missing remote endpoint can make git ask for
		// credentials and stall the server-side process waiting on a terminal that
		// never responds.
		"GIT_TERMINAL_PROMPT=0",
		"GIT_ASKPASS=/bin/true",
		"GCM_INTERACTIVE=Never",
	}
}

// remoteDefaultBranch returns the branch that HEAD of the mirror source in
// given URL points to.
func remoteDefaultBranch(timeout time.Duration, url string) (string, error) {
	ctx, cancel := git.WithTimeout(context.Background(), timeout)
	defer cancel()
	ref, err := git.RemoteHEAD(ctx, url, git.RemoteHEADOptions{
		Envs: mirrorGitEnv(),
	})
	if err != nil {
		return "", err
	}
	return git.RefShortName(ref), nil
}

// isMirrorURLAccessible reports whether the given remote URL is reachable
// without following HTTP redirects, matching the redirect policy used by the
// mirror clone and sync.
func isMirrorURLAccessible(timeout time.Duration, url string) bool {
	ctx, cancel := git.WithTimeout(context.Background(), timeout)
	defer cancel()
	_, err := git.LsRemote(ctx, url, git.LsRemoteOptions{
		Patterns: []string{"HEAD"},
		Envs:     mirrorGitEnv(),
	})
	return err == nil
}

// mirrorRemoteUpdate fetches updates for the mirror repository in given path,
// and returns the output that reports updated references.
func mirrorRemoteUpdate(timeout time.Duration, repoPath string, prune bool) (string, error) {
	ctx, cancel := git.WithTimeout(context.Background(), timeout)
	defer cancel()
	return git.RepoRemoteUpdate(ctx, repoPath, git.RemoteUpdateOptions{
		Prune: prune,
		Envs:  mirrorGitEnv(),
	})
}

// runSync returns true if sync finished without error.
func (m *Mirror) runSync(logger *logx.Logger) ([]*mirrorSyncResult, bool) {
	repoPath := m.Repo.RepoPath()
	wikiPath := m.Repo.WikiPath()
	timeout := time.Duration(conf.Git.Timeout.Mirror) * time.Second

	// Re-check the mirror address against the local-network blocklist on every
	// sync. The address was validated when the mirror was created, but DNS for
	// that hostname may have changed in the meantime to point at an internal
	// host, so the up-front check is not sufficient on its own.
	rawAddr := m.RawAddress()
	if u, err := url.Parse(rawAddr); err == nil &&
		(u.Scheme == "http" || u.Scheme == "https" || u.Scheme == "git") &&
		netx.IsBlockedLocalHostname(u.Hostname(), conf.Security.LocalNetworkAllowlist) {
		desc := fmt.Sprintf("Source URL of mirror repository '%s' resolves to a blocked local address: %s", m.Repo.FullName(), m.MosaicsAddress())
		logger.Error("Source URL of mirror repository resolves to a blocked local address", "repo", m.Repo.FullName(), "address", m.MosaicsAddress())
		if err := Handle.Notices().Create(context.TODO(), NoticeTypeRepository, desc); err != nil {
			logger.Error("Failed to create repository notice", "error", err)
		}
		return nil, false
	}

	// Do a fast-fail test against the repository URL to ensure it is accessible,
	// to prevent long blocking on URL resolution without syncing anything.
	if !isMirrorURLAccessible(time.Minute, rawAddr) {
		desc := fmt.Sprintf("Source URL of mirror repository '%s' is not accessible: %s", m.Repo.FullName(), m.MosaicsAddress())
		if err := Handle.Notices().Create(context.TODO(), NoticeTypeRepository, desc); err != nil {
			logger.Error("Failed to create repository notice", "error", err)
		}
		return nil, false
	}

	output, err := mirrorRemoteUpdate(timeout, repoPath, m.EnablePrune)
	if err != nil {
		logger.Error("Failed to update mirror repository", "repoPath", repoPath, "error", err)
		if err = Handle.Notices().Create(
			context.TODO(),
			NoticeTypeRepository,
			fmt.Sprintf("Failed to update mirror repository %q: %v", repoPath, err),
		); err != nil {
			logger.Error("Failed to create repository notice", "error", err)
		}
		return nil, false
	}

	if err := Handle.Repositories().UpdateSize(context.Background(), m.Repo); err != nil {
		logger.Error("Failed to update repository size", "repoID", m.Repo.ID, "error", err)
	}

	if m.Repo.HasWiki() {
		// Even if wiki sync failed, we still want results from the main repository.
		if _, err := mirrorRemoteUpdate(timeout, wikiPath, true); err != nil {
			logger.Error("Failed to update mirror wiki repository", "wikiPath", wikiPath, "error", err)
			if err = Handle.Notices().Create(
				context.TODO(),
				NoticeTypeRepository,
				fmt.Sprintf("Failed to update mirror wiki repository %q: %v", wikiPath, err),
			); err != nil {
				logger.Error("Failed to create repository notice", "error", err)
			}
		}
	}

	return parseRemoteUpdateOutput(logger, output), true
}

// MirrorUpdate checks and updates mirror repositories.
func MirrorUpdate(logger *logx.Logger) {
	if taskStatusTable.IsRunning(taskNameMirrorUpdate) {
		return
	}
	taskStatusTable.Start(taskNameMirrorUpdate)
	defer taskStatusTable.Stop(taskNameMirrorUpdate)

	logger.Debug("Queueing mirrors for update")

	repoIDs, err := Handle.Mirrors().listDueRepoIDs(context.Background(), logger, time.Now())
	if err != nil {
		logger.Error("Failed to queue mirrors for update", "error", err)
	}
	for _, repoID := range repoIDs {
		MirrorQueue.Add(repoID)
	}
}

// SyncMirrors checks and syncs mirrors.
//
// TODO: sync more mirrors at same time.
func SyncMirrors(logger *logx.Logger) {
	ctx := context.Background()
	mirrors := Handle.Mirrors()

	// Start listening on new sync requests.
	for repoID := range MirrorQueue.Queue() {
		logger.Debug("Syncing mirror", "repoID", repoID)
		MirrorQueue.Remove(repoID)

		id, _ := strconv.ParseInt(repoID, 10, 64)
		m, err := mirrors.GetByRepoID(ctx, id)
		if err != nil {
			logger.Error("Failed to get mirror", "repoID", repoID, "error", err)
			continue
		}

		results, ok := m.runSync(logger)
		if !ok {
			continue
		}

		if err = mirrors.scheduleNextSync(ctx, m.RepoID, time.Now()); err != nil {
			logger.Error("Failed to schedule next sync of mirror", "repoID", m.RepoID, "error", err)
			continue
		}

		// Pushes clear the empty state of a repository, but a mirror of an empty
		// repository only receives commits through syncs.
		if m.Repo.IsEmpty {
			hasBranches, err := git.RepoHasBranches(ctx, m.Repo.RepoPath())
			if err != nil {
				logger.Error("Failed to check if repository has branches", "repoID", m.RepoID, "error", err)
			} else if hasBranches {
				if err = Handle.Repositories().Touch(ctx, m.RepoID); err != nil {
					logger.Error("Failed to touch repository", "repoID", m.RepoID, "error", err)
				} else {
					m.Repo.IsEmpty = false
				}
			}
		}

		if !m.Repo.IsEmpty && !git.RepoHasBranch(ctx, m.Repo.RepoPath(), m.Repo.DefaultBranch) {
			branch, err := remoteDefaultBranch(time.Duration(conf.Git.Timeout.Mirror)*time.Second, m.RawAddress())
			if err != nil {
				logger.Error("Failed to get default branch of mirror source", "repoID", m.RepoID, "error", err)
			} else if err = m.Repo.setDefaultBranchIfMissing(ctx, branch); err != nil {
				logger.Error("Failed to set default branch", "repoID", m.RepoID, "error", err)
			}
		}

		// TODO:
		// - Create "Mirror Sync" webhook event
		// - Create mirror sync (create, push and delete) events and trigger the "mirror sync" webhooks

		if len(results) == 0 {
			logger.Debug("No commits fetched", "repoID", m.RepoID)
		}

		gitRepo, err := git.Open(m.Repo.RepoPath())
		if err != nil {
			logger.Error("Failed to open repository", "repoID", m.RepoID, "error", err)
			continue
		}

		for _, result := range results {
			// Discard GitHub pull requests, i.e., refs/pull/*.
			if strings.HasPrefix(result.refName, "refs/pull/") {
				continue
			}

			// Delete reference
			if result.newCommitID == gitShortEmptyID {
				if err = Handle.Actions().MirrorSyncDelete(ctx, m.Repo.MustOwner(), m.Repo, result.refName); err != nil {
					logger.Error("Failed to create action for mirror sync delete", "repoID", m.RepoID, "error", err)
				}
				continue
			}

			// New reference
			isNewRef := false
			if result.oldCommitID == gitShortEmptyID {
				if err = Handle.Actions().MirrorSyncCreate(ctx, m.Repo.MustOwner(), m.Repo, result.refName); err != nil {
					logger.Error("Failed to create action for mirror sync create", "repoID", m.RepoID, "error", err)
					continue
				}
				isNewRef = true
			}

			// Push commits
			var commits []*git.Commit
			var newRefCommitsCount int64
			var oldCommitID string
			var newCommitID string
			if !isNewRef {
				oldCommitID, err = gitRepo.RevParse(ctx, result.oldCommitID)
				if err != nil {
					logger.Error("Failed to parse revision", "repoID", m.RepoID, "oldCommitID", result.oldCommitID, "error", err)
					continue
				}
				newCommitID, err = gitRepo.RevParse(ctx, result.newCommitID)
				if err != nil {
					logger.Error("Failed to parse revision", "repoID", m.RepoID, "newCommitID", result.newCommitID, "error", err)
					continue
				}
				commits, err = gitRepo.RevList(ctx, []string{oldCommitID + "..." + newCommitID})
				if err != nil {
					logger.Error("Failed to list commits", "repoID", m.RepoID, "oldCommitID", oldCommitID, "newCommitID", newCommitID, "error", err)
					continue
				}

			} else if gitRepo.HasBranch(ctx, result.refName) {
				refNewCommit, err := gitRepo.BranchCommit(ctx, result.refName)
				if err != nil {
					logger.Error("Failed to get branch commit", "repoID", m.RepoID, "branch", result.refName, "error", err)
					continue
				}

				// The commits the branch adds since it forked from the default branch, or
				// all of its commits when it adds none, e.g., it is the default branch,
				// or they share no history, which is what the compare link of the feed
				// shows.
				rev := refNewCommit.ID.String()
				mergeBase, err := gitRepo.MergeBase(ctx, git.RefsHeads+m.Repo.DefaultBranch, rev)
				if err != nil && !git.IsErrNoMergeBase(err) {
					logger.Error("Failed to get merge base", "repoID", m.RepoID, "commitID", refNewCommit.ID, "error", err)
					continue
				} else if err == nil && mergeBase != rev {
					rev = mergeBase + ".." + rev
				}
				newRefCommitsCount, err = gitRepo.RevListCount(ctx, []string{rev})
				if err != nil {
					logger.Error("Failed to count commits", "repoID", m.RepoID, "rev", rev, "error", err)
					continue
				}
				commits, err = gitRepo.Log(ctx, rev, git.LogOptions{MaxCount: 10})
				if err != nil {
					logger.Error("Failed to list commits", "repoID", m.RepoID, "rev", rev, "error", err)
					continue
				}

				objectFormat, err := gitRepo.ObjectFormat(ctx)
				if err != nil {
					logger.Error("Failed to get object format", "repoID", m.RepoID, "error", err)
					continue
				}
				oldCommitID = objectFormat.EmptyID()
				newCommitID = refNewCommit.ID.String()
			}

			pushCommits := CommitsToPushCommits(commits)
			if newRefCommitsCount > 0 && newRefCommitsCount <= math.MaxInt {
				pushCommits.Len = int(newRefCommitsCount)
			}
			err = Handle.Actions().MirrorSyncPush(ctx, logger,
				MirrorSyncPushOptions{
					Owner:       m.Repo.MustOwner(),
					Repo:        m.Repo,
					RefName:     result.refName,
					OldCommitID: oldCommitID,
					NewCommitID: newCommitID,
					Commits:     pushCommits,
				},
			)
			if err != nil {
				logger.Error("Failed to create action for mirror sync push", "repoID", m.RepoID, "error", err)
				continue
			}
		}

		if err = mirrors.markSynced(ctx, m.RepoID, time.Now()); err != nil {
			logger.Error("Failed to update mirror sync time", "repoID", m.RepoID, "error", err)
			continue
		}

		if m.Repo.IsEmpty {
			continue
		}

		// Get latest commit date and compare to current repository updated time,
		// update if latest commit date is newer.
		latestCommitTime, err := gitRepo.LatestCommitTime(ctx)
		if err != nil {
			logger.Error("Failed to get latest commit date", "repoID", m.RepoID, "error", err)
			continue
		} else if !latestCommitTime.After(m.Repo.Updated) {
			continue
		}

		if err = Handle.db.Exec("UPDATE repository SET updated_unix = ? WHERE id = ?", latestCommitTime.Unix(), m.RepoID).Error; err != nil {
			logger.Error("Failed to update repository updated time", "repoID", m.RepoID, "error", err)
			continue
		}
	}
}

func InitSyncMirrors(logger *logx.Logger) {
	go SyncMirrors(logger)
}
