package database

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/gogs/git-module"
	"github.com/unknwon/com"
	"gopkg.in/ini.v1"
	"gorm.io/gorm"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/process"
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
	Repo        *Repository `gorm:"-" json:"-"`
	Interval    int         // Hour.
	EnablePrune bool        `gorm:"not null;default:true"`

	// Last and next sync time of Git data from upstream
	LastSync     time.Time `gorm:"-" json:"-"`
	LastSyncUnix int64     `gorm:"column:updated_unix"`
	NextSync     time.Time `gorm:"-" json:"-"`
	NextSyncUnix int64     `gorm:"column:next_update_unix"`

	address string `gorm:"-"`
}

func (m *Mirror) BeforeCreate(tx *gorm.DB) error {
	m.NextSyncUnix = m.NextSync.Unix()
	return nil
}

func (m *Mirror) BeforeUpdate(tx *gorm.DB) error {
	m.LastSyncUnix = m.LastSync.Unix()
	m.NextSyncUnix = m.NextSync.Unix()
	return nil
}

func (m *Mirror) AfterFind(tx *gorm.DB) error {
	if m.RepoID > 0 {
		var err error
		m.Repo, err = GetRepositoryByID(m.RepoID)
		if err != nil {
			log.Error("GetRepositoryByID [%d]: %v", m.ID, err)
		}
	}
	m.LastSync = time.Unix(m.LastSyncUnix, 0).Local()
	m.NextSync = time.Unix(m.NextSyncUnix, 0).Local()
	return nil
}

// ScheduleNextSync calculates and sets next sync time based on repository mirror setting.
func (m *Mirror) ScheduleNextSync() {
	m.NextSync = time.Now().Add(time.Duration(m.Interval) * time.Hour)
}

func (m *Mirror) readAddress() {
	if len(m.address) > 0 {
		return
	}

	cfg, err := ini.LoadSources(
		ini.LoadOptions{IgnoreInlineComment: true},
		m.Repo.GitConfigPath(),
	)
	if err != nil {
		log.Error("load config: %v", err)
		return
	}
	m.address = cfg.Section("remote \"origin\"").Key("url").Value()
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
func (m *Mirror) SaveAddress(addr string) error {
	repoPath := m.Repo.RepoPath()

	err := git.RemoteRemove(repoPath, "origin")
	if err != nil {
		return errors.Newf("remove remote 'origin': %v", err)
	}

	addrURL, err := url.Parse(addr)
	if err != nil {
		return err
	}

	err = git.RemoteAdd(repoPath, "origin", addrURL.String(), git.RemoteAddOptions{MirrorFetch: true})
	if err != nil {
		return errors.Newf("add remote 'origin': %v", err)
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
				log.Error("SHA delimiter not found: %q", lines[i])
				continue
			}
			shas := strings.Split(lines[i][3:delimIdx+3], "..")
			if len(shas) != 2 {
				log.Error("Expect two SHAs but not what found: %q", lines[i])
				continue
			}
			results = append(results, &mirrorSyncResult{
				refName:     refName,
				oldCommitID: shas[0],
				newCommitID: shas[1],
			})

		default:
			log.Warn("parseRemoteUpdateOutput: unexpected update line %q", lines[i])
		}
	}
	return results
}

// runSync returns true if sync finished without error.
func (m *Mirror) runSync() ([]*mirrorSyncResult, bool) {
	repoPath := m.Repo.RepoPath()
	wikiPath := m.Repo.WikiPath()
	timeout := time.Duration(conf.Git.Timeout.Mirror) * time.Second

	// Do a fast-fail testing against on repository URL to ensure it is accessible under
	// good condition to prevent long blocking on URL resolution without syncing anything.
	if !git.IsURLAccessible(time.Minute, m.RawAddress()) {
		desc := fmt.Sprintf("Source URL of mirror repository '%s' is not accessible: %s", m.Repo.FullName(), m.MosaicsAddress())
		if err := Handle.Notices().Create(context.TODO(), NoticeTypeRepository, desc); err != nil {
			log.Error("CreateRepositoryNotice: %v", err)
		}
		return nil, false
	}

	gitArgs := []string{"remote", "update"}
	if m.EnablePrune {
		gitArgs = append(gitArgs, "--prune")
	}
	_, stderr, err := process.ExecDir(
		timeout, repoPath, fmt.Sprintf("Mirror.runSync: %s", repoPath),
		"git", gitArgs...)
	if err != nil {
		const fmtStr = "Failed to update mirror repository %q: %s"
		log.Error(fmtStr, repoPath, stderr)
		if err = Handle.Notices().Create(
			context.TODO(),
			NoticeTypeRepository,
			fmt.Sprintf(fmtStr, repoPath, stderr),
		); err != nil {
			log.Error("CreateRepositoryNotice: %v", err)
		}
		return nil, false
	}
	output := stderr

	if err := m.Repo.UpdateSize(); err != nil {
		log.Error("UpdateSize [repo_id: %d]: %v", m.Repo.ID, err)
	}

	if m.Repo.HasWiki() {
		// Even if wiki sync failed, we still want results from the main repository
		if _, stderr, err := process.ExecDir(
			timeout, wikiPath, fmt.Sprintf("Mirror.runSync: %s", wikiPath),
			"git", "remote", "update", "--prune"); err != nil {
			const fmtStr = "Failed to update mirror wiki repository %q: %s"
			log.Error(fmtStr, wikiPath, stderr)
			if err = Handle.Notices().Create(
				context.TODO(),
				NoticeTypeRepository,
				fmt.Sprintf(fmtStr, wikiPath, stderr),
			); err != nil {
				log.Error("CreateRepositoryNotice: %v", err)
			}
		}
	}

	return parseRemoteUpdateOutput(output), true
}

func getMirrorByRepoID(e *gorm.DB, repoID int64) (*Mirror, error) {
	m := &Mirror{}
	err := e.Where("repo_id = ?", repoID).First(m).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, MirrorNotExist{RepoID: repoID}
		}
		return nil, err
	}
	return m, nil
}

// GetMirrorByRepoID returns mirror information of a repository.
func GetMirrorByRepoID(repoID int64) (*Mirror, error) {
	return getMirrorByRepoID(db, repoID)
}

func updateMirror(e *gorm.DB, m *Mirror) error {
	return e.Model(m).Where("id = ?", m.ID).Updates(m).Error
}

func UpdateMirror(m *Mirror) error {
	return updateMirror(db, m)
}

func DeleteMirrorByRepoID(repoID int64) error {
	return db.Where("repo_id = ?", repoID).Delete(&Mirror{}).Error
}

// MirrorUpdate checks and updates mirror repositories.
func MirrorUpdate() {
	if taskStatusTable.IsRunning(taskNameMirrorUpdate) {
		return
	}
	taskStatusTable.Start(taskNameMirrorUpdate)
	defer taskStatusTable.Stop(taskNameMirrorUpdate)

	log.Trace("Doing: MirrorUpdate")

	var mirrors []*Mirror
	if err := db.Where("next_update_unix <= ?", time.Now().Unix()).Find(&mirrors).Error; err != nil {
		log.Error("MirrorUpdate: find mirrors: %v", err)
		return
	}

	for _, m := range mirrors {
		if m.Repo == nil {
			log.Error("Disconnected mirror repository found: %d", m.ID)
			continue
		}
		MirrorQueue.Add(m.RepoID)
	}
}

// SyncMirrors checks and syncs mirrors.
// TODO: sync more mirrors at same time.
func SyncMirrors() {
	ctx := context.Background()

	// Start listening on new sync requests.
	for repoID := range MirrorQueue.Queue() {
		log.Trace("SyncMirrors [repo_id: %s]", repoID)
		MirrorQueue.Remove(repoID)

		m, err := GetMirrorByRepoID(com.StrTo(repoID).MustInt64())
		if err != nil {
			log.Error("GetMirrorByRepoID [%v]: %v", repoID, err)
			continue
		}

		results, ok := m.runSync()
		if !ok {
			continue
		}

		m.ScheduleNextSync()
		if err = UpdateMirror(m); err != nil {
			log.Error("UpdateMirror [%d]: %v", m.RepoID, err)
			continue
		}

		// TODO:
		// - Create "Mirror Sync" webhook event
		// - Create mirror sync (create, push and delete) events and trigger the "mirror sync" webhooks

		if len(results) == 0 {
			log.Trace("SyncMirrors [repo_id: %d]: no commits fetched", m.RepoID)
		}

		gitRepo, err := git.Open(m.Repo.RepoPath())
		if err != nil {
			log.Error("Failed to open repository [repo_id: %d]: %v", m.RepoID, err)
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
					log.Error("Failed to create action for mirror sync delete [repo_id: %d]: %v", m.RepoID, err)
				}
				continue
			}

			// New reference
			isNewRef := false
			if result.oldCommitID == gitShortEmptyID {
				if err = Handle.Actions().MirrorSyncCreate(ctx, m.Repo.MustOwner(), m.Repo, result.refName); err != nil {
					log.Error("Failed to create action for mirror sync create [repo_id: %d]: %v", m.RepoID, err)
					continue
				}
				isNewRef = true
			}

			// Push commits
			var commits []*git.Commit
			var oldCommitID string
			var newCommitID string
			if !isNewRef {
				oldCommitID, err = gitRepo.RevParse(result.oldCommitID)
				if err != nil {
					log.Error("Failed to parse revision [repo_id: %d, old_commit_id: %s]: %v", m.RepoID, result.oldCommitID, err)
					continue
				}
				newCommitID, err = gitRepo.RevParse(result.newCommitID)
				if err != nil {
					log.Error("Failed to parse revision [repo_id: %d, new_commit_id: %s]: %v", m.RepoID, result.newCommitID, err)
					continue
				}
				commits, err = gitRepo.RevList([]string{oldCommitID + "..." + newCommitID})
				if err != nil {
					log.Error("Failed to list commits [repo_id: %d, old_commit_id: %s, new_commit_id: %s]: %v", m.RepoID, oldCommitID, newCommitID, err)
					continue
				}

			} else if gitRepo.HasBranch(result.refName) {
				refNewCommit, err := gitRepo.BranchCommit(result.refName)
				if err != nil {
					log.Error("Failed to get branch commit [repo_id: %d, branch: %s]: %v", m.RepoID, result.refName, err)
					continue
				}

				// TODO(unknwon): Get the commits for the new ref until the closest ancestor branch like GitHub does.
				commits, err = refNewCommit.Ancestors(git.LogOptions{MaxCount: 9})
				if err != nil {
					log.Error("Failed to get ancestors [repo_id: %d, commit_id: %s]: %v", m.RepoID, refNewCommit.ID, err)
					continue
				}

				// Put the latest commit in front of ancestors
				commits = append([]*git.Commit{refNewCommit}, commits...)

				oldCommitID = git.EmptyID
				newCommitID = refNewCommit.ID.String()
			}

			err = Handle.Actions().MirrorSyncPush(ctx,
				MirrorSyncPushOptions{
					Owner:       m.Repo.MustOwner(),
					Repo:        m.Repo,
					RefName:     result.refName,
					OldCommitID: oldCommitID,
					NewCommitID: newCommitID,
					Commits:     CommitsToPushCommits(commits),
				},
			)
			if err != nil {
				log.Error("Failed to create action for mirror sync push [repo_id: %d]: %v", m.RepoID, err)
				continue
			}
		}

		if err = db.Exec("UPDATE mirror SET updated_unix = ? WHERE repo_id = ?", time.Now().Unix(), m.RepoID).Error; err != nil {
			log.Error("Update 'mirror.updated_unix' [%d]: %v", m.RepoID, err)
			continue
		}

		// Get latest commit date and compare to current repository updated time,
		// update if latest commit date is newer.
		latestCommitTime, err := gitRepo.LatestCommitTime()
		if err != nil {
			log.Error("GetLatestCommitDate [%d]: %v", m.RepoID, err)
			continue
		} else if !latestCommitTime.After(m.Repo.Updated) {
			continue
		}

		if err = db.Exec("UPDATE repository SET updated_unix = ? WHERE id = ?", latestCommitTime.Unix(), m.RepoID).Error; err != nil {
			log.Error("Update 'repository.updated_unix' [%d]: %v", m.RepoID, err)
			continue
		}
	}
}

func InitSyncMirrors() {
	go SyncMirrors()
}
