// Copyright 2016 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/Unknwon/com"
	"github.com/go-xorm/xorm"
	log "gopkg.in/clog.v1"
	"gopkg.in/ini.v1"

	"github.com/gogs/git-module"

	"github.com/gogs/gogs/models/errors"
	"github.com/gogs/gogs/pkg/process"
	"github.com/gogs/gogs/pkg/setting"
	"github.com/gogs/gogs/pkg/sync"
)

var MirrorQueue = sync.NewUniqueQueue(setting.Repository.MirrorQueueLength)

// Mirror represents mirror information of a repository.
type Mirror struct {
	ID          int64
	RepoID      int64
	Repo        *Repository `xorm:"-"`
	Interval    int         // Hour.
	EnablePrune bool        `xorm:"NOT NULL DEFAULT true"`

	// Last and next sync time of Git data from upstream
	LastSync     time.Time `xorm:"-"`
	LastSyncUnix int64     `xorm:"updated_unix"`
	NextSync     time.Time `xorm:"-"`
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
			log.Error(3, "GetRepositoryByID [%d]: %v", m.ID, err)
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

// findPasswordInMirrorAddress returns start (inclusive) and end index (exclusive)
// of password portion of credentials in given mirror address.
// It returns a boolean value to indicate whether password portion is found.
func findPasswordInMirrorAddress(addr string) (start int, end int, found bool) {
	// Find end of credentials (start of path)
	end = strings.LastIndex(addr, "@")
	if end == -1 {
		return -1, -1, false
	}

	// Find delimiter of credentials (end of username)
	start = strings.Index(addr, "://")
	if start == -1 {
		return -1, -1, false
	}
	start += 3
	delim := strings.Index(addr[start:], ":")
	if delim == -1 {
		return -1, -1, false
	}
	delim += 1

	if start+delim >= end {
		return -1, -1, false // No password portion presented
	}

	return start + delim, end, true
}

// unescapeMirrorCredentials returns mirror address with unescaped credentials.
func unescapeMirrorCredentials(addr string) string {
	start, end, found := findPasswordInMirrorAddress(addr)
	if !found {
		return addr
	}

	password, _ := url.QueryUnescape(addr[start:end])
	return addr[:start] + password + addr[end:]
}

func (m *Mirror) readAddress() {
	if len(m.address) > 0 {
		return
	}

	cfg, err := ini.Load(m.Repo.GitConfigPath())
	if err != nil {
		log.Error(2, "Load: %v", err)
		return
	}
	m.address = cfg.Section("remote \"origin\"").Key("url").Value()
}

// HandleMirrorCredentials replaces user credentials from HTTP/HTTPS URL
// with placeholder <credentials>.
// It returns original string if protocol is not HTTP/HTTPS.
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

// FullAddress returns mirror address from Git repository config with unescaped credentials.
func (m *Mirror) FullAddress() string {
	m.readAddress()
	return unescapeMirrorCredentials(m.address)
}

// escapeCredentials returns mirror address with escaped credentials.
func escapeMirrorCredentials(addr string) string {
	start, end, found := findPasswordInMirrorAddress(addr)
	if !found {
		return addr
	}

	return addr[:start] + url.QueryEscape(addr[start:end]) + addr[end:]
}

// SaveAddress writes new address to Git repository config.
func (m *Mirror) SaveAddress(addr string) error {
	configPath := m.Repo.GitConfigPath()
	cfg, err := ini.Load(configPath)
	if err != nil {
		return fmt.Errorf("Load: %v", err)
	}

	cfg.Section(`remote "origin"`).Key("url").SetValue(escapeMirrorCredentials(addr))
	return cfg.SaveToIndent(configPath, "\t")
}

const GIT_SHORT_EMPTY_SHA = "0000000"

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
				oldCommitID: GIT_SHORT_EMPTY_SHA,
			})
		case strings.HasPrefix(lines[i], " - "): // Delete reference
			results = append(results, &mirrorSyncResult{
				refName:     refName,
				newCommitID: GIT_SHORT_EMPTY_SHA,
			})
		case strings.HasPrefix(lines[i], "   "): // New commits of a reference
			delimIdx := strings.Index(lines[i][3:], " ")
			if delimIdx == -1 {
				log.Error(2, "SHA delimiter not found: %q", lines[i])
				continue
			}
			shas := strings.Split(lines[i][3:delimIdx+3], "..")
			if len(shas) != 2 {
				log.Error(2, "Expect two SHAs but not what found: %q", lines[i])
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
	timeout := time.Duration(setting.Git.Timeout.Mirror) * time.Second

	// Do a fast-fail testing against on repository URL to ensure it is accessible under
	// good condition to prevent long blocking on URL resolution without syncing anything.
	if !git.IsRepoURLAccessible(git.NetworkOptions{
		URL:     m.RawAddress(),
		Timeout: 10 * time.Second,
	}) {
		desc := fmt.Sprintf("Source URL of mirror repository '%s' is not accessible: %s", m.Repo.FullName(), m.MosaicsAddress())
		if err := CreateRepositoryNotice(desc); err != nil {
			log.Error(2, "CreateRepositoryNotice: %v", err)
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
		desc := fmt.Sprintf("Fail to update mirror repository '%s': %s", repoPath, stderr)
		log.Error(2, desc)
		if err = CreateRepositoryNotice(desc); err != nil {
			log.Error(2, "CreateRepositoryNotice: %v", err)
		}
		return nil, false
	}
	output := stderr

	if err := m.Repo.UpdateSize(); err != nil {
		log.Error(2, "UpdateSize [repo_id: %d]: %v", m.Repo.ID, err)
	}

	if m.Repo.HasWiki() {
		// Even if wiki sync failed, we still want results from the main repository
		if _, stderr, err := process.ExecDir(
			timeout, wikiPath, fmt.Sprintf("Mirror.runSync: %s", wikiPath),
			"git", "remote", "update", "--prune"); err != nil {
			desc := fmt.Sprintf("Fail to update mirror wiki repository '%s': %s", wikiPath, stderr)
			log.Error(2, desc)
			if err = CreateRepositoryNotice(desc); err != nil {
				log.Error(2, "CreateRepositoryNotice: %v", err)
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
		return nil, errors.MirrorNotExist{repoID}
	}
	return m, nil
}

// GetMirrorByRepoID returns mirror information of a repository.
func GetMirrorByRepoID(repoID int64) (*Mirror, error) {
	return getMirrorByRepoID(x, repoID)
}

func updateMirror(e Engine, m *Mirror) error {
	_, err := e.Id(m.ID).AllCols().Update(m)
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
			log.Error(2, "Disconnected mirror repository found: %d", m.ID)
			return nil
		}

		MirrorQueue.Add(m.RepoID)
		return nil
	}); err != nil {
		log.Error(2, "MirrorUpdate: %v", err)
	}
}

// SyncMirrors checks and syncs mirrors.
// TODO: sync more mirrors at same time.
func SyncMirrors() {
	// Start listening on new sync requests.
	for repoID := range MirrorQueue.Queue() {
		log.Trace("SyncMirrors [repo_id: %d]", repoID)
		MirrorQueue.Remove(repoID)

		m, err := GetMirrorByRepoID(com.StrTo(repoID).MustInt64())
		if err != nil {
			log.Error(2, "GetMirrorByRepoID [%d]: %v", m.RepoID, err)
			continue
		}

		results, ok := m.runSync()
		if !ok {
			continue
		}

		m.ScheduleNextSync()
		if err = UpdateMirror(m); err != nil {
			log.Error(2, "UpdateMirror [%d]: %v", m.RepoID, err)
			continue
		}

		// TODO:
		// - Create "Mirror Sync" webhook event
		// - Create mirror sync (create, push and delete) events and trigger the "mirror sync" webhooks

		var gitRepo *git.Repository
		if len(results) == 0 {
			log.Trace("SyncMirrors [repo_id: %d]: no commits fetched", m.RepoID)
		} else {
			gitRepo, err = git.OpenRepository(m.Repo.RepoPath())
			if err != nil {
				log.Error(2, "OpenRepository [%d]: %v", m.RepoID, err)
				continue
			}
		}

		for _, result := range results {
			// Discard GitHub pull requests, i.e. refs/pull/*
			if strings.HasPrefix(result.refName, "refs/pull/") {
				continue
			}

			// Create reference
			if result.oldCommitID == GIT_SHORT_EMPTY_SHA {
				if err = MirrorSyncCreateAction(m.Repo, result.refName); err != nil {
					log.Error(2, "MirrorSyncCreateAction [repo_id: %d]: %v", m.RepoID, err)
				}
				continue
			}

			// Delete reference
			if result.newCommitID == GIT_SHORT_EMPTY_SHA {
				if err = MirrorSyncDeleteAction(m.Repo, result.refName); err != nil {
					log.Error(2, "MirrorSyncDeleteAction [repo_id: %d]: %v", m.RepoID, err)
				}
				continue
			}

			// Push commits
			oldCommitID, err := git.GetFullCommitID(gitRepo.Path, result.oldCommitID)
			if err != nil {
				log.Error(2, "GetFullCommitID [%d]: %v", m.RepoID, err)
				continue
			}
			newCommitID, err := git.GetFullCommitID(gitRepo.Path, result.newCommitID)
			if err != nil {
				log.Error(2, "GetFullCommitID [%d]: %v", m.RepoID, err)
				continue
			}
			commits, err := gitRepo.CommitsBetweenIDs(newCommitID, oldCommitID)
			if err != nil {
				log.Error(2, "CommitsBetweenIDs [repo_id: %d, new_commit_id: %s, old_commit_id: %s]: %v", m.RepoID, newCommitID, oldCommitID, err)
				continue
			}
			if err = MirrorSyncPushAction(m.Repo, MirrorSyncPushActionOptions{
				RefName:     result.refName,
				OldCommitID: oldCommitID,
				NewCommitID: newCommitID,
				Commits:     ListToPushCommits(commits),
			}); err != nil {
				log.Error(2, "MirrorSyncPushAction [repo_id: %d]: %v", m.RepoID, err)
				continue
			}
		}

		if _, err = x.Exec("UPDATE mirror SET updated_unix = ? WHERE repo_id = ?", time.Now().Unix(), m.RepoID); err != nil {
			log.Error(2, "Update 'mirror.updated_unix' [%d]: %v", m.RepoID, err)
			continue
		}

		// Get latest commit date and compare to current repository updated time,
		// update if latest commit date is newer.
		commitDate, err := git.GetLatestCommitDate(m.Repo.RepoPath(), "")
		if err != nil {
			log.Error(2, "GetLatestCommitDate [%d]: %v", m.RepoID, err)
			continue
		} else if commitDate.Before(m.Repo.Updated) {
			continue
		}

		if _, err = x.Exec("UPDATE repository SET updated_unix = ? WHERE id = ?", commitDate.Unix(), m.RepoID); err != nil {
			log.Error(2, "Update 'repository.updated_unix' [%d]: %v", m.RepoID, err)
			continue
		}
	}
}

func InitSyncMirrors() {
	go SyncMirrors()
}
