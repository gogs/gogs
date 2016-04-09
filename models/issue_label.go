// Copyright 2016 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

import (
	"fmt"
	"html/template"
	"strconv"
	"strings"

	"github.com/go-xorm/xorm"
)

// Label represents a label of repository for issues.
type Label struct {
	ID              int64 `xorm:"pk autoincr"`
	RepoID          int64 `xorm:"INDEX"`
	Name            string
	Color           string `xorm:"VARCHAR(7)"`
	NumIssues       int
	NumClosedIssues int
	NumOpenIssues   int  `xorm:"-"`
	IsChecked       bool `xorm:"-"`
}

// CalOpenIssues calculates the open issues of label.
func (m *Label) CalOpenIssues() {
	m.NumOpenIssues = m.NumIssues - m.NumClosedIssues
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

			if luminance < 0.5 {
				return template.CSS("#fff")
			}
		}
	}

	// default to black
	return template.CSS("#000")
}

// NewLabel creates new label of repository.
func NewLabel(l *Label) error {
	_, err := x.Insert(l)
	return err
}

func getLabelByID(e Engine, id int64) (*Label, error) {
	if id <= 0 {
		return nil, ErrLabelNotExist{id}
	}

	l := &Label{ID: id}
	has, err := x.Get(l)
	if err != nil {
		return nil, err
	} else if !has {
		return nil, ErrLabelNotExist{l.ID}
	}
	return l, nil
}

// GetLabelByID returns a label by given ID.
func GetLabelByID(id int64) (*Label, error) {
	return getLabelByID(x, id)
}

// GetLabelsByRepoID returns all labels that belong to given repository by ID.
func GetLabelsByRepoID(repoID int64) ([]*Label, error) {
	labels := make([]*Label, 0, 10)
	return labels, x.Where("repo_id=?", repoID).Find(&labels)
}

func getLabelsByIssueID(e Engine, issueID int64) ([]*Label, error) {
	issueLabels, err := getIssueLabels(e, issueID)
	if err != nil {
		return nil, fmt.Errorf("getIssueLabels: %v", err)
	}

	var label *Label
	labels := make([]*Label, 0, len(issueLabels))
	for idx := range issueLabels {
		label, err = getLabelByID(e, issueLabels[idx].LabelID)
		if err != nil && !IsErrLabelNotExist(err) {
			return nil, fmt.Errorf("getLabelByID: %v", err)
		}
		labels = append(labels, label)
	}
	return labels, nil
}

// GetLabelsByIssueID returns all labels that belong to given issue by ID.
func GetLabelsByIssueID(issueID int64) ([]*Label, error) {
	return getLabelsByIssueID(x, issueID)
}

func updateLabel(e Engine, l *Label) error {
	_, err := e.Id(l.ID).AllCols().Update(l)
	return err
}

// UpdateLabel updates label information.
func UpdateLabel(l *Label) error {
	return updateLabel(x, l)
}

// DeleteLabel delete a label of given repository.
func DeleteLabel(repoID, labelID int64) error {
	l, err := GetLabelByID(labelID)
	if err != nil {
		if IsErrLabelNotExist(err) {
			return nil
		}
		return err
	}

	sess := x.NewSession()
	defer sessionRelease(sess)
	if err = sess.Begin(); err != nil {
		return err
	}

	if _, err = x.Where("label_id=?", labelID).Delete(new(IssueLabel)); err != nil {
		return err
	} else if _, err = sess.Delete(l); err != nil {
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

// IssueLabel represetns an issue-lable relation.
type IssueLabel struct {
	ID      int64 `xorm:"pk autoincr"`
	IssueID int64 `xorm:"UNIQUE(s)"`
	LabelID int64 `xorm:"UNIQUE(s)"`
}

func hasIssueLabel(e Engine, issueID, labelID int64) bool {
	has, _ := e.Where("issue_id=? AND label_id=?", issueID, labelID).Get(new(IssueLabel))
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
	return updateLabel(e, label)
}

// NewIssueLabel creates a new issue-label relation.
func NewIssueLabel(issue *Issue, label *Label) (err error) {
	sess := x.NewSession()
	defer sessionRelease(sess)
	if err = sess.Begin(); err != nil {
		return err
	}

	if err = newIssueLabel(sess, issue, label); err != nil {
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
	return updateLabel(e, label)
}

// DeleteIssueLabel deletes issue-label relation.
func DeleteIssueLabel(issue *Issue, label *Label) (err error) {
	sess := x.NewSession()
	defer sessionRelease(sess)
	if err = sess.Begin(); err != nil {
		return err
	}

	if err = deleteIssueLabel(sess, issue, label); err != nil {
		return err
	}

	return sess.Commit()
}
