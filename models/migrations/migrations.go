// Copyright 2015 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package migrations

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/Unknwon/com"
	"github.com/go-xorm/xorm"
	gouuid "github.com/satori/go.uuid"
	"gopkg.in/ini.v1"

	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/log"
	"github.com/gogits/gogs/modules/setting"
)

const _MIN_DB_VER = 4

type Migration interface {
	Description() string
	Migrate(*xorm.Engine) error
}

type migration struct {
	description string
	migrate     func(*xorm.Engine) error
}

func NewMigration(desc string, fn func(*xorm.Engine) error) Migration {
	return &migration{desc, fn}
}

func (m *migration) Description() string {
	return m.description
}

func (m *migration) Migrate(x *xorm.Engine) error {
	return m.migrate(x)
}

// The version table. Should have only one row with id==1
type Version struct {
	Id      int64
	Version int64
}

// This is a sequence of migrations. Add new migrations to the bottom of the list.
// If you want to "retire" a migration, remove it from the top of the list and
// update _MIN_VER_DB accordingly
var migrations = []Migration{
	NewMigration("fix locale file load panic", fixLocaleFileLoadPanic),                 // V4 -> V5:v0.6.0
	NewMigration("trim action compare URL prefix", trimCommitActionAppUrlPrefix),       // V5 -> V6:v0.6.3
	NewMigration("generate issue-label from issue", issueToIssueLabel),                 // V6 -> V7:v0.6.4
	NewMigration("refactor attachment table", attachmentRefactor),                      // V7 -> V8:v0.6.4
	NewMigration("rename pull request fields", renamePullRequestFields),                // V8 -> V9:v0.6.16
	NewMigration("clean up migrate repo info", cleanUpMigrateRepoInfo),                 // V9 -> V10:v0.6.20
	NewMigration("generate rands and salt for organizations", generateOrgRandsAndSalt), // V10 -> V11:v0.8.5
}

// Migrate database to current version
func Migrate(x *xorm.Engine) error {
	if err := x.Sync(new(Version)); err != nil {
		return fmt.Errorf("sync: %v", err)
	}

	currentVersion := &Version{Id: 1}
	has, err := x.Get(currentVersion)
	if err != nil {
		return fmt.Errorf("get: %v", err)
	} else if !has {
		// If the version record does not exist we think
		// it is a fresh installation and we can skip all migrations.
		currentVersion.Version = int64(_MIN_DB_VER + len(migrations))

		if _, err = x.InsertOne(currentVersion); err != nil {
			return fmt.Errorf("insert: %v", err)
		}
	}

	v := currentVersion.Version
	if _MIN_DB_VER > v {
		log.Fatal(4, `Gogs no longer supports auto-migration from your previously installed version. 
Please try to upgrade to a lower version (>= v0.6.0) first, then upgrade to current version.`)
		return nil
	}

	if int(v-_MIN_DB_VER) > len(migrations) {
		// User downgraded Gogs.
		currentVersion.Version = int64(len(migrations) + _MIN_DB_VER)
		_, err = x.Id(1).Update(currentVersion)
		return err
	}
	for i, m := range migrations[v-_MIN_DB_VER:] {
		log.Info("Migration: %s", m.Description())
		if err = m.Migrate(x); err != nil {
			return fmt.Errorf("do migrate: %v", err)
		}
		currentVersion.Version = v + int64(i) + 1
		if _, err = x.Id(1).Update(currentVersion); err != nil {
			return err
		}
	}
	return nil
}

func sessionRelease(sess *xorm.Session) {
	if !sess.IsCommitedOrRollbacked {
		sess.Rollback()
	}
	sess.Close()
}

func fixLocaleFileLoadPanic(_ *xorm.Engine) error {
	cfg, err := ini.Load(setting.CustomConf)
	if err != nil {
		return fmt.Errorf("load custom config: %v", err)
	}

	cfg.DeleteSection("i18n")
	if err = cfg.SaveTo(setting.CustomConf); err != nil {
		return fmt.Errorf("save custom config: %v", err)
	}

	setting.Langs = strings.Split(strings.Replace(strings.Join(setting.Langs, ","), "fr-CA", "fr-FR", 1), ",")
	return nil
}

func trimCommitActionAppUrlPrefix(x *xorm.Engine) error {
	type PushCommit struct {
		Sha1        string
		Message     string
		AuthorEmail string
		AuthorName  string
	}

	type PushCommits struct {
		Len        int
		Commits    []*PushCommit
		CompareUrl string
	}

	type Action struct {
		ID      int64  `xorm:"pk autoincr"`
		Content string `xorm:"TEXT"`
	}

	results, err := x.Query("SELECT `id`,`content` FROM `action` WHERE `op_type`=?", 5)
	if err != nil {
		return fmt.Errorf("select commit actions: %v", err)
	}

	sess := x.NewSession()
	defer sessionRelease(sess)
	if err = sess.Begin(); err != nil {
		return err
	}

	var pushCommits *PushCommits
	for _, action := range results {
		actID := com.StrTo(string(action["id"])).MustInt64()
		if actID == 0 {
			continue
		}

		pushCommits = new(PushCommits)
		if err = json.Unmarshal(action["content"], pushCommits); err != nil {
			return fmt.Errorf("unmarshal action content[%d]: %v", actID, err)
		}

		infos := strings.Split(pushCommits.CompareUrl, "/")
		if len(infos) <= 4 {
			continue
		}
		pushCommits.CompareUrl = strings.Join(infos[len(infos)-4:], "/")

		p, err := json.Marshal(pushCommits)
		if err != nil {
			return fmt.Errorf("marshal action content[%d]: %v", actID, err)
		}

		if _, err = sess.Id(actID).Update(&Action{
			Content: string(p),
		}); err != nil {
			return fmt.Errorf("update action[%d]: %v", actID, err)
		}
	}
	return sess.Commit()
}

func issueToIssueLabel(x *xorm.Engine) error {
	type IssueLabel struct {
		ID      int64 `xorm:"pk autoincr"`
		IssueID int64 `xorm:"UNIQUE(s)"`
		LabelID int64 `xorm:"UNIQUE(s)"`
	}

	issueLabels := make([]*IssueLabel, 0, 50)
	results, err := x.Query("SELECT `id`,`label_ids` FROM `issue`")
	if err != nil {
		if strings.Contains(err.Error(), "no such column") ||
			strings.Contains(err.Error(), "Unknown column") {
			return nil
		}
		return fmt.Errorf("select issues: %v", err)
	}
	for _, issue := range results {
		issueID := com.StrTo(issue["id"]).MustInt64()

		// Just in case legacy code can have duplicated IDs for same label.
		mark := make(map[int64]bool)
		for _, idStr := range strings.Split(string(issue["label_ids"]), "|") {
			labelID := com.StrTo(strings.TrimPrefix(idStr, "$")).MustInt64()
			if labelID == 0 || mark[labelID] {
				continue
			}

			mark[labelID] = true
			issueLabels = append(issueLabels, &IssueLabel{
				IssueID: issueID,
				LabelID: labelID,
			})
		}
	}

	sess := x.NewSession()
	defer sessionRelease(sess)
	if err = sess.Begin(); err != nil {
		return err
	}

	if err = sess.Sync2(new(IssueLabel)); err != nil {
		return fmt.Errorf("sync2: %v", err)
	} else if _, err = sess.Insert(issueLabels); err != nil {
		return fmt.Errorf("insert issue-labels: %v", err)
	}

	return sess.Commit()
}

func attachmentRefactor(x *xorm.Engine) error {
	type Attachment struct {
		ID   int64  `xorm:"pk autoincr"`
		UUID string `xorm:"uuid INDEX"`

		// For rename purpose.
		Path    string `xorm:"-"`
		NewPath string `xorm:"-"`
	}

	results, err := x.Query("SELECT * FROM `attachment`")
	if err != nil {
		return fmt.Errorf("select attachments: %v", err)
	}

	attachments := make([]*Attachment, 0, len(results))
	for _, attach := range results {
		if !com.IsExist(string(attach["path"])) {
			// If the attachment is already missing, there is no point to update it.
			continue
		}
		attachments = append(attachments, &Attachment{
			ID:   com.StrTo(attach["id"]).MustInt64(),
			UUID: gouuid.NewV4().String(),
			Path: string(attach["path"]),
		})
	}

	sess := x.NewSession()
	defer sessionRelease(sess)
	if err = sess.Begin(); err != nil {
		return err
	}

	if err = sess.Sync2(new(Attachment)); err != nil {
		return fmt.Errorf("Sync2: %v", err)
	}

	// Note: Roll back for rename can be a dead loop,
	// 	so produces a backup file.
	var buf bytes.Buffer
	buf.WriteString("# old path -> new path\n")

	// Update database first because this is where error happens the most often.
	for _, attach := range attachments {
		if _, err = sess.Id(attach.ID).Update(attach); err != nil {
			return err
		}

		attach.NewPath = path.Join(setting.AttachmentPath, attach.UUID[0:1], attach.UUID[1:2], attach.UUID)
		buf.WriteString(attach.Path)
		buf.WriteString("\t")
		buf.WriteString(attach.NewPath)
		buf.WriteString("\n")
	}

	// Then rename attachments.
	isSucceed := true
	defer func() {
		if isSucceed {
			return
		}

		dumpPath := path.Join(setting.LogRootPath, "attachment_path.dump")
		ioutil.WriteFile(dumpPath, buf.Bytes(), 0666)
		fmt.Println("Fail to rename some attachments, old and new paths are saved into:", dumpPath)
	}()
	for _, attach := range attachments {
		if err = os.MkdirAll(path.Dir(attach.NewPath), os.ModePerm); err != nil {
			isSucceed = false
			return err
		}

		if err = os.Rename(attach.Path, attach.NewPath); err != nil {
			isSucceed = false
			return err
		}
	}

	return sess.Commit()
}

func renamePullRequestFields(x *xorm.Engine) (err error) {
	type PullRequest struct {
		ID         int64 `xorm:"pk autoincr"`
		PullID     int64 `xorm:"INDEX"`
		PullIndex  int64
		HeadBarcnh string

		IssueID    int64 `xorm:"INDEX"`
		Index      int64
		HeadBranch string
	}

	if err = x.Sync(new(PullRequest)); err != nil {
		return fmt.Errorf("sync: %v", err)
	}

	results, err := x.Query("SELECT `id`,`pull_id`,`pull_index`,`head_barcnh` FROM `pull_request`")
	if err != nil {
		if strings.Contains(err.Error(), "no such column") {
			return nil
		}
		return fmt.Errorf("select pull requests: %v", err)
	}

	sess := x.NewSession()
	defer sessionRelease(sess)
	if err = sess.Begin(); err != nil {
		return err
	}

	var pull *PullRequest
	for _, pr := range results {
		pull = &PullRequest{
			ID:         com.StrTo(pr["id"]).MustInt64(),
			IssueID:    com.StrTo(pr["pull_id"]).MustInt64(),
			Index:      com.StrTo(pr["pull_index"]).MustInt64(),
			HeadBranch: string(pr["head_barcnh"]),
		}
		if pull.Index == 0 {
			continue
		}
		if _, err = sess.Id(pull.ID).Update(pull); err != nil {
			return err
		}
	}

	return sess.Commit()
}

func cleanUpMigrateRepoInfo(x *xorm.Engine) (err error) {
	type (
		User struct {
			ID        int64 `xorm:"pk autoincr"`
			LowerName string
		}
		Repository struct {
			ID        int64 `xorm:"pk autoincr"`
			OwnerID   int64
			LowerName string
		}
	)

	repos := make([]*Repository, 0, 25)
	if err = x.Where("is_mirror=?", false).Find(&repos); err != nil {
		return fmt.Errorf("select all non-mirror repositories: %v", err)
	}
	var user *User
	for _, repo := range repos {
		user = &User{ID: repo.OwnerID}
		has, err := x.Get(user)
		if err != nil {
			return fmt.Errorf("get owner of repository[%d - %d]: %v", repo.ID, repo.OwnerID, err)
		} else if !has {
			continue
		}

		configPath := filepath.Join(setting.RepoRootPath, user.LowerName, repo.LowerName+".git/config")

		// In case repository file is somehow missing.
		if !com.IsFile(configPath) {
			continue
		}

		cfg, err := ini.Load(configPath)
		if err != nil {
			return fmt.Errorf("open config file: %v", err)
		}
		cfg.DeleteSection("remote \"origin\"")
		if err = cfg.SaveToIndent(configPath, "\t"); err != nil {
			return fmt.Errorf("save config file: %v", err)
		}
	}

	return nil
}

func generateOrgRandsAndSalt(x *xorm.Engine) (err error) {
	type User struct {
		ID    int64  `xorm:"pk autoincr"`
		Rands string `xorm:"VARCHAR(10)"`
		Salt  string `xorm:"VARCHAR(10)"`
	}

	orgs := make([]*User, 0, 10)
	if err = x.Where("type=1").And("rands=''").Find(&orgs); err != nil {
		return fmt.Errorf("select all organizations: %v", err)
	}

	sess := x.NewSession()
	defer sessionRelease(sess)
	if err = sess.Begin(); err != nil {
		return err
	}

	for _, org := range orgs {
		org.Rands = base.GetRandomString(10)
		org.Salt = base.GetRandomString(10)
		if _, err = sess.Id(org.ID).Update(org); err != nil {
			return err
		}
	}

	return sess.Commit()
}
