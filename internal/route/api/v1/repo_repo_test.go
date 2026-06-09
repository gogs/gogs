package v1

import (
	stdctx "context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"gopkg.in/macaron.v1"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/errx"
)

// testForkStore is a configurable test double for forkStore.
type testForkStore struct {
	getUserByUsernameFn    func(ctx stdctx.Context, username string) (*database.User, error)
	getOrgByNameFn        func(name string) (*database.User, error)
	getRepositoryByNameFn func(ctx stdctx.Context, ownerID int64, name string) (*database.Repository, error)
	forkRepositoryFn      func(doer, owner *database.User, baseRepo *database.Repository, name, desc string) (*database.Repository, error)
}

func (s *testForkStore) getUserByUsername(ctx stdctx.Context, username string) (*database.User, error) {
	return s.getUserByUsernameFn(ctx, username)
}

func (s *testForkStore) getOrgByName(name string) (*database.User, error) {
	return s.getOrgByNameFn(name)
}

func (s *testForkStore) getRepositoryByName(ctx stdctx.Context, ownerID int64, name string) (*database.Repository, error) {
	return s.getRepositoryByNameFn(ctx, ownerID, name)
}

func (s *testForkStore) forkRepository(doer, owner *database.User, baseRepo *database.Repository, name, desc string) (*database.Repository, error) {
	return s.forkRepositoryFn(doer, owner, baseRepo, name, desc)
}

// notFoundErr is a test error that satisfies errx.NotFound.
type notFoundErr struct{ msg string }

func (e notFoundErr) Error() string { return e.msg }
func (notFoundErr) NotFound() bool  { return true }

var _ errx.NotFound = notFoundErr{}

// newForkAPIContext constructs an *context.APIContext suitable for unit tests.
// The loggedInUser is set as the authenticated user; URL params come from
// macaron's routing when the handler is invoked via a real macaron instance.
func newForkAPIContext(mc *macaron.Context, loggedInUser *database.User) *context.APIContext {
	internalCtx := &context.Context{
		Context: mc,
		User:    loggedInUser,
		Repo:    &context.Repository{PullRequest: &context.PullRequest{}},
	}
	return &context.APIContext{Context: internalCtx}
}

// callDoForkRepo sets up a minimal macaron instance, registers a handler that
// invokes doForkRepo with the given store and opt, and makes a POST request to
// "/:username/:reponame/forks". It returns the HTTP response.
func callDoForkRepo(t *testing.T, loggedInUser *database.User, repoOwner, repoName string, opt forkRepoRequest, store forkStore) *http.Response {
	t.Helper()
	m := macaron.New()
	m.Use(macaron.Renderer())
	m.Use(func(mc *macaron.Context) {
		mc.Map(newForkAPIContext(mc, loggedInUser))
	})
	m.Post("/:username/:reponame/forks", func(c *context.APIContext) {
		doForkRepo(c, store, opt)
	})

	r, _ := http.NewRequest(http.MethodPost, "/"+repoOwner+"/"+repoName+"/forks", http.NoBody)
	rr := httptest.NewRecorder()
	m.ServeHTTP(rr, r)
	return rr.Result()
}

func TestDoForkRepo(t *testing.T) {
	conf.SetMockServer(t, conf.ServerOpts{
		ExternalURL: "https://gogs.example.com/",
	})

	requester := &database.User{ID: 1, Name: "alice"}
	repoOwner := &database.User{ID: 2, Name: "bob"}
	org := &database.User{ID: 3, Name: "myorg"}

	baseRepo := &database.Repository{
		ID:    10,
		Name:  "myrepo",
		Owner: repoOwner,
	}

	tests := []struct {
		name          string
		repoOwner     string
		repoName      string
		opt           forkRepoRequest
		store         *testForkStore
		expStatusCode int
	}{
		{
			name:      "fork to own namespace",
			repoOwner: "bob",
			repoName:  "myrepo",
			opt:       forkRepoRequest{},
			store: &testForkStore{
				getUserByUsernameFn: func(_ stdctx.Context, username string) (*database.User, error) {
					assert.Equal(t, "bob", username)
					return repoOwner, nil
				},
				getRepositoryByNameFn: func(_ stdctx.Context, ownerID int64, name string) (*database.Repository, error) {
					assert.Equal(t, repoOwner.ID, ownerID)
					assert.Equal(t, "myrepo", name)
					return baseRepo, nil
				},
				forkRepositoryFn: func(doer, owner *database.User, _ *database.Repository, name, _ string) (*database.Repository, error) {
					assert.Equal(t, requester.ID, doer.ID)
					assert.Equal(t, requester.ID, owner.ID, "fork owner should be the requesting user when no org given")
					assert.Equal(t, "myrepo", name, "fork name defaults to base repo name")
					return &database.Repository{ID: 11, Name: "myrepo", Owner: requester}, nil
				},
			},
			expStatusCode: http.StatusCreated,
		},
		{
			name:      "fork into organization",
			repoOwner: "bob",
			repoName:  "myrepo",
			opt:       forkRepoRequest{Organization: "myorg"},
			store: &testForkStore{
				getUserByUsernameFn: func(_ stdctx.Context, _ string) (*database.User, error) {
					return repoOwner, nil
				},
				getOrgByNameFn: func(name string) (*database.User, error) {
					assert.Equal(t, "myorg", name)
					return org, nil
				},
				getRepositoryByNameFn: func(_ stdctx.Context, _ int64, _ string) (*database.Repository, error) {
					return baseRepo, nil
				},
				forkRepositoryFn: func(doer, owner *database.User, _ *database.Repository, _, _ string) (*database.Repository, error) {
					assert.Equal(t, requester.ID, doer.ID)
					assert.Equal(t, org.ID, owner.ID, "fork owner should be the specified org")
					return &database.Repository{ID: 12, Name: "myrepo", Owner: org}, nil
				},
			},
			expStatusCode: http.StatusCreated,
		},
		{
			name:      "fork with custom name",
			repoOwner: "bob",
			repoName:  "myrepo",
			opt:       forkRepoRequest{Name: "renamed-fork"},
			store: &testForkStore{
				getUserByUsernameFn: func(_ stdctx.Context, _ string) (*database.User, error) {
					return repoOwner, nil
				},
				getRepositoryByNameFn: func(_ stdctx.Context, _ int64, _ string) (*database.Repository, error) {
					return baseRepo, nil
				},
				forkRepositoryFn: func(_, _ *database.User, _ *database.Repository, name, _ string) (*database.Repository, error) {
					assert.Equal(t, "renamed-fork", name, "opt.Name overrides the default fork name")
					return &database.Repository{ID: 13, Name: "renamed-fork", Owner: requester}, nil
				},
			},
			expStatusCode: http.StatusCreated,
		},
		{
			name:      "repo owner not found returns 404",
			repoOwner: "nobody",
			repoName:  "myrepo",
			opt:       forkRepoRequest{},
			store: &testForkStore{
				getUserByUsernameFn: func(_ stdctx.Context, _ string) (*database.User, error) {
					return nil, notFoundErr{"user does not exist"}
				},
			},
			expStatusCode: http.StatusNotFound,
		},
		{
			name:      "repository not found returns 404",
			repoOwner: "bob",
			repoName:  "nosuchrepo",
			opt:       forkRepoRequest{},
			store: &testForkStore{
				getUserByUsernameFn: func(_ stdctx.Context, _ string) (*database.User, error) {
					return repoOwner, nil
				},
				getRepositoryByNameFn: func(_ stdctx.Context, _ int64, _ string) (*database.Repository, error) {
					return nil, notFoundErr{"repository does not exist"}
				},
			},
			expStatusCode: http.StatusNotFound,
		},
		{
			name:      "organization not found returns 500",
			repoOwner: "bob",
			repoName:  "myrepo",
			opt:       forkRepoRequest{Organization: "noorg"},
			store: &testForkStore{
				getUserByUsernameFn: func(_ stdctx.Context, _ string) (*database.User, error) {
					return repoOwner, nil
				},
				getRepositoryByNameFn: func(_ stdctx.Context, _ int64, _ string) (*database.Repository, error) {
					return baseRepo, nil
				},
				getOrgByNameFn: func(_ string) (*database.User, error) {
					// database.GetOrgByName returns ErrOrgNotExist which does not
					// implement errx.NotFound, so NotFoundOrError returns 500.
					return nil, database.ErrOrgNotExist
				},
			},
			expStatusCode: http.StatusInternalServerError,
		},
		{
			name:      "fork creation failure returns 500",
			repoOwner: "bob",
			repoName:  "myrepo",
			opt:       forkRepoRequest{},
			store: &testForkStore{
				getUserByUsernameFn: func(_ stdctx.Context, _ string) (*database.User, error) {
					return repoOwner, nil
				},
				getRepositoryByNameFn: func(_ stdctx.Context, _ int64, _ string) (*database.Repository, error) {
					return baseRepo, nil
				},
				forkRepositoryFn: func(_, _ *database.User, _ *database.Repository, _, _ string) (*database.Repository, error) {
					return nil, database.ErrReachLimitOfRepo{Limit: 5}
				},
			},
			expStatusCode: http.StatusInternalServerError,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resp := callDoForkRepo(t, requester, test.repoOwner, test.repoName, test.opt, test.store)
			assert.Equal(t, test.expStatusCode, resp.StatusCode)
		})
	}
}
