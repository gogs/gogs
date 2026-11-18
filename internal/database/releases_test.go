package database

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"unknwon.dev/x/logx"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/git"
)

func TestReleases(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	t.Parallel()

	ctx := context.Background()
	s := &ReleasesStore{
		db: newTestDB(t, "ReleasesStore"),
	}

	for _, tc := range []struct {
		name string
		test func(t *testing.T, ctx context.Context, s *ReleasesStore)
	}{
		{"Create", releasesCreate},
		{"Get", releasesGet},
		{"List", releasesList},
		{"LoadAttributes", releasesLoadAttributes},
		{"Update", releasesUpdate},
		{"DeleteByID", releasesDeleteByID},
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

// newTestReleaseRepo creates a user of given name and a repository it owns,
// whose Git repository has a commit on the "main" branch.
//
// NOTE: Owners must have names that no other test uses, because tests that
// delete users remove their directories under the repository root that is
// mocked at that time.
func newTestReleaseRepo(t *testing.T, ctx context.Context, s *ReleasesStore, ownerName string) (*Repository, *git.Repository) {
	t.Helper()

	owner, err := newUsersStore(s.db).Create(ctx, ownerName, ownerName+"@example.com", CreateUserOptions{})
	require.NoError(t, err)
	repo, err := newReposStore(s.db).Create(ctx, owner.ID, CreateRepoOptions{Name: "repo1"})
	require.NoError(t, err)
	repo.Owner = owner

	run := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		output, err := cmd.CombinedOutput()
		require.NoError(t, err, string(output))
	}
	work := t.TempDir()
	run(work, "init", "--initial-branch=main")
	run(work, "-c", "user.name=alice", "-c", "user.email=alice@example.com", "commit", "--allow-empty", "--message=initial")
	run(work, "clone", "--bare", work, repo.RepoPath())

	gitRepo, err := git.Open(repo.RepoPath())
	require.NoError(t, err)
	return repo, gitRepo
}

func countTestHookTasks(t *testing.T, s *ReleasesStore) int64 {
	t.Helper()

	var count int64
	err := s.db.Model(new(HookTask)).Where("event_type = ?", HookEventTypeRelease).Count(&count).Error
	require.NoError(t, err)
	return count
}

func releasesCreate(t *testing.T, ctx context.Context, s *ReleasesStore) {
	// Webhook payloads of releases have links, which read the server and SSH
	// settings that other suites mock.
	conf.SetMockServer(t, conf.ServerOpts{})
	conf.SetMockSSH(t, conf.SSHOpts{})
	conf.SetMockRepository(t, conf.RepositoryOpts{Root: t.TempDir()})
	setMockAttachmentPath(t)
	logger := logx.NewNoopLogger()
	attachments := newAttachmentsStore(s.db)

	repo, gitRepo := newTestReleaseRepo(t, ctx, s, "rel-create")
	newTestWebhook(t, ctx, newWebhooksStore(s.db), repo.ID, 0, HookEvents{Release: true})
	unbound := newTestAttachment(t, ctx, attachments, "unbound", 0, 0, 0)
	ofIssue := newTestAttachment(t, ctx, attachments, "of-issue", 9, 0, 0)

	// A draft gets no tag and is not announced.
	draft := &Release{RepoID: repo.ID, PublisherID: repo.OwnerID, TagName: "Draft-1", Target: "main", IsDraft: true}
	err := s.Create(ctx, logger, gitRepo, draft, []string{unbound.UUID, ofIssue.UUID, "not-a-uuid"})
	require.NoError(t, err)
	assert.NotZero(t, draft.ID)
	assert.False(t, gitRepo.HasTag(ctx, "Draft-1"), "tag of draft created")
	assert.Zero(t, countTestHookTasks(t, s))

	got, err := s.GetByID(ctx, draft.ID)
	require.NoError(t, err)
	assert.Equal(t, "draft-1", got.LowerTagName)
	assert.NotZero(t, got.CreatedUnix)
	bound, err := attachments.ListByReleaseID(ctx, draft.ID)
	require.NoError(t, err)
	assert.Equal(t, []string{unbound.UUID}, attachmentUUIDs(bound))

	// Publishing creates the tag and announces the release.
	published := &Release{RepoID: repo.ID, PublisherID: repo.OwnerID, TagName: "v1.0", Target: "main", Title: "First"}
	err = s.Create(ctx, logger, gitRepo, published, nil)
	require.NoError(t, err)
	assert.True(t, gitRepo.HasTag(ctx, "v1.0"), "tag not created")
	assert.Equal(t, int64(1), countTestHookTasks(t, s))

	// Tag names are unique regardless of their case.
	err = s.Create(ctx, logger, gitRepo, &Release{RepoID: repo.ID, TagName: "V1.0", Target: "main"}, nil)
	assert.True(t, IsErrReleaseAlreadyExist(err), "got %v", err)
	// Unless they belong to another repository.
	err = s.Create(ctx, logger, gitRepo, &Release{RepoID: 9999, TagName: "draft-1", IsDraft: true}, nil)
	require.NoError(t, err)

	err = s.Create(ctx, logger, gitRepo, &Release{RepoID: repo.ID, TagName: "bad..name", Target: "main"}, nil)
	assert.True(t, IsErrInvalidTagName(err), "got %v", err)
	_, err = s.GetByTagName(ctx, repo.ID, "bad..name")
	assert.True(t, IsErrReleaseNotExist(err), "got %v", err)
}

func releasesGet(t *testing.T, ctx context.Context, s *ReleasesStore) {
	r := &Release{RepoID: 1, TagName: "V1.0", LowerTagName: "v1.0"}
	err := s.db.Create(r).Error
	require.NoError(t, err)
	err = s.db.Create(&Release{RepoID: 2, TagName: "v2.0", LowerTagName: "v2.0"}).Error
	require.NoError(t, err)

	got, err := s.GetByTagName(ctx, 1, "v1.0")
	require.NoError(t, err)
	assert.Equal(t, r.ID, got.ID)
	assert.Equal(t, "V1.0", got.TagName)
	assert.Equal(t, r.CreatedUnix, got.Created.Unix())

	_, err = s.GetByTagName(ctx, 1, "v2.0")
	assert.True(t, IsErrReleaseNotExist(err), "got %v", err)
	_, err = s.GetByTagName(ctx, 1, "")
	assert.True(t, IsErrReleaseNotExist(err), "got %v", err)

	got, err = s.GetByID(ctx, r.ID)
	require.NoError(t, err)
	assert.Equal(t, "V1.0", got.TagName)
	_, err = s.GetByID(ctx, 9999)
	assert.True(t, IsErrReleaseNotExist(err), "got %v", err)
}

func releasesList(t *testing.T, ctx context.Context, s *ReleasesStore) {
	create := func(repoID int64, tagName string, isDraft bool, createdUnix int64) *Release {
		r := &Release{RepoID: repoID, TagName: tagName, LowerTagName: tagName, IsDraft: isDraft, CreatedUnix: createdUnix}
		err := s.db.Create(r).Error
		require.NoError(t, err)
		return r
	}
	older := create(1, "v1", false, 1000)
	draft1 := create(1, "v3", true, 3000)
	newer := create(1, "v2", false, 2000)
	draft2 := create(1, "v4", true, 500)
	create(2, "v1", false, 4000)
	create(2, "v9", true, 4000)

	ids := func(releases []*Release) []int64 {
		ids := make([]int64, 0, len(releases))
		for _, r := range releases {
			ids = append(ids, r.ID)
		}
		return ids
	}

	got, err := s.ListPublishedByRepoID(ctx, 1)
	require.NoError(t, err)
	assert.Equal(t, []int64{newer.ID, older.ID}, ids(got))

	got, err = s.ListPublishedByRepoID(ctx, 1, "v1", "v3", "v404")
	require.NoError(t, err)
	assert.Equal(t, []int64{older.ID}, ids(got))

	got, err = s.ListDraftsByRepoID(ctx, 1)
	require.NoError(t, err)
	assert.Equal(t, []int64{draft1.ID, draft2.ID}, ids(got))

	got, err = s.ListByRepoID(ctx, 1)
	require.NoError(t, err)
	assert.Equal(t, []int64{older.ID, draft1.ID, newer.ID, draft2.ID}, ids(got))

	got, err = s.ListByRepoID(ctx, 404)
	require.NoError(t, err)
	assert.NotNil(t, got)
	assert.Empty(t, got)
}

func releasesLoadAttributes(t *testing.T, ctx context.Context, s *ReleasesStore) {
	setMockAttachmentPath(t)

	owner, err := newUsersStore(s.db).Create(ctx, "rel-load", "rel-load@example.com", CreateUserOptions{})
	require.NoError(t, err)
	repo, err := newReposStore(s.db).Create(ctx, owner.ID, CreateRepoOptions{Name: "repo1"})
	require.NoError(t, err)

	withPublisher := &Release{RepoID: repo.ID, PublisherID: owner.ID, TagName: "v1", LowerTagName: "v1"}
	require.NoError(t, s.db.Create(withPublisher).Error)
	withoutPublisher := &Release{RepoID: repo.ID, PublisherID: 9999, TagName: "v2", LowerTagName: "v2"}
	require.NoError(t, s.db.Create(withoutPublisher).Error)
	a := newTestAttachment(t, ctx, newAttachmentsStore(s.db), "a", 0, 0, withPublisher.ID)

	err = s.LoadAttributes(ctx, withPublisher, withoutPublisher)
	require.NoError(t, err)
	require.NotNil(t, withPublisher.Repo)
	assert.Equal(t, repo.ID, withPublisher.Repo.ID)
	require.NotNil(t, withPublisher.Repo.Owner)
	assert.Equal(t, owner.ID, withPublisher.Repo.Owner.ID)
	require.NotNil(t, withPublisher.Publisher)
	assert.Equal(t, owner.ID, withPublisher.Publisher.ID)
	assert.Equal(t, []string{a.UUID}, attachmentUUIDs(withPublisher.Attachments))

	assert.Equal(t, int64(-1), withoutPublisher.PublisherID)
	assert.Equal(t, NewGhostUser().Name, withoutPublisher.Publisher.Name)
	assert.NotNil(t, withoutPublisher.Attachments)
	assert.Empty(t, withoutPublisher.Attachments)

	// Attributes that are set are kept.
	preset := &Release{RepoID: 9999, PublisherID: 9999, Repo: repo, Publisher: owner, Attachments: []*Attachment{}}
	err = s.LoadAttributes(ctx, preset)
	require.NoError(t, err)
	assert.Same(t, repo, preset.Repo)
	assert.Same(t, owner, preset.Publisher)
	assert.Equal(t, int64(9999), preset.PublisherID)

	err = s.LoadAttributes(ctx, &Release{RepoID: 9999})
	assert.Error(t, err)
}

func releasesUpdate(t *testing.T, ctx context.Context, s *ReleasesStore) {
	// Webhook payloads of releases have links, which read the server and SSH
	// settings that other suites mock.
	conf.SetMockServer(t, conf.ServerOpts{})
	conf.SetMockSSH(t, conf.SSHOpts{})
	conf.SetMockRepository(t, conf.RepositoryOpts{Root: t.TempDir()})
	setMockAttachmentPath(t)
	logger := logx.NewNoopLogger()
	attachments := newAttachmentsStore(s.db)

	repo, gitRepo := newTestReleaseRepo(t, ctx, s, "rel-update")
	newTestWebhook(t, ctx, newWebhooksStore(s.db), repo.ID, 0, HookEvents{Release: true})
	doer, err := newUsersStore(s.db).Create(ctx, "rel-update-doer", "rel-update-doer@example.com", CreateUserOptions{})
	require.NoError(t, err)

	r := &Release{RepoID: repo.ID, PublisherID: repo.OwnerID, TagName: "v1.0", Target: "main", IsDraft: true}
	dropped := newTestAttachment(t, ctx, attachments, "dropped", 0, 0, 0)
	kept := newTestAttachment(t, ctx, attachments, "kept", 0, 0, 0)
	err = s.Create(ctx, logger, gitRepo, r, []string{dropped.UUID, kept.UUID})
	require.NoError(t, err)
	added := newTestAttachment(t, ctx, attachments, "added", 0, 0, 0)
	ofIssue := newTestAttachment(t, ctx, attachments, "of-issue", 9, 0, 0)

	// Saving a draft keeps it unannounced and without a tag.
	r.Title = "Draft title"
	err = s.Update(ctx, logger, doer, gitRepo, r, false, []string{kept.UUID, dropped.UUID})
	require.NoError(t, err)
	assert.False(t, gitRepo.HasTag(ctx, "v1.0"), "tag of draft created")
	assert.Zero(t, countTestHookTasks(t, s))

	r.Title = "Published"
	r.IsDraft = false
	err = s.Update(ctx, logger, doer, gitRepo, r, true, []string{kept.UUID, added.UUID, ofIssue.UUID})
	require.NoError(t, err)
	assert.True(t, gitRepo.HasTag(ctx, "v1.0"), "tag not created")
	assert.Equal(t, int64(1), countTestHookTasks(t, s))

	got, err := s.GetByID(ctx, r.ID)
	require.NoError(t, err)
	assert.Equal(t, "Published", got.Title)
	assert.False(t, got.IsDraft)
	assert.Equal(t, doer.ID, got.PublisherID)

	bound, err := attachments.ListByReleaseID(ctx, r.ID)
	require.NoError(t, err)
	assert.Equal(t, []string{kept.UUID, added.UUID}, attachmentUUIDs(bound))
	// Attachments that are left out are detached but not deleted.
	gotDropped, err := attachments.GetByUUID(ctx, dropped.UUID)
	require.NoError(t, err)
	assert.Zero(t, gotDropped.ReleaseID)
	gotOfIssue, err := attachments.GetByUUID(ctx, ofIssue.UUID)
	require.NoError(t, err)
	assert.Equal(t, []int64{9, 0}, []int64{gotOfIssue.IssueID, gotOfIssue.ReleaseID})

	// A draft gets its tag only when published, which is when an invalid tag
	// name is found.
	bad := &Release{RepoID: repo.ID, PublisherID: repo.OwnerID, TagName: "bad..name", Target: "main", IsDraft: true}
	err = s.Create(ctx, logger, gitRepo, bad, nil)
	require.NoError(t, err)
	bad.IsDraft = false
	err = s.Update(ctx, logger, doer, gitRepo, bad, true, nil)
	assert.True(t, IsErrInvalidTagName(err), "got %v", err)
}

func releasesDeleteByID(t *testing.T, ctx context.Context, s *ReleasesStore) {
	// Webhook payloads of releases have links, which read the server and SSH
	// settings that other suites mock.
	conf.SetMockServer(t, conf.ServerOpts{})
	conf.SetMockSSH(t, conf.SSHOpts{})
	conf.SetMockRepository(t, conf.RepositoryOpts{Root: t.TempDir()})
	setMockAttachmentPath(t)
	logger := logx.NewNoopLogger()

	repo, gitRepo := newTestReleaseRepo(t, ctx, s, "rel-delete")
	r := &Release{RepoID: repo.ID, PublisherID: repo.OwnerID, TagName: "v1.0", Target: "main"}
	err := s.Create(ctx, logger, gitRepo, r, nil)
	require.NoError(t, err)
	other := &Release{RepoID: repo.ID, PublisherID: repo.OwnerID, TagName: "v2.0", Target: "main"}
	err = s.Create(ctx, logger, gitRepo, other, nil)
	require.NoError(t, err)
	attachments := newAttachmentsStore(s.db)
	// A file that cannot be removed does not stop the deletion.
	stuck := newTestAttachment(t, ctx, attachments, "stuck", 0, 0, r.ID)
	err = os.Remove(stuck.LocalPath())
	require.NoError(t, err)
	err = os.MkdirAll(filepath.Join(stuck.LocalPath(), "child"), os.ModePerm)
	require.NoError(t, err)
	ofRelease := newTestAttachment(t, ctx, attachments, "of-release", 0, 0, r.ID)
	ofOther := newTestAttachment(t, ctx, attachments, "of-other", 0, 0, other.ID)

	// A release of another repository is left alone.
	err = s.DeleteByID(ctx, logger, 9999, r.ID)
	require.NoError(t, err)
	_, err = s.GetByID(ctx, r.ID)
	require.NoError(t, err)
	assert.True(t, gitRepo.HasTag(ctx, "v1.0"), "tag deleted")

	err = s.DeleteByID(ctx, logger, repo.ID, r.ID)
	require.NoError(t, err)
	_, err = s.GetByID(ctx, r.ID)
	assert.True(t, IsErrReleaseNotExist(err), "got %v", err)
	assert.False(t, gitRepo.HasTag(ctx, "v1.0"), "tag not deleted")
	assert.True(t, gitRepo.HasTag(ctx, "v2.0"), "tag of other release deleted")
	_, err = attachments.GetByUUID(ctx, ofRelease.UUID)
	assert.True(t, IsErrAttachmentNotExist(err), "attachment not deleted: %v", err)
	assert.NoFileExists(t, ofRelease.LocalPath())
	_, err = attachments.GetByUUID(ctx, stuck.UUID)
	assert.True(t, IsErrAttachmentNotExist(err), "attachment not deleted: %v", err)
	_, err = attachments.GetByUUID(ctx, ofOther.UUID)
	assert.NoError(t, err, "attachment of other release deleted")
	assert.FileExists(t, ofOther.LocalPath())

	err = s.DeleteByID(ctx, logger, repo.ID, r.ID)
	assert.True(t, IsErrReleaseNotExist(err), "got %v", err)

	// A release whose tag is gone is still deleted.
	err = git.RepoDeleteTag(ctx, repo.RepoPath(), "v2.0")
	require.NoError(t, err)
	err = s.DeleteByID(ctx, logger, repo.ID, other.ID)
	require.NoError(t, err)
	_, err = s.GetByID(ctx, other.ID)
	assert.True(t, IsErrReleaseNotExist(err), "got %v", err)
}
