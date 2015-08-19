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
	"strings"
	"time"

	"github.com/Unknwon/com"
	"github.com/go-xorm/xorm"
	"gopkg.in/ini.v1"

	"github.com/gogits/gogs/modules/log"
	"github.com/gogits/gogs/modules/setting"
	gouuid "github.com/gogits/gogs/modules/uuid"
)

const _MIN_DB_VER = 0

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
	NewMigration("generate collaboration from access", accessToCollaboration),    // V0 -> V1:v0.5.13
	NewMigration("make authorize 4 if team is owners", ownerTeamUpdate),          // V1 -> V2:v0.5.13
	NewMigration("refactor access table to use id's", accessRefactor),            // V2 -> V3:v0.5.13
	NewMigration("generate team-repo from team", teamToTeamRepo),                 // V3 -> V4:v0.5.13
	NewMigration("fix locale file load panic", fixLocaleFileLoadPanic),           // V4 -> V5:v0.6.0
	NewMigration("trim action compare URL prefix", trimCommitActionAppUrlPrefix), // V5 -> V6:v0.6.3
	NewMigration("generate issue-label from issue", issueToIssueLabel),           // V6 -> V7:v0.6.4
	NewMigration("refactor attachment table", attachmentRefactor),                // V7 -> V8:v0.6.4
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
		// If the user table does not exist it is a fresh installation and we
		// can skip all migrations.
		needsMigration, err := x.IsTableExist("user")
		if err != nil {
			return err
		}
		if needsMigration {
			isEmpty, err := x.IsTableEmpty("user")
			if err != nil {
				return err
			}
			// If the user table is empty it is a fresh installation and we can
			// skip all migrations.
			needsMigration = !isEmpty
		}
		if !needsMigration {
			currentVersion.Version = int64(_MIN_DB_VER + len(migrations))
		}

		if _, err = x.InsertOne(currentVersion); err != nil {
			return fmt.Errorf("insert: %v", err)
		}
	}

	v := currentVersion.Version
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

func accessToCollaboration(x *xorm.Engine) (err error) {
	type Collaboration struct {
		ID      int64 `xorm:"pk autoincr"`
		RepoID  int64 `xorm:"UNIQUE(s) INDEX NOT NULL"`
		UserID  int64 `xorm:"UNIQUE(s) INDEX NOT NULL"`
		Created time.Time
	}

	if err = x.Sync(new(Collaboration)); err != nil {
		return fmt.Errorf("sync: %v", err)
	}

	results, err := x.Query("SELECT u.id AS `uid`, a.repo_name AS `repo`, a.mode AS `mode`, a.created as `created` FROM `access` a JOIN `user` u ON a.user_name=u.lower_name")
	if err != nil {
		if strings.Contains(err.Error(), "no such column") {
			return nil
		}
		return err
	}

	sess := x.NewSession()
	defer sessionRelease(sess)
	if err = sess.Begin(); err != nil {
		return err
	}

	offset := strings.Split(time.Now().String(), " ")[2]
	for _, result := range results {
		mode := com.StrTo(result["mode"]).MustInt64()
		// Collaborators must have write access.
		if mode < 2 {
			continue
		}

		userID := com.StrTo(result["uid"]).MustInt64()
		repoRefName := string(result["repo"])

		var created time.Time
		switch {
		case setting.UseSQLite3:
			created, _ = time.Parse(time.RFC3339, string(result["created"]))
		case setting.UseMySQL:
			created, _ = time.Parse("2006-01-02 15:04:05-0700", string(result["created"])+offset)
		case setting.UsePostgreSQL:
			created, _ = time.Parse("2006-01-02T15:04:05Z-0700", string(result["created"])+offset)
		}

		// find owner of repository
		parts := strings.SplitN(repoRefName, "/", 2)
		ownerName := parts[0]
		repoName := parts[1]

		results, err := sess.Query("SELECT u.id as `uid`, ou.uid as `memberid` FROM `user` u LEFT JOIN org_user ou ON ou.org_id=u.id WHERE u.lower_name=?", ownerName)
		if err != nil {
			return err
		}
		if len(results) < 1 {
			continue
		}

		ownerID := com.StrTo(results[0]["uid"]).MustInt64()
		if ownerID == userID {
			continue
		}

		// test if user is member of owning organization
		isMember := false
		for _, member := range results {
			memberID := com.StrTo(member["memberid"]).MustInt64()
			// We can skip all cases that a user is member of the owning organization
			if memberID == userID {
				isMember = true
			}
		}
		if isMember {
			continue
		}

		results, err = sess.Query("SELECT id FROM `repository` WHERE owner_id=? AND lower_name=?", ownerID, repoName)
		if err != nil {
			return err
		} else if len(results) < 1 {
			continue
		}

		collaboration := &Collaboration{
			UserID: userID,
			RepoID: com.StrTo(results[0]["id"]).MustInt64(),
		}
		has, err := sess.Get(collaboration)
		if err != nil {
			return err
		} else if has {
			continue
		}

		collaboration.Created = created
		if _, err = sess.InsertOne(collaboration); err != nil {
			return err
		}
	}

	return sess.Commit()
}

func ownerTeamUpdate(x *xorm.Engine) (err error) {
	if _, err := x.Exec("UPDATE `team` SET authorize=4 WHERE lower_name=?", "owners"); err != nil {
		return fmt.Errorf("update owner team table: %v", err)
	}
	return nil
}

func accessRefactor(x *xorm.Engine) (err error) {
	type (
		AccessMode int
		Access     struct {
			ID     int64 `xorm:"pk autoincr"`
			UserID int64 `xorm:"UNIQUE(s)"`
			RepoID int64 `xorm:"UNIQUE(s)"`
			Mode   AccessMode
		}
		UserRepo struct {
			UserID int64
			RepoID int64
		}
	)

	// We consiously don't start a session yet as we make only reads for now, no writes

	accessMap := make(map[UserRepo]AccessMode, 50)

	results, err := x.Query("SELECT r.id AS `repo_id`, r.is_private AS `is_private`, r.owner_id AS `owner_id`, u.type AS `owner_type` FROM `repository` r LEFT JOIN `user` u ON r.owner_id=u.id")
	if err != nil {
		return fmt.Errorf("select repositories: %v", err)
	}
	for _, repo := range results {
		repoID := com.StrTo(repo["repo_id"]).MustInt64()
		isPrivate := com.StrTo(repo["is_private"]).MustInt() > 0
		ownerID := com.StrTo(repo["owner_id"]).MustInt64()
		ownerIsOrganization := com.StrTo(repo["owner_type"]).MustInt() > 0

		results, err := x.Query("SELECT `user_id` FROM `collaboration` WHERE repo_id=?", repoID)
		if err != nil {
			return fmt.Errorf("select collaborators: %v", err)
		}
		for _, user := range results {
			userID := com.StrTo(user["user_id"]).MustInt64()
			accessMap[UserRepo{userID, repoID}] = 2 // WRITE ACCESS
		}

		if !ownerIsOrganization {
			continue
		}

		// The minimum level to add a new access record,
		// because public repository has implicit open access.
		minAccessLevel := AccessMode(0)
		if !isPrivate {
			minAccessLevel = 1
		}

		repoString := "$" + string(repo["repo_id"]) + "|"

		results, err = x.Query("SELECT `id`,`authorize`,`repo_ids` FROM `team` WHERE org_id=? AND authorize>? ORDER BY `authorize` ASC", ownerID, int(minAccessLevel))
		if err != nil {
			if strings.Contains(err.Error(), "no such column") {
				return nil
			}
			return fmt.Errorf("select teams from org: %v", err)
		}

		for _, team := range results {
			if !strings.Contains(string(team["repo_ids"]), repoString) {
				continue
			}
			teamID := com.StrTo(team["id"]).MustInt64()
			mode := AccessMode(com.StrTo(team["authorize"]).MustInt())

			results, err := x.Query("SELECT `uid` FROM `team_user` WHERE team_id=?", teamID)
			if err != nil {
				return fmt.Errorf("select users from team: %v", err)
			}
			for _, user := range results {
				userID := com.StrTo(user["uid"]).MustInt64()
				accessMap[UserRepo{userID, repoID}] = mode
			}
		}
	}

	// Drop table can't be in a session (at least not in sqlite)
	if _, err = x.Exec("DROP TABLE `access`"); err != nil {
		return fmt.Errorf("drop access table: %v", err)
	}

	// Now we start writing so we make a session
	sess := x.NewSession()
	defer sessionRelease(sess)
	if err = sess.Begin(); err != nil {
		return err
	}

	if err = sess.Sync2(new(Access)); err != nil {
		return fmt.Errorf("sync: %v", err)
	}

	accesses := make([]*Access, 0, len(accessMap))
	for ur, mode := range accessMap {
		accesses = append(accesses, &Access{UserID: ur.UserID, RepoID: ur.RepoID, Mode: mode})
	}

	if _, err = sess.Insert(accesses); err != nil {
		return fmt.Errorf("insert accesses: %v", err)
	}

	return sess.Commit()
}

func teamToTeamRepo(x *xorm.Engine) error {
	type TeamRepo struct {
		ID     int64 `xorm:"pk autoincr"`
		OrgID  int64 `xorm:"INDEX"`
		TeamID int64 `xorm:"UNIQUE(s)"`
		RepoID int64 `xorm:"UNIQUE(s)"`
	}

	teamRepos := make([]*TeamRepo, 0, 50)

	results, err := x.Query("SELECT `id`,`org_id`,`repo_ids` FROM `team`")
	if err != nil {
		if strings.Contains(err.Error(), "no such column") {
			return nil
		}
		return fmt.Errorf("select teams: %v", err)
	}
	for _, team := range results {
		orgID := com.StrTo(team["org_id"]).MustInt64()
		teamID := com.StrTo(team["id"]).MustInt64()

		// #1032: legacy code can have duplicated IDs for same repository.
		mark := make(map[int64]bool)
		for _, idStr := range strings.Split(string(team["repo_ids"]), "|") {
			repoID := com.StrTo(strings.TrimPrefix(idStr, "$")).MustInt64()
			if repoID == 0 || mark[repoID] {
				continue
			}

			mark[repoID] = true
			teamRepos = append(teamRepos, &TeamRepo{
				OrgID:  orgID,
				TeamID: teamID,
				RepoID: repoID,
			})
		}
	}

	sess := x.NewSession()
	defer sessionRelease(sess)
	if err = sess.Begin(); err != nil {
		return err
	}

	if err = sess.Sync2(new(TeamRepo)); err != nil {
		return fmt.Errorf("sync2: %v", err)
	} else if _, err = sess.Insert(teamRepos); err != nil {
		return fmt.Errorf("insert team-repos: %v", err)
	}

	return sess.Commit()
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
			return fmt.Errorf("unmarshal action content[%s]: %v", actID, err)
		}

		infos := strings.Split(pushCommits.CompareUrl, "/")
		if len(infos) <= 4 {
			continue
		}
		pushCommits.CompareUrl = strings.Join(infos[len(infos)-4:], "/")

		p, err := json.Marshal(pushCommits)
		if err != nil {
			return fmt.Errorf("marshal action content[%s]: %v", actID, err)
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
		if strings.Contains(err.Error(), "no such column") {
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
