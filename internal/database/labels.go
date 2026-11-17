package database

import (
	"context"
	"slices"

	"github.com/cockroachdb/errors"
	"gorm.io/gorm"
)

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

// Update saves all fields of the label.
func (s *LabelsStore) Update(ctx context.Context, l *Label) error {
	return s.db.WithContext(ctx).Model(l).Select("*").Updates(l).Error
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

		label.NumIssues++
		if issue.IsClosed {
			label.NumClosedIssues++
		}
		if err = s.Update(ctx, label); err != nil {
			return errors.Wrapf(err, "update label %d", label.ID)
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

	label.NumIssues--
	if issue.IsClosed {
		label.NumClosedIssues--
	}
	err := s.Update(ctx, label)
	if err != nil {
		return errors.Wrap(err, "update label")
	}

	issue.Labels = slices.DeleteFunc(issue.Labels, func(l *Label) bool { return l.ID == label.ID })
	return nil
}
