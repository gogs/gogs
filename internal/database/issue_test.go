package database

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdateIssueMentions(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	t.Parallel()

	db := newTestDB(t, "updateIssueMentions")

	users := []*User{
		{ID: 1, LowerName: "alice", Name: "alice"},
		{ID: 2, LowerName: "bob", Name: "bob"},
		{ID: 3, LowerName: "carol", Name: "carol"},
		{ID: 4, LowerName: "acme", Name: "acme", Type: UserTypeOrganization, NumMembers: 1},
	}
	require.NoError(t, db.Create(users).Error)
	// The membership row has an ID that belongs to another user.
	require.NoError(t, db.Create(&OrgUser{ID: 1, UID: 3, OrgID: 4}).Error)

	const repoID, issueID = 7, 9
	require.NoError(t, updateIssueMentions(db, repoID, issueID, []string{"Bob", "acme"}))

	var issueUsers []*IssueUser
	require.NoError(t, db.Order("uid").Find(&issueUsers).Error)
	got := make(map[int64]int64, len(issueUsers))
	for _, iu := range issueUsers {
		assert.Equal(t, int64(issueID), iu.IssueID)
		assert.True(t, iu.IsMentioned)
		got[iu.UserID] = iu.RepoID
	}
	assert.Equal(t, map[int64]int64{2: repoID, 3: repoID, 4: repoID}, got)
}
