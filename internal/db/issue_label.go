// Copyright 2016 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package db

import (
	"fmt"
	"html/template"
	"strconv"
	"strings"

	"xorm.io/xorm"

	api "github.com/gogs/go-gogs-client"

	"gogs.io/gogs/internal/errutil"
	"gogs.io/gogs/internal/lazyregexp"
	"gogs.io/gogs/internal/tool"
)

var labelColorPattern = lazyregexp.New("#([a-fA-F0-9]{6})")

// GetLabelTemplateFile loads the label template file by given name,
// then parses and returns a list of name-color pairs.
func GetLabelTemplateFile(name string) ([][2]string, error) {
	data, err := getRepoInitFile("label", name)
	if err != nil {
		return nil, fmt.Errorf("getRepoInitFile: %v", err)
	}

	lines := strings.Split(string(data), "\n")
	list := make([][2]string, 0, len(lines))
	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}

		fields := strings.SplitN(line, " ", 2)
		if len(fields) != 2 {
			return nil, fmt.Errorf("line is malformed: %s", line)
		}

		if !labelColorPattern.MatchString(fields[0]) {
			return nil, fmt.Errorf("bad HTML color code in line: %s", line)
		}

		fields[1] = strings.TrimSpace(fields[1])
		list = append(list, [2]string{fields[1], fields[0]})
	}

	return list, nil
}

// Label represents a label of repository for issues.
type Label struct {
	ID              int64
	RepoID          int64 `xorm:"INDEX"`
	Name            string
	Color           string `xorm:"VARCHAR(7)"`
	NumIssues       int
	NumClosedIssues int
	NumOpenIssues   int  `xorm:"-" json:"-"`
	IsChecked       bool `xorm:"-" json:"-"`
}

func (label *Label) APIFormat() *api.Label {
	return &api.Label{
		ID:    label.ID,
		Name:  label.Name,
		Color: strings.TrimLeft(label.Color, "#"),
	}
}

// CalOpenIssues calculates the open issues of label.
func (label *Label) CalOpenIssues() {
	label.NumOpenIssues = label.NumIssues - label.NumClosedIssues
}

// ForegroundColor calculates the text color for labels based
// on their background color.
func (l *Label) ForegroundColor() template.CSS {
	if strings.HasPrefix(l.Color, "#") {
		if color, err := strconv.ParseUint(l.Color[1:], 16, 64); err == nil {
			r := float32(0xFF & (color >> 16))
			g := float32(0xFF & (color >> 8))
			b := float32(0xFF & color)
			luminance := (0.2126*r + 0.7152*g + 0.0722*b) / 255

			if luminance < 0.66 {
				return template.CSS("#fff")
			}
		}
	}

	// default to black
	return template.CSS("#000")
}

// NewLabels creates new label(s) for a repository.
func NewLabels(labels ...*Label) error {
	_, err := x.Insert(labels)
	return err
}

var _ errutil.NotFound = (*ErrLabelNotExist)(nil)

type ErrLabelNotExist struct {
	args map[string]any
}

func IsErrLabelNotExist(err error) bool {
	_, ok := err.(ErrLabelNotExist)
	return ok
}

func (err ErrLabelNotExist) Error() string {
	return fmt.Sprintf("label does not exist: %v", err.args)
}

func (ErrLabelNotExist) NotFound() bool {
	return true
}

// getLabelOfRepoByName returns a label by Name in given repository.
// If pass repoID as 0, then ORM will ignore limitation of repository
// and can return arbitrary label with any valid ID.
func getLabelOfRepoByName(e Engine, repoID int64, labelName string) (*Label, error) {
	if len(labelName) <= 0 {
		return nil, ErrLabelNotExist{args: map[string]any{"repoID": repoID}}
	}

	l := &Label{
		Name:   labelName,
		RepoID: repoID,
	}
	has, err := e.Get(l)
	if err != nil {
		return nil, err
	} else if !has {
		return nil, ErrLabelNotExist{args: map[string]any{"repoID": repoID}}
	}
	return l, nil
}

// getLabelInRepoByID returns a label by ID in given repository.
// If pass repoID as 0, then ORM will ignore limitation of repository
// and can return arbitrary label with any valid ID.
func getLabelOfRepoByID(e Engine, repoID, labelID int64) (*Label, error) {
	if labelID <= 0 {
		return nil, ErrLabelNotExist{args: map[string]any{"repoID": repoID, "labelID": labelID}}
	}

	l := &Label{
		ID:     labelID,
		RepoID: repoID,
	}
	has, err := e.Get(l)
	if err != nil {
		return nil, err
	} else if !has {
		return nil, ErrLabelNotExist{args: map[string]any{"repoID": repoID, "labelID": labelID}}
	}
	return l, nil
}

// GetLabelByID returns a label by given ID.
func GetLabelByID(id int64) (*Label, error) {
	return getLabelOfRepoByID(x, 0, id)
}

// GetLabelOfRepoByID returns a label by ID in given repository.
func GetLabelOfRepoByID(repoID, labelID int64) (*Label, error) {
	return getLabelOfRepoByID(x, repoID, labelID)
}

// GetLabelOfRepoByName returns a label by name in given repository.
func GetLabelOfRepoByName(repoID int64, labelName string) (*Label, error) {
	return getLabelOfRepoByName(x, repoID, labelName)
}

// GetLabelsInRepoByIDs returns a list of labels by IDs in given repository,
// it silently ignores label IDs that are not belong to the repository.
func GetLabelsInRepoByIDs(repoID int64, labelIDs []int64) ([]*Label, error) {
	labels := make([]*Label, 0, len(labelIDs))
	return labels, x.Where("repo_id = ?", repoID).In("id", tool.Int64sToStrings(labelIDs)).Asc("name").Find(&labels)
}

// GetLabelsByRepoID returns all labels that belong to given repository by ID.
func GetLabelsByRepoID(repoID int64) ([]*Label, error) {
	labels := make([]*Label, 0, 10)
	return labels, x.Where("repo_id = ?", repoID).Asc("name").Find(&labels)
}

func getLabelsByIssueID(e Engine, issueID int64) ([]*Label, error) {
	issueLabels, err := getIssueLabels(e, issueID)
	if err != nil {
		return nil, fmt.Errorf("getIssueLabels: %v", err)
	} else if len(issueLabels) == 0 {
		return []*Label{}, nil
	}

	labelIDs := make([]int64, len(issueLabels))
	for i := range issueLabels {
		labelIDs[i] = issueLabels[i].LabelID
	}

	labels := make([]*Label, 0, len(labelIDs))
	return labels, e.Where("id > 0").In("id", tool.Int64sToStrings(labelIDs)).Asc("name").Find(&labels)
}

// GetLabelsByIssueID returns all labels that belong to given issue by ID.
func GetLabelsByIssueID(issueID int64) ([]*Label, error) {
	return getLabelsByIssueID(x, issueID)
}

func updateLabel(e Engine, l *Label) error {
	_, err := e.ID(l.ID).AllCols().Update(l)
	return err
}

// UpdateLabel updates label information.
func UpdateLabel(l *Label) error {
	return updateLabel(x, l)
}

// DeleteLabel delete a label of given repository.
func DeleteLabel(repoID, labelID int64) error {
	_, err := GetLabelOfRepoByID(repoID, labelID)
	if err != nil {
		if IsErrLabelNotExist(err) {
			return nil
		}
		return err
	}

	sess := x.NewSession()
	defer sess.Close()
	if err = sess.Begin(); err != nil {
		return err
	}

	if _, err = sess.ID(labelID).Delete(new(Label)); err != nil {
		return err
	} else if _, err = sess.Where("label_id = ?", labelID).Delete(new(IssueLabel)); err != nil {
		return err
	}

	return sess.Commit()
}

// .___                            .____          ___.          .__
// |   | ______ ________ __   ____ |    |   _____ \_ |__   ____ |  |
// |   |/  ___//  ___/  |  \_/ __ \|    |   \__  \ | __ \_/ __ \|  |
// |   |\___ \ \___ \|  |  /\  ___/|    |___ / __ \| \_\ \  ___/|  |__
// |___/____  >____  >____/  \___  >_______ (____  /___  /\___  >____/
//          \/     \/            \/        \/    \/    \/     \/

// IssueLabel represents an issue-lable relation.
type IssueLabel struct {
	ID      int64
	IssueID int64 `xorm:"UNIQUE(s)"`
	LabelID int64 `xorm:"UNIQUE(s)"`
}

func hasIssueLabel(e Engine, issueID, labelID int64) bool {
	has, _ := e.Where("issue_id = ? AND label_id = ?", issueID, labelID).Get(new(IssueLabel))
	return has
}

// HasIssueLabel returns true if issue has been labeled.
func HasIssueLabel(issueID, labelID int64) bool {
	return hasIssueLabel(x, issueID, labelID)
}

func newIssueLabel(e *xorm.Session, issue *Issue, label *Label) (err error) {
	if _, err = e.Insert(&IssueLabel{
		IssueID: issue.ID,
		LabelID: label.ID,
	}); err != nil {
		return err
	}

	label.NumIssues++
	if issue.IsClosed {
		label.NumClosedIssues++
	}

	if err = updateLabel(e, label); err != nil {
		return fmt.Errorf("updateLabel: %v", err)
	}

	issue.Labels = append(issue.Labels, label)
	return nil
}

// NewIssueLabel creates a new issue-label relation.
func NewIssueLabel(issue *Issue, label *Label) (err error) {
	if HasIssueLabel(issue.ID, label.ID) {
		return nil
	}

	sess := x.NewSession()
	defer sess.Close()
	if err = sess.Begin(); err != nil {
		return err
	}

	if err = newIssueLabel(sess, issue, label); err != nil {
		return err
	}

	return sess.Commit()
}

func newIssueLabels(e *xorm.Session, issue *Issue, labels []*Label) (err error) {
	for i := range labels {
		if hasIssueLabel(e, issue.ID, labels[i].ID) {
			continue
		}

		if err = newIssueLabel(e, issue, labels[i]); err != nil {
			return fmt.Errorf("newIssueLabel: %v", err)
		}
	}

	return nil
}

// NewIssueLabels creates a list of issue-label relations.
func NewIssueLabels(issue *Issue, labels []*Label) (err error) {
	sess := x.NewSession()
	defer sess.Close()
	if err = sess.Begin(); err != nil {
		return err
	}

	if err = newIssueLabels(sess, issue, labels); err != nil {
		return err
	}

	return sess.Commit()
}

func getIssueLabels(e Engine, issueID int64) ([]*IssueLabel, error) {
	issueLabels := make([]*IssueLabel, 0, 10)
	return issueLabels, e.Where("issue_id=?", issueID).Asc("label_id").Find(&issueLabels)
}

// GetIssueLabels returns all issue-label relations of given issue by ID.
func GetIssueLabels(issueID int64) ([]*IssueLabel, error) {
	return getIssueLabels(x, issueID)
}

func deleteIssueLabel(e *xorm.Session, issue *Issue, label *Label) (err error) {
	if _, err = e.Delete(&IssueLabel{
		IssueID: issue.ID,
		LabelID: label.ID,
	}); err != nil {
		return err
	}

	label.NumIssues--
	if issue.IsClosed {
		label.NumClosedIssues--
	}
	if err = updateLabel(e, label); err != nil {
		return fmt.Errorf("updateLabel: %v", err)
	}

	for i := range issue.Labels {
		if issue.Labels[i].ID == label.ID {
			issue.Labels = append(issue.Labels[:i], issue.Labels[i+1:]...)
			break
		}
	}
	return nil
}

// DeleteIssueLabel deletes issue-label relation.
func DeleteIssueLabel(issue *Issue, label *Label) (err error) {
	sess := x.NewSession()
	defer sess.Close()
	if err = sess.Begin(); err != nil {
		return err
	}

	if err = deleteIssueLabel(sess, issue, label); err != nil {
		return err
	}

	return sess.Commit()
}
