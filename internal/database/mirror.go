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
	"unknwon.dev/ini/v2"
	"xorm.io/xorm"

	"gogs.io/gogs/internal/applog"
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
	Repo        *Repository `xorm:"-" json:"-" gorm:"-"`
	Interval    int         // Hour.
	EnablePrune bool        `xorm:"NOT NULL DEFAULT true"`

	// Last and next sync time of Git data from upstream
	LastSync     time.Time `xorm:"-" json:"-" gorm:"-"`
	LastSyncUnix int64     `xorm:"updated_unix"`
	NextSync     time.Time `xorm:"-" json:"-" gorm:"-"`
	NextSyncUnix int64     `xorm:"next_update_unix"`

	address string `xorm:"-"`
}

func (m *Mirror) BeforeInsert() {
	m.NextSyncUnix = m.NextSync.Unix()
}

func (m *Mirror) BeforeUpdate() {
	m.LastSyncUnix = m.LastSync.Unix()
	m.NextSyncUnix = m.NextSync.Unix()
}

func (m *Mirror) AfterSet(colName string, _ xorm.Cell) {
	var err error
	switch colName {
	case "repo_id":
		m.Repo, err = GetRepositoryByID(m.RepoID)
		if err != nil {
			applog.Logger.Errorf("GetRepositoryByID [%d]: %v", m.ID, err)
		}
	case "updated_unix":
		m.LastSync = time.Unix(m.LastSyncUnix, 0).Local()
	case "next_update_unix":
		m.NextSync = time.Unix(m.NextSyncUnix, 0).Local()
	}
}

// ScheduleNextSync calculates and sets next sync time based on repository mirror setting.
func (m *Mirror) ScheduleNextSync() {
	m.NextSync = time.Now().Add(time.Duration(m.Interval) * time.Hour)
}

func (m *Mirror) readAddress() {
	if len(m.address) > 0 {
		return
	}

	cfg, err := ini.Load(m.Repo.GitConfigPath(), ini.IgnoreInlineComment(true))
	if err != nil {
		applog.Logger.Errorf("load config: %v", err)
		return
	}
	m.address = cfg.Subsection("remote", "origin").Key("url").Value()
}

// HandleMirrorCredentials replaces user credentials from HTTP/HTTPS URL
// with placeholder <credentials>.
// It returns original string if protocol is not HTTP/HTTPS.
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
	m.readAddress()
	return HandleMirrorCredentials(m.address, false)
}

// MosaicsAddress returns mirror address from Git repository config with credentials under mosaics.
func (m *Mirror) MosaicsAddress() string {
	m.readAddress()
	return HandleMirrorCredentials(m.address, true)
}

// RawAddress returns raw mirror address directly from Git repository config.
func (m *Mirror) RawAddress() string {
	m.readAddress()
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

const gitShortEmptyID = "0000000"

// mirrorSyncResult contains information of a updated reference.
// If the oldCommitID is "0000000", it means a new reference, the value of newCommitID is empty.
// If the newCommitID is "0000000", it means the reference is deleted, the value of oldCommitID is empty.
type mirrorSyncResult struct {
	refName     string
	oldCommitID string
	newCommitID string
}

// parseRemoteUpdateOutput detects create, update and delete operations of references from upstream.
func parseRemoteUpdateOutput(output string) []*mirrorSyncResult {
	results := make([]*mirrorSyncResult, 0, 3)
	lines := strings.Split(output, "\n")
	for i := range lines {
		// Make sure reference name is presented before continue
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
				applog.Logger.Errorf("SHA delimiter not found: %q", lines[i])
				continue
			}
			shas := strings.Split(lines[i][3:delimIdx+3], "..")
			if len(shas) != 2 {
				applog.Logger.Errorf("Expect two SHAs but not what found: %q", lines[i])
				continue
			}
			results = append(results, &mirrorSyncResult{
				refName:     refName,
				oldCommitID: shas[0],
				newCommitID: shas[1],
			})

		default:
			applog.Logger.Warnf("parseRemoteUpdateOutput: unexpected update line %q", lines[i])
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
func (m *Mirror) runSync() ([]*mirrorSyncResult, bool) {
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
		applog.Logger.Errorf("Mirror.runSync: %s", desc)
		if err := Handle.Notices().Create(context.TODO(), NoticeTypeRepository, desc); err != nil {
			applog.Logger.Errorf("CreateRepositoryNotice: %v", err)
		}
		return nil, false
	}

	// Do a fast-fail testing against on repository URL to ensure it is accessible under
	// good condition to prevent long blocking on URL resolution without syncing anything.
	if !isMirrorURLAccessible(time.Minute, rawAddr) {
		desc := fmt.Sprintf("Source URL of mirror repository '%s' is not accessible: %s", m.Repo.FullName(), m.MosaicsAddress())
		if err := Handle.Notices().Create(context.TODO(), NoticeTypeRepository, desc); err != nil {
			applog.Logger.Errorf("CreateRepositoryNotice: %v", err)
		}
		return nil, false
	}

	output, err := mirrorRemoteUpdate(timeout, repoPath, m.EnablePrune)
	if err != nil {
		const fmtStr = "Failed to update mirror repository %q: %v"
		applog.Logger.Errorf(fmtStr, repoPath, err)
		if err = Handle.Notices().Create(
			context.TODO(),
			NoticeTypeRepository,
			fmt.Sprintf(fmtStr, repoPath, err),
		); err != nil {
			applog.Logger.Errorf("CreateRepositoryNotice: %v", err)
		}
		return nil, false
	}

	if err := m.Repo.UpdateSize(context.Background()); err != nil {
		applog.Logger.Errorf("UpdateSize [repo_id: %d]: %v", m.Repo.ID, err)
	}

	if m.Repo.HasWiki() {
		// Even if wiki sync failed, we still want results from the main repository
		if _, err := mirrorRemoteUpdate(timeout, wikiPath, true); err != nil {
			const fmtStr = "Failed to update mirror wiki repository %q: %v"
			applog.Logger.Errorf(fmtStr, wikiPath, err)
			if err = Handle.Notices().Create(
				context.TODO(),
				NoticeTypeRepository,
				fmt.Sprintf(fmtStr, wikiPath, err),
			); err != nil {
				applog.Logger.Errorf("CreateRepositoryNotice: %v", err)
			}
		}
	}

	return parseRemoteUpdateOutput(output), true
}

func getMirrorByRepoID(e Engine, repoID int64) (*Mirror, error) {
	m := &Mirror{RepoID: repoID}
	has, err := e.Get(m)
	if err != nil {
		return nil, err
	} else if !has {
		return nil, MirrorNotExist{RepoID: repoID}
	}
	return m, nil
}

// GetMirrorByRepoID returns mirror information of a repository.
func GetMirrorByRepoID(repoID int64) (*Mirror, error) {
	return getMirrorByRepoID(x, repoID)
}

func updateMirror(e Engine, m *Mirror) error {
	_, err := e.ID(m.ID).AllCols().Update(m)
	return err
}

func UpdateMirror(m *Mirror) error {
	return updateMirror(x, m)
}

func DeleteMirrorByRepoID(repoID int64) error {
	_, err := x.Delete(&Mirror{RepoID: repoID})
	return err
}

// MirrorUpdate checks and updates mirror repositories.
func MirrorUpdate() {
	if taskStatusTable.IsRunning(taskNameMirrorUpdate) {
		return
	}
	taskStatusTable.Start(taskNameMirrorUpdate)
	defer taskStatusTable.Stop(taskNameMirrorUpdate)

	applog.Logger.Tracef("Doing: MirrorUpdate")

	if err := x.Where("next_update_unix<=?", time.Now().Unix()).Iterate(new(Mirror), func(idx int, bean any) error {
		m := bean.(*Mirror)
		if m.Repo == nil {
			applog.Logger.Errorf("Disconnected mirror repository found: %d", m.ID)
			return nil
		}

		MirrorQueue.Add(m.RepoID)
		return nil
	}); err != nil {
		applog.Logger.Errorf("MirrorUpdate: %v", err)
	}
}

// SyncMirrors checks and syncs mirrors.
// TODO: sync more mirrors at same time.
func SyncMirrors() {
	ctx := context.Background()

	// Start listening on new sync requests.
	for repoID := range MirrorQueue.Queue() {
		applog.Logger.Tracef("SyncMirrors [repo_id: %s]", repoID)
		MirrorQueue.Remove(repoID)

		id, _ := strconv.ParseInt(repoID, 10, 64)
		m, err := GetMirrorByRepoID(id)
		if err != nil {
			applog.Logger.Errorf("GetMirrorByRepoID [%v]: %v", repoID, err)
			continue
		}

		results, ok := m.runSync()
		if !ok {
			continue
		}

		m.ScheduleNextSync()
		if err = UpdateMirror(m); err != nil {
			applog.Logger.Errorf("UpdateMirror [%d]: %v", m.RepoID, err)
			continue
		}

		// Pushes clear the empty state of a repository, but a mirror of an empty
		// repository only receives commits through syncs.
		if m.Repo.IsEmpty {
			hasBranches, err := git.RepoHasBranches(ctx, m.Repo.RepoPath())
			if err != nil {
				applog.Logger.Errorf("Failed to check if repository has branches [repo_id: %d]: %v", m.RepoID, err)
			} else if hasBranches {
				if err = Handle.Repositories().Touch(ctx, m.RepoID); err != nil {
					applog.Logger.Errorf("Failed to touch repository [repo_id: %d]: %v", m.RepoID, err)
				} else {
					m.Repo.IsEmpty = false
				}
			}
		}

		if !m.Repo.IsEmpty && !git.RepoHasBranch(ctx, m.Repo.RepoPath(), m.Repo.DefaultBranch) {
			branch, err := remoteDefaultBranch(time.Duration(conf.Git.Timeout.Mirror)*time.Second, m.RawAddress())
			if err != nil {
				applog.Logger.Errorf("Failed to get default branch of mirror source [repo_id: %d]: %v", m.RepoID, err)
			} else if err = m.Repo.setDefaultBranchIfMissing(ctx, branch); err != nil {
				applog.Logger.Errorf("Failed to set default branch [repo_id: %d]: %v", m.RepoID, err)
			}
		}

		// TODO:
		// - Create "Mirror Sync" webhook event
		// - Create mirror sync (create, push and delete) events and trigger the "mirror sync" webhooks

		if len(results) == 0 {
			applog.Logger.Tracef("SyncMirrors [repo_id: %d]: no commits fetched", m.RepoID)
		}

		gitRepo, err := git.Open(m.Repo.RepoPath())
		if err != nil {
			applog.Logger.Errorf("Failed to open repository [repo_id: %d]: %v", m.RepoID, err)
			continue
		}

		for _, result := range results {
			// Discard GitHub pull requests, i.e. refs/pull/*
			if strings.HasPrefix(result.refName, "refs/pull/") {
				continue
			}

			// Delete reference
			if result.newCommitID == gitShortEmptyID {
				if err = Handle.Actions().MirrorSyncDelete(ctx, m.Repo.MustOwner(), m.Repo, result.refName); err != nil {
					applog.Logger.Errorf("Failed to create action for mirror sync delete [repo_id: %d]: %v", m.RepoID, err)
				}
				continue
			}

			// New reference
			isNewRef := false
			if result.oldCommitID == gitShortEmptyID {
				if err = Handle.Actions().MirrorSyncCreate(ctx, m.Repo.MustOwner(), m.Repo, result.refName); err != nil {
					applog.Logger.Errorf("Failed to create action for mirror sync create [repo_id: %d]: %v", m.RepoID, err)
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
					applog.Logger.Errorf("Failed to parse revision [repo_id: %d, old_commit_id: %s]: %v", m.RepoID, result.oldCommitID, err)
					continue
				}
				newCommitID, err = gitRepo.RevParse(ctx, result.newCommitID)
				if err != nil {
					applog.Logger.Errorf("Failed to parse revision [repo_id: %d, new_commit_id: %s]: %v", m.RepoID, result.newCommitID, err)
					continue
				}
				commits, err = gitRepo.RevList(ctx, []string{oldCommitID + "..." + newCommitID})
				if err != nil {
					applog.Logger.Errorf("Failed to list commits [repo_id: %d, old_commit_id: %s, new_commit_id: %s]: %v", m.RepoID, oldCommitID, newCommitID, err)
					continue
				}

			} else if gitRepo.HasBranch(ctx, result.refName) {
				refNewCommit, err := gitRepo.BranchCommit(ctx, result.refName)
				if err != nil {
					applog.Logger.Errorf("Failed to get branch commit [repo_id: %d, branch: %s]: %v", m.RepoID, result.refName, err)
					continue
				}

				// The commits the branch adds since it forked from the default branch, or
				// all of its commits when it adds none, e.g., it is the default branch,
				// or they share no history, which is what the compare link of the feed
				// shows.
				rev := refNewCommit.ID.String()
				mergeBase, err := gitRepo.MergeBase(ctx, git.RefsHeads+m.Repo.DefaultBranch, rev)
				if err != nil && !git.IsErrNoMergeBase(err) {
					applog.Logger.Errorf("Failed to get merge base [repo_id: %d, commit_id: %s]: %v", m.RepoID, refNewCommit.ID, err)
					continue
				} else if err == nil && mergeBase != rev {
					rev = mergeBase + ".." + rev
				}
				newRefCommitsCount, err = gitRepo.RevListCount(ctx, []string{rev})
				if err != nil {
					applog.Logger.Errorf("Failed to count commits [repo_id: %d, rev: %s]: %v", m.RepoID, rev, err)
					continue
				}
				commits, err = gitRepo.Log(ctx, rev, git.LogOptions{MaxCount: 10})
				if err != nil {
					applog.Logger.Errorf("Failed to list commits [repo_id: %d, rev: %s]: %v", m.RepoID, rev, err)
					continue
				}

				objectFormat, err := gitRepo.ObjectFormat(ctx)
				if err != nil {
					applog.Logger.Errorf("Failed to get object format [repo_id: %d]: %v", m.RepoID, err)
					continue
				}
				oldCommitID = objectFormat.EmptyID()
				newCommitID = refNewCommit.ID.String()
			}

			pushCommits := CommitsToPushCommits(commits)
			if newRefCommitsCount > 0 && newRefCommitsCount <= math.MaxInt {
				pushCommits.Len = int(newRefCommitsCount)
			}
			err = Handle.Actions().MirrorSyncPush(ctx,
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
				applog.Logger.Errorf("Failed to create action for mirror sync push [repo_id: %d]: %v", m.RepoID, err)
				continue
			}
		}

		if _, err = x.Exec("UPDATE mirror SET updated_unix = ? WHERE repo_id = ?", time.Now().Unix(), m.RepoID); err != nil {
			applog.Logger.Errorf("Update 'mirror.updated_unix' [%d]: %v", m.RepoID, err)
			continue
		}

		if m.Repo.IsEmpty {
			continue
		}

		// Get latest commit date and compare to current repository updated time,
		// update if latest commit date is newer.
		latestCommitTime, err := gitRepo.LatestCommitTime(ctx)
		if err != nil {
			applog.Logger.Errorf("GetLatestCommitDate [%d]: %v", m.RepoID, err)
			continue
		} else if !latestCommitTime.After(m.Repo.Updated) {
			continue
		}

		if _, err = x.Exec("UPDATE repository SET updated_unix = ? WHERE id = ?", latestCommitTime.Unix(), m.RepoID); err != nil {
			applog.Logger.Errorf("Update 'repository.updated_unix' [%d]: %v", m.RepoID, err)
			continue
		}
	}
}

func InitSyncMirrors() {
	go SyncMirrors()
}
