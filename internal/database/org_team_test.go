package database

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTeamMemberAccesses(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	before := Handle
	Handle = &DB{db: newTestDB(t, "teamMemberAccesses")}
	t.Cleanup(func() { Handle = before })
	db := Handle.db

	const (
		orgID   = int64(100)
		aliceID = int64(1)
		bobID   = int64(2)
		otherID = int64(3)

		repo1 = int64(11)
		repo2 = int64(12)
		// The owner team has no team-repo row for repo3, but still gives access
		// to it like to every repository of the organization.
		repo3     = int64(13)
		otherRepo = int64(14)

		ownersID  = int64(1)
		readersID = int64(2)
		writersID = int64(3)
	)
	require.NoError(t, db.Create([]*User{
		{ID: orgID, Name: "org", LowerName: "org", Type: UserTypeOrganization, NumMembers: 1},
		{ID: aliceID, Name: "alice", LowerName: "alice", Type: UserTypeIndividual},
		{ID: bobID, Name: "bob", LowerName: "bob", Type: UserTypeIndividual},
		{ID: otherID, Name: "other", LowerName: "other", Type: UserTypeIndividual},
	}).Error)
	require.NoError(t, db.Create([]*Repository{
		{ID: repo1, OwnerID: orgID, Name: "r1", LowerName: "r1"},
		{ID: repo2, OwnerID: orgID, Name: "r2", LowerName: "r2"},
		{ID: repo3, OwnerID: orgID, Name: "r3", LowerName: "r3"},
		{ID: otherRepo, OwnerID: otherID, Name: "r4", LowerName: "r4"},
	}).Error)
	require.NoError(t, db.Create([]*Team{
		{ID: ownersID, OrgID: orgID, Name: ownerTeamName, LowerName: "owners", Authorize: AccessModeOwner, NumMembers: 1},
		{ID: readersID, OrgID: orgID, Name: "Readers", LowerName: "readers", Authorize: AccessModeRead},
		{ID: writersID, OrgID: orgID, Name: "Writers", LowerName: "writers", Authorize: AccessModeWrite},
	}).Error)
	require.NoError(t, db.Create([]*TeamRepo{
		{OrgID: orgID, TeamID: ownersID, RepoID: repo1},
		{OrgID: orgID, TeamID: ownersID, RepoID: repo2},
		{OrgID: orgID, TeamID: readersID, RepoID: repo1},
		{OrgID: orgID, TeamID: readersID, RepoID: repo2},
		{OrgID: orgID, TeamID: writersID, RepoID: repo1},
	}).Error)
	require.NoError(t, db.Create(&OrgUser{UID: aliceID, OrgID: orgID, IsOwner: true, NumTeams: 1}).Error)
	require.NoError(t, db.Create(&TeamUser{UID: aliceID, OrgID: orgID, TeamID: ownersID}).Error)
	require.NoError(t, db.Create([]*Collaboration{
		{UserID: bobID, RepoID: repo2, Mode: AccessModeAdmin},
		{UserID: bobID, RepoID: repo3, Mode: AccessModeWrite},
	}).Error)
	require.NoError(t, db.Create([]*Access{
		{UserID: aliceID, RepoID: repo1, Mode: AccessModeOwner},
		{UserID: aliceID, RepoID: repo2, Mode: AccessModeOwner},
		{UserID: aliceID, RepoID: repo3, Mode: AccessModeOwner},
		{UserID: bobID, RepoID: repo2, Mode: AccessModeAdmin},
		{UserID: bobID, RepoID: repo3, Mode: AccessModeWrite},
		{UserID: bobID, RepoID: otherRepo, Mode: AccessModeRead},
	}).Error)

	accesses := func(t *testing.T) map[int64]map[int64]AccessMode {
		var all []*Access
		require.NoError(t, db.Find(&all).Error)
		got := make(map[int64]map[int64]AccessMode)
		for _, a := range all {
			if got[a.UserID] == nil {
				got[a.UserID] = make(map[int64]AccessMode)
			}
			got[a.UserID][a.RepoID] = a.Mode
		}
		return got
	}
	alice := map[int64]AccessMode{repo1: AccessModeOwner, repo2: AccessModeOwner, repo3: AccessModeOwner}

	// Each step adds or removes Bob, whose access to a repository is the highest
	// of his collaboration and his teams. Other accesses must not change.
	steps := []struct {
		name    string
		change  func() error
		wantBob map[int64]AccessMode
	}{
		{
			name:    "add to readers",
			change:  func() error { return AddTeamMember(orgID, readersID, bobID) },
			wantBob: map[int64]AccessMode{repo1: AccessModeRead, repo2: AccessModeAdmin, repo3: AccessModeWrite, otherRepo: AccessModeRead},
		},
		{
			name:    "add to writers",
			change:  func() error { return AddTeamMember(orgID, writersID, bobID) },
			wantBob: map[int64]AccessMode{repo1: AccessModeWrite, repo2: AccessModeAdmin, repo3: AccessModeWrite, otherRepo: AccessModeRead},
		},
		{
			name:    "add to owners",
			change:  func() error { return AddTeamMember(orgID, ownersID, bobID) },
			wantBob: map[int64]AccessMode{repo1: AccessModeOwner, repo2: AccessModeOwner, repo3: AccessModeOwner, otherRepo: AccessModeRead},
		},
		{
			name:    "remove from owners",
			change:  func() error { return RemoveTeamMember(orgID, ownersID, bobID) },
			wantBob: map[int64]AccessMode{repo1: AccessModeWrite, repo2: AccessModeAdmin, repo3: AccessModeWrite, otherRepo: AccessModeRead},
		},
		{
			name:    "remove from writers",
			change:  func() error { return RemoveTeamMember(orgID, writersID, bobID) },
			wantBob: map[int64]AccessMode{repo1: AccessModeRead, repo2: AccessModeAdmin, repo3: AccessModeWrite, otherRepo: AccessModeRead},
		},
		{
			name:    "remove from readers",
			change:  func() error { return RemoveTeamMember(orgID, readersID, bobID) },
			wantBob: map[int64]AccessMode{repo2: AccessModeAdmin, repo3: AccessModeWrite, otherRepo: AccessModeRead},
		},
	}
	for _, step := range steps {
		require.NoError(t, step.change(), step.name)
		want := map[int64]map[int64]AccessMode{aliceID: alice, bobID: step.wantBob}
		assert.Equal(t, want, accesses(t), step.name)
	}
}
