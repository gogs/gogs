package database

import (
	"context"
	"fmt"
	"html/template"
	"slices"
	"strconv"
	"strings"

	"github.com/cockroachdb/errors"
	"gorm.io/gorm"

	"gogs.io/gogs/internal/api/v1types"
	"gogs.io/gogs/internal/errx"
	"gogs.io/gogs/internal/lazyregexp"
)

var labelColorPattern = lazyregexp.New("#([a-fA-F0-9]{6})")

// GetLabelTemplateFile loads the label template file by given name,
// then parses and returns a list of name-color pairs.
func GetLabelTemplateFile(name string) ([][2]string, error) {
	data, err := getRepoInitFile("label", name)
	if err != nil {
		return nil, errors.Newf("getRepoInitFile: %v", err)
	}

	lines := strings.Split(string(data), "\n")
	list := make([][2]string, 0, len(lines))
	for i := range lines {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}

		fields := strings.SplitN(line, " ", 2)
		if len(fields) != 2 {
			return nil, errors.Newf("line is malformed: %s", line)
		}

		if !labelColorPattern.MatchString(fields[0]) {
			return nil, errors.Newf("bad HTML color code in line: %s", line)
		}

		fields[1] = strings.TrimSpace(fields[1])
		list = append(list, [2]string{fields[1], fields[0]})
	}

	return list, nil
}

// Label represents a label of repository for issues.
type Label struct {
	ID              int64
	RepoID          int64 `gorm:"index:IDX_label_repo_id"`
	Name            string
	Color           string `gorm:"type:VARCHAR(7)"`
	NumIssues       int
	NumClosedIssues int
	NumOpenIssues   int  `json:"-" gorm:"-"`
	IsChecked       bool `json:"-" gorm:"-"`
}

func (l *Label) APIFormat() *v1types.IssueLabel {
	return &v1types.IssueLabel{
		ID:    l.ID,
		Name:  l.Name,
		Color: strings.TrimLeft(l.Color, "#"),
	}
}

// CalOpenIssues calculates the open issues of label.
func (l *Label) CalOpenIssues() {
	l.NumOpenIssues = l.NumIssues - l.NumClosedIssues
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

	// Default to black.
	return template.CSS("#000")
}

var _ errx.NotFound = (*ErrLabelNotExist)(nil)

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

// IssueLabel represents an issue-label relation.
type IssueLabel struct {
	ID      int64
	IssueID int64 `gorm:"uniqueIndex:UQE_issue_label_s"`
	LabelID int64 `gorm:"uniqueIndex:UQE_issue_label_s"`
}

// LabelsStore is the storage layer for labels of repositories and their
// relations to issues.
type LabelsStore struct {
	db *gorm.DB
}

func newLabelsStore(db *gorm.DB) *LabelsStore {
	return &LabelsStore{db: db}
}

// Create creates given labels.
func (s *LabelsStore) Create(ctx context.Context, labels ...*Label) error {
	if len(labels) == 0 {
		return nil
	}
	return s.db.WithContext(ctx).Create(&labels).Error
}

func (s *LabelsStore) get(ctx context.Context, args map[string]any, query string, values ...any) (*Label, error) {
	l := new(Label)
	err := s.db.WithContext(ctx).Where(query, values...).First(l).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrLabelNotExist{args: args}
		}
		return nil, err
	}
	return l, nil
}

// GetByID returns the label with given ID of the repository with given ID. It
// returns ErrLabelNotExist when not found.
func (s *LabelsStore) GetByID(ctx context.Context, repoID, id int64) (*Label, error) {
	return s.get(ctx, map[string]any{"repoID": repoID, "labelID": id}, "repo_id = ? AND id = ?", repoID, id)
}

// GetByName returns the label with given name of the repository with given
// ID. It returns ErrLabelNotExist when not found.
func (s *LabelsStore) GetByName(ctx context.Context, repoID int64, name string) (*Label, error) {
	return s.get(ctx, map[string]any{"repoID": repoID, "name": name}, "repo_id = ? AND name = ?", repoID, name)
}

// ListByRepoID returns the labels of the repository with given ID, sorted by
// name.
func (s *LabelsStore) ListByRepoID(ctx context.Context, repoID int64) ([]*Label, error) {
	labels := make([]*Label, 0, 10)
	return labels, s.db.WithContext(ctx).Where("repo_id = ?", repoID).Order("name ASC").Order("id ASC").Find(&labels).Error
}

// ListByIDs returns the labels with given IDs of the repository with given ID,
// sorted by name. IDs of labels of other repositories are skipped.
func (s *LabelsStore) ListByIDs(ctx context.Context, repoID int64, ids []int64) ([]*Label, error) {
	labels := make([]*Label, 0, len(ids))
	if len(ids) == 0 {
		return labels, nil
	}
	return labels, s.db.WithContext(ctx).Where("repo_id = ? AND id IN ?", repoID, ids).Order("name ASC").Order("id ASC").Find(&labels).Error
}

// ListByIssueID returns the labels of the issue with given ID, sorted by name.
func (s *LabelsStore) ListByIssueID(ctx context.Context, issueID int64) ([]*Label, error) {
	labels := make([]*Label, 0, 5)
	return labels, s.db.WithContext(ctx).
		Joins("JOIN issue_label ON issue_label.label_id = label.id").
		Where("issue_label.issue_id = ?", issueID).
		Order("label.name ASC").Order("label.id ASC").
		Find(&labels).Error
}

// Update saves the name and color of the label.
func (s *LabelsStore) Update(ctx context.Context, l *Label) error {
	return s.db.WithContext(ctx).Model(l).Select("name", "color").Updates(l).Error
}

// addToCounts adds to the numbers of issues and closed issues of the label.
// The database adds to the numbers it has rather than saving the ones of the
// label, so that changes made since the label was read are kept.
func (s *LabelsStore) addToCounts(ctx context.Context, label *Label, issues, closedIssues int) error {
	err := s.db.WithContext(ctx).Exec(
		"UPDATE label SET num_issues = num_issues + ?, num_closed_issues = num_closed_issues + ? WHERE id = ?",
		issues, closedIssues, label.ID,
	).Error
	if err != nil {
		return err
	}
	label.NumIssues += issues
	label.NumClosedIssues += closedIssues
	return nil
}

// DeleteByID deletes the label with given ID of the repository with given ID,
// and removes it from issues. Deleting a label that does not exist is not an
// error.
func (s *LabelsStore) DeleteByID(ctx context.Context, repoID, id int64) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Where("repo_id = ? AND id = ?", repoID, id).Delete(new(Label))
		if result.Error != nil {
			return errors.Wrap(result.Error, "delete label")
		} else if result.RowsAffected == 0 {
			return nil
		}
		return tx.Where("label_id = ?", id).Delete(new(IssueLabel)).Error
	})
}

// AddToIssue adds given labels to the issue, and appends them to its labels.
// Labels that the issue has are skipped.
func (s *LabelsStore) AddToIssue(ctx context.Context, issue *Issue, labels ...*Label) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return newLabelsStore(tx).addToIssue(ctx, issue, labels...)
	})
}

// addToIssue is AddToIssue without a transaction of its own.
func (s *LabelsStore) addToIssue(ctx context.Context, issue *Issue, labels ...*Label) error {
	db := s.db.WithContext(ctx)
	for _, label := range labels {
		var count int64
		err := db.Model(new(IssueLabel)).Where("issue_id = ? AND label_id = ?", issue.ID, label.ID).Count(&count).Error
		if err != nil {
			return errors.Wrap(err, "check issue label")
		} else if count > 0 {
			continue
		}

		err = db.Create(&IssueLabel{IssueID: issue.ID, LabelID: label.ID}).Error
		if err != nil {
			return errors.Wrapf(err, "add label %d", label.ID)
		}

		closedIssues := 0
		if issue.IsClosed {
			closedIssues = 1
		}
		if err = s.addToCounts(ctx, label, 1, closedIssues); err != nil {
			return errors.Wrapf(err, "count issues of label %d", label.ID)
		}
		issue.Labels = append(issue.Labels, label)
	}
	return nil
}

// RemoveFromIssue removes the label from the issue and from its labels.
// Removing a label that the issue does not have is a no-op.
func (s *LabelsStore) RemoveFromIssue(ctx context.Context, issue *Issue, label *Label) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return newLabelsStore(tx).removeFromIssue(ctx, issue, label)
	})
}

// removeFromIssue is RemoveFromIssue without a transaction of its own.
func (s *LabelsStore) removeFromIssue(ctx context.Context, issue *Issue, label *Label) error {
	result := s.db.WithContext(ctx).Where("issue_id = ? AND label_id = ?", issue.ID, label.ID).Delete(new(IssueLabel))
	if result.Error != nil {
		return errors.Wrap(result.Error, "remove label")
	} else if result.RowsAffected == 0 {
		return nil
	}

	closedIssues := 0
	if issue.IsClosed {
		closedIssues = -1
	}
	err := s.addToCounts(ctx, label, -1, closedIssues)
	if err != nil {
		return errors.Wrap(err, "count issues of label")
	}

	issue.Labels = slices.DeleteFunc(issue.Labels, func(l *Label) bool { return l.ID == label.ID })
	return nil
}
