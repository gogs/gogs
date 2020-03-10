// Copyright 2016 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package db

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/unknwon/com"
	"gopkg.in/ini.v1"
	log "unknwon.dev/clog/v2"
	"xorm.io/xorm"

	"github.com/gogs/git-module"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/db/errors"
	"gogs.io/gogs/internal/process"
	"gogs.io/gogs/internal/sync"
)

var MirrorQueue = sync.NewUniqueQueue(1000)

// Mirror represents mirror information of a repository.
type Mirror struct {
	ID          int64
	RepoID      int64
	Repo        *Repository `xorm:"-" json:"-"`
	Interval    int         // Hour.
	EnablePrune bool        `xorm:"NOT NULL DEFAULT true"`

	// Last and next sync time of Git data from upstream
	LastSync     time.Time `xorm:"-" json:"-"`
	LastSyncUnix int64     `xorm:"updated_unix"`
	NextSync     time.Time `xorm:"-" json:"-"`
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
			log.Error("GetRepositoryByID [%d]: %v", m.ID, err)
		}
	case "updated_unix":
		m.LastSync = time.Unix(m.LastSyncUnix, 0).Local()
	case "next_update_unix":
		m.NextSync = time.Unix(m.NextSyncUnix, 0).Local()
	}
}

// ScheduleNextSync calculates and sets next sync time based on repostiroy mirror setting.
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

	err := git.RepoRemoveRemote(repoPath, "origin")
	if err != nil {
		return fmt.Errorf("remove remote 'origin': %v", err)
	}

	addrURL, err := url.Parse(addr)
	if err != nil {
		return err
	}

	err = git.RepoAddRemote(repoPath, "origin", addrURL.String(), git.AddRemoteOptions{MirrorFetch: true})
	if err != nil {
		return fmt.Errorf("add remote 'origin': %v", err)
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
		if err := CreateRepositoryNotice(desc); err != nil {
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
		desc := fmt.Sprintf("Failed to update mirror repository '%s': %s", repoPath, stderr)
		log.Error(desc)
		if err = CreateRepositoryNotice(desc); err != nil {
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
			desc := fmt.Sprintf("Failed to update mirror wiki repository '%s': %s", wikiPath, stderr)
			log.Error(desc)
			if err = CreateRepositoryNotice(desc); err != nil {
				log.Error("CreateRepositoryNotice: %v", err)
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
		return nil, errors.MirrorNotExist{RepoID: repoID}
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
	if taskStatusTable.IsRunning(_MIRROR_UPDATE) {
		return
	}
	taskStatusTable.Start(_MIRROR_UPDATE)
	defer taskStatusTable.Stop(_MIRROR_UPDATE)

	log.Trace("Doing: MirrorUpdate")

	if err := x.Where("next_update_unix<=?", time.Now().Unix()).Iterate(new(Mirror), func(idx int, bean interface{}) error {
		m := bean.(*Mirror)
		if m.Repo == nil {
			log.Error("Disconnected mirror repository found: %d", m.ID)
			return nil
		}

		MirrorQueue.Add(m.RepoID)
		return nil
	}); err != nil {
		log.Error("MirrorUpdate: %v", err)
	}
}

// SyncMirrors checks and syncs mirrors.
// TODO: sync more mirrors at same time.
func SyncMirrors() {
	// Start listening on new sync requests.
	for repoID := range MirrorQueue.Queue() {
		log.Trace("SyncMirrors [repo_id: %s]", repoID)
		MirrorQueue.Remove(repoID)

		m, err := GetMirrorByRepoID(com.StrTo(repoID).MustInt64())
		if err != nil {
			log.Error("GetMirrorByRepoID [%d]: %v", m.RepoID, err)
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
				if err = MirrorSyncDeleteAction(m.Repo, result.refName); err != nil {
					log.Error("MirrorSyncDeleteAction [repo_id: %d]: %v", m.RepoID, err)
				}
				continue
			}

			// New reference
			isNewRef := false
			if result.oldCommitID == gitShortEmptyID {
				if err = MirrorSyncCreateAction(m.Repo, result.refName); err != nil {
					log.Error("MirrorSyncCreateAction [repo_id: %d]: %v", m.RepoID, err)
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

			if err = MirrorSyncPushAction(m.Repo, MirrorSyncPushActionOptions{
				RefName:     result.refName,
				OldCommitID: oldCommitID,
				NewCommitID: newCommitID,
				Commits:     CommitsToPushCommits(commits),
			}); err != nil {
				log.Error("MirrorSyncPushAction [repo_id: %d]: %v", m.RepoID, err)
				continue
			}
		}

		if _, err = x.Exec("UPDATE mirror SET updated_unix = ? WHERE repo_id = ?", time.Now().Unix(), m.RepoID); err != nil {
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

		if _, err = x.Exec("UPDATE repository SET updated_unix = ? WHERE id = ?", latestCommitTime.Unix(), m.RepoID); err != nil {
			log.Error("Update 'repository.updated_unix' [%d]: %v", m.RepoID, err)
			continue
		}
	}
}

func InitSyncMirrors() {
	go SyncMirrors()
}
