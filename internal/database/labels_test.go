package database

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLabels(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	t.Parallel()

	ctx := context.Background()
	s := &LabelsStore{
		db: newTestDB(t, "LabelsStore"),
	}

	for _, tc := range []struct {
		name string
		test func(t *testing.T, ctx context.Context, s *LabelsStore)
	}{
		{"Get", labelsGet},
		{"List", labelsList},
		{"Update", labelsUpdate},
		{"DeleteByID", labelsDeleteByID},
		{"AddToIssue", labelsAddToIssue},
		{"RemoveFromIssue", labelsRemoveFromIssue},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Cleanup(func() {
				err := clearTables(t, s.db)
				require.NoError(t, err)
			})
			tc.test(t, ctx, s)
		})
		if t.Failed() {
			break
		}
	}
}

func newTestLabel(t *testing.T, ctx context.Context, s *LabelsStore, repoID int64, name string) *Label {
	t.Helper()

	l := &Label{RepoID: repoID, Name: name, Color: "#123456"}
	err := s.Create(ctx, l)
	require.NoError(t, err)
	require.NotZero(t, l.ID)
	return l
}

func labelIDs(labels []*Label) []int64 {
	ids := make([]int64, 0, len(labels))
	for _, l := range labels {
		ids = append(ids, l.ID)
	}
	return ids
}

func labelsGet(t *testing.T, ctx context.Context, s *LabelsStore) {
	bug := newTestLabel(t, ctx, s, 1, "bug")
	newTestLabel(t, ctx, s, 2, "feature")

	got, err := s.GetByID(ctx, 1, bug.ID)
	require.NoError(t, err)
	assert.Equal(t, "bug", got.Name)
	assert.Equal(t, "#123456", got.Color)

	got, err = s.GetByName(ctx, 1, "bug")
	require.NoError(t, err)
	assert.Equal(t, bug.ID, got.ID)

	// Labels of other repositories are not found.
	_, err = s.GetByID(ctx, 2, bug.ID)
	assert.True(t, IsErrLabelNotExist(err), "got %v", err)
	_, err = s.GetByName(ctx, 1, "feature")
	assert.True(t, IsErrLabelNotExist(err), "got %v", err)
	_, err = s.GetByID(ctx, 1, 0)
	assert.True(t, IsErrLabelNotExist(err), "got %v", err)
}

func labelsList(t *testing.T, ctx context.Context, s *LabelsStore) {
	zeta := newTestLabel(t, ctx, s, 1, "zeta")
	alpha := newTestLabel(t, ctx, s, 1, "alpha")
	beta := newTestLabel(t, ctx, s, 1, "beta")
	other := newTestLabel(t, ctx, s, 2, "aaa")

	got, err := s.ListByRepoID(ctx, 1)
	require.NoError(t, err)
	assert.Equal(t, []int64{alpha.ID, beta.ID, zeta.ID}, labelIDs(got))

	got, err = s.ListByIDs(ctx, 1, []int64{zeta.ID, other.ID, alpha.ID, 9999})
	require.NoError(t, err)
	assert.Equal(t, []int64{alpha.ID, zeta.ID}, labelIDs(got))

	got, err = s.ListByIDs(ctx, 1, nil)
	require.NoError(t, err)
	assert.Empty(t, got)

	for _, l := range []*Label{zeta, alpha} {
		err = s.db.Create(&IssueLabel{IssueID: 7, LabelID: l.ID}).Error
		require.NoError(t, err)
	}
	err = s.db.Create(&IssueLabel{IssueID: 8, LabelID: beta.ID}).Error
	require.NoError(t, err)

	got, err = s.ListByIssueID(ctx, 7)
	require.NoError(t, err)
	assert.Equal(t, []int64{alpha.ID, zeta.ID}, labelIDs(got))
	assert.Equal(t, "alpha", got[0].Name)

	got, err = s.ListByIssueID(ctx, 404)
	require.NoError(t, err)
	assert.NotNil(t, got)
	assert.Empty(t, got)
}

func labelsUpdate(t *testing.T, ctx context.Context, s *LabelsStore) {
	l := newTestLabel(t, ctx, s, 1, "bug")
	other := newTestLabel(t, ctx, s, 1, "feature")

	l.Name = "defect"
	l.Color = "#abcdef"
	l.NumIssues = 0
	err := s.Update(ctx, l)
	require.NoError(t, err)

	got, err := s.GetByID(ctx, 1, l.ID)
	require.NoError(t, err)
	assert.Equal(t, "defect", got.Name)
	assert.Equal(t, "#abcdef", got.Color)

	got, err = s.GetByID(ctx, 1, other.ID)
	require.NoError(t, err)
	assert.Equal(t, "feature", got.Name)
}

func labelsDeleteByID(t *testing.T, ctx context.Context, s *LabelsStore) {
	bug := newTestLabel(t, ctx, s, 1, "bug")
	feature := newTestLabel(t, ctx, s, 1, "feature")
	for _, l := range []*Label{bug, feature} {
		err := s.db.Create(&IssueLabel{IssueID: 7, LabelID: l.ID}).Error
		require.NoError(t, err)
	}

	// A label of another repository is left alone.
	err := s.DeleteByID(ctx, 2, bug.ID)
	require.NoError(t, err)
	got, err := s.ListByIssueID(ctx, 7)
	require.NoError(t, err)
	assert.Equal(t, []int64{bug.ID, feature.ID}, labelIDs(got))

	err = s.DeleteByID(ctx, 1, bug.ID)
	require.NoError(t, err)
	_, err = s.GetByID(ctx, 1, bug.ID)
	assert.True(t, IsErrLabelNotExist(err), "got %v", err)
	var count int64
	err = s.db.Model(new(IssueLabel)).Where("label_id = ?", bug.ID).Count(&count).Error
	require.NoError(t, err)
	assert.Zero(t, count)
	got, err = s.ListByIssueID(ctx, 7)
	require.NoError(t, err)
	assert.Equal(t, []int64{feature.ID}, labelIDs(got))

	err = s.DeleteByID(ctx, 1, bug.ID)
	require.NoError(t, err)
}

func labelsAddToIssue(t *testing.T, ctx context.Context, s *LabelsStore) {
	bug := newTestLabel(t, ctx, s, 1, "bug")
	feature := newTestLabel(t, ctx, s, 1, "feature")

	open := &Issue{ID: 7}
	err := s.AddToIssue(ctx, open, bug, feature)
	require.NoError(t, err)
	assert.Equal(t, []int64{bug.ID, feature.ID}, labelIDs(open.Labels))

	closed := &Issue{ID: 8, IsClosed: true}
	err = s.AddToIssue(ctx, closed, bug)
	require.NoError(t, err)

	// Adding a label that the issue has changes nothing.
	err = s.AddToIssue(ctx, open, bug)
	require.NoError(t, err)
	assert.Len(t, open.Labels, 2)

	got, err := s.GetByID(ctx, 1, bug.ID)
	require.NoError(t, err)
	assert.Equal(t, 2, got.NumIssues)
	assert.Equal(t, 1, got.NumClosedIssues)
	got, err = s.GetByID(ctx, 1, feature.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, got.NumIssues)
	assert.Equal(t, 0, got.NumClosedIssues)

	labels, err := s.ListByIssueID(ctx, 7)
	require.NoError(t, err)
	assert.Equal(t, []int64{bug.ID, feature.ID}, labelIDs(labels))
}

func labelsRemoveFromIssue(t *testing.T, ctx context.Context, s *LabelsStore) {
	bug := newTestLabel(t, ctx, s, 1, "bug")
	feature := newTestLabel(t, ctx, s, 1, "feature")

	closed := &Issue{ID: 7, IsClosed: true}
	err := s.AddToIssue(ctx, closed, bug, feature)
	require.NoError(t, err)
	open := &Issue{ID: 8}
	err = s.AddToIssue(ctx, open, bug)
	require.NoError(t, err)

	err = s.RemoveFromIssue(ctx, closed, bug)
	require.NoError(t, err)
	assert.Equal(t, []int64{feature.ID}, labelIDs(closed.Labels))
	got, err := s.GetByID(ctx, 1, bug.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, got.NumIssues)
	assert.Equal(t, 0, got.NumClosedIssues)

	// Removing a label that the issue does not have changes no counts.
	err = s.RemoveFromIssue(ctx, closed, bug)
	require.NoError(t, err)
	got, err = s.GetByID(ctx, 1, bug.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, got.NumIssues)

	labels, err := s.ListByIssueID(ctx, 7)
	require.NoError(t, err)
	assert.Equal(t, []int64{feature.ID}, labelIDs(labels))
	labels, err = s.ListByIssueID(ctx, 8)
	require.NoError(t, err)
	assert.Equal(t, []int64{bug.ID}, labelIDs(labels))
}
