package integrationtests

import (
	"context"
	"encoding/base64"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/git"
)

func TestAPIV1Contents(t *testing.T) {
	t.Parallel()

	owner := seedUser(t, CreateUserOptions{})
	repo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
	api := apiV1Client(t, owner)

	var put struct {
		Content apiV1Content `json:"content"`
		Commit  struct {
			SHA    string `json:"sha"`
			Commit struct {
				Message string `json:"message"`
			} `json:"commit"`
			Author apiV1User `json:"author"`
		} `json:"commit"`
	}
	api.doJSON(http.MethodPut, apiV1RepoPath(repo, "/contents/docs/guide.md"), map[string]any{
		"message": "Add guide",
		"content": base64.StdEncoding.EncodeToString([]byte("Guide")),
	}, http.StatusCreated, &put)
	assert.Equal(t, "file", put.Content.Type)
	assert.Equal(t, "guide.md", put.Content.Name)
	assert.Equal(t, "docs/guide.md", put.Content.Path)
	assert.Equal(t, "Add guide", put.Commit.Commit.Message)
	assert.Equal(t, owner.Name, put.Commit.Author.UserName)

	status, body := api.do(http.MethodPut, apiV1RepoPath(repo, "/contents/.git/config"), map[string]any{
		"message": "Overwrite config",
		"content": base64.StdEncoding.EncodeToString([]byte("x")),
	})
	assert.Equal(t, http.StatusUnprocessableEntity, status, "response: %s", body)

	// The commit is recorded in the news feed like a push.
	actions := gitRepoActions(t, owner.ID, repo.ID)
	require.NotEmpty(t, actions)
	assert.Equal(t, database.ActionCommitRepo, actions[0].OpType)
	assert.Equal(t, "main", actions[0].RefName)
	assert.Contains(t, actions[0].Content, put.Commit.SHA)

	var file apiV1Content
	api.doJSON(http.MethodGet, apiV1RepoPath(repo, "/contents/docs/guide.md"), nil, http.StatusOK, &file)
	assert.Equal(t, "file", file.Type)
	assert.Equal(t, "base64", file.Encoding)
	assert.Equal(t, int64(len("Guide")), file.Size)
	content, err := base64.StdEncoding.DecodeString(file.Content)
	require.NoError(t, err)
	assert.Equal(t, "Guide", string(content))

	type entry struct {
		Type string `json:"type"`
		Path string `json:"path"`
	}
	var root []entry
	api.doJSON(http.MethodGet, apiV1RepoPath(repo, "/contents"), nil, http.StatusOK, &root)
	assert.ElementsMatch(t, []entry{{Type: "file", Path: "README.md"}, {Type: "dir", Path: "docs"}}, root)

	var dir []entry
	api.doJSON(http.MethodGet, apiV1RepoPath(repo, "/contents/docs?ref=main"), nil, http.StatusOK, &dir)
	assert.Equal(t, []entry{{Type: "file", Path: "docs/guide.md"}}, dir)

	apiV1PutFile(t, api, repo, "docs/guide.md", "Updated guide", "")
	file = apiV1Content{}
	api.doJSON(http.MethodGet, apiV1RepoPath(repo, "/contents/docs/guide.md"), nil, http.StatusOK, &file)
	content, err = base64.StdEncoding.DecodeString(file.Content)
	require.NoError(t, err)
	assert.Equal(t, "Updated guide", string(content))

	t.Run("existing branch", func(t *testing.T) {
		firstSHA := apiV1PutFile(t, api, repo, "branch.md", "First", "feature/contents")
		secondSHA := apiV1PutFile(t, api, repo, "branch.md", "Second", "feature/contents")

		var second struct {
			Parents []struct {
				SHA string `json:"sha"`
			} `json:"parents"`
		}
		api.doJSON(http.MethodGet, apiV1RepoPath(repo, "/commits/"+secondSHA), nil, http.StatusOK, &second)
		require.Len(t, second.Parents, 1)
		assert.Equal(t, firstSHA, second.Parents[0].SHA, "second update is not on top of the first")

		var file apiV1Content
		api.doJSON(http.MethodGet, apiV1RepoPath(repo, "/contents/branch.md?ref=feature/contents"), nil, http.StatusOK, &file)
		content, err := base64.StdEncoding.DecodeString(file.Content)
		require.NoError(t, err)
		assert.Equal(t, "Second", string(content))

		status, body := api.do(http.MethodGet, apiV1RepoPath(repo, "/contents/branch.md"), nil)
		assert.Equal(t, http.StatusNotFound, status, "file only on another branch, response: %s", body)
	})

	status, body = api.do(http.MethodGet, apiV1RepoPath(repo, "/contents/missing.md"), nil)
	assert.Equal(t, http.StatusNotFound, status, "response: %s", body)

	status, body = apiV1Client(t, seedUser(t, CreateUserOptions{})).do(http.MethodPut, apiV1RepoPath(repo, "/contents/other.md"), map[string]any{
		"message": "Add other",
		"content": base64.StdEncoding.EncodeToString([]byte("Other")),
	})
	assert.Equal(t, http.StatusForbidden, status, "written by another user, response: %s", body)
}

func TestAPIV1GitObjects(t *testing.T) {
	t.Parallel()

	owner := seedUser(t, CreateUserOptions{})
	repo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
	api := apiV1Client(t, owner)
	commitSHA := apiV1PutFile(t, api, repo, "docs/guide.md", "Guide", "")

	var readme, docs apiV1Content
	var root []apiV1Content
	api.doJSON(http.MethodGet, apiV1RepoPath(repo, "/contents"), nil, http.StatusOK, &root)
	for _, c := range root {
		switch c.Name {
		case "README.md":
			readme = c
		case "docs":
			docs = c
		}
	}
	require.NotEmpty(t, readme.Sha)
	require.NotEmpty(t, docs.Sha)

	type treeEntry struct {
		Path string `json:"path"`
		Mode string `json:"mode"`
		Type string `json:"type"`
		Sha  string `json:"sha"`
	}
	var tree struct {
		Sha  string      `json:"sha"`
		Tree []treeEntry `json:"tree"`
	}
	api.doJSON(http.MethodGet, apiV1RepoPath(repo, "/git/trees/"+commitSHA), nil, http.StatusOK, &tree)
	assert.Equal(t, commitSHA, tree.Sha)
	require.Len(t, tree.Tree, 2)
	entries := make(map[string]treeEntry)
	for _, e := range tree.Tree {
		entries[e.Path] = e
	}
	assert.Equal(t, "blob", entries["README.md"].Type)
	assert.Equal(t, readme.Sha, entries["README.md"].Sha)
	assert.Equal(t, "tree", entries["docs"].Type)
	assert.Equal(t, "040000", entries["docs"].Mode)
	assert.Equal(t, docs.Sha, entries["docs"].Sha)
	t.Run("blob mode", func(t *testing.T) {
		assert.Equal(t, "100644", entries["README.md"].Mode)
	})

	var blob struct {
		Content  string `json:"content"`
		Encoding string `json:"encoding"`
		SHA      string `json:"sha"`
		Size     int64  `json:"size"`
	}
	api.doJSON(http.MethodGet, apiV1RepoPath(repo, "/git/blobs/"+readme.Sha), nil, http.StatusOK, &blob)
	assert.Equal(t, readme.Sha, blob.SHA)
	assert.Equal(t, "base64", blob.Encoding)
	assert.Equal(t, readme.Content, blob.Content)
	assert.Equal(t, readme.Size, blob.Size)

	t.Run("missing blob", func(t *testing.T) {
		status, body := api.do(http.MethodGet, apiV1RepoPath(repo, "/git/blobs/"+strings.Repeat("0", len(readme.Sha))), nil)
		assert.Equal(t, http.StatusNotFound, status, "response: %s", body)
	})

	private := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true, Private: true})
	status, body := apiV1Client(t, seedUser(t, CreateUserOptions{})).do(http.MethodGet, apiV1RepoPath(private, "/git/trees/main"), nil)
	assert.Equal(t, http.StatusNotFound, status, "private repository of another user, response: %s", body)
}

func TestAPIV1RawFile(t *testing.T) {
	t.Parallel()

	owner := seedUser(t, CreateUserOptions{})
	repo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true, Private: true})
	api := apiV1Client(t, owner)
	commitSHA := apiV1PutFile(t, api, repo, "docs/guide.md", "Guide", "feature/raw")

	for _, ref := range []string{"feature/raw", commitSHA} {
		status, header, body := api.doRaw(http.MethodGet, apiV1RepoPath(repo, "/raw/"+ref+"/docs/guide.md"), "", nil)
		require.Equal(t, http.StatusOK, status, "ref %q, response: %s", ref, body)
		assert.Equal(t, "Guide", string(body))
		assert.Equal(t, "text/plain; charset=utf-8", header.Get("Content-Type"))
	}

	status, _, body := api.doRaw(http.MethodGet, apiV1RepoPath(repo, "/raw/main/docs/guide.md"), "", nil)
	assert.Equal(t, http.StatusNotFound, status, "file only on another branch, response: %s", body)
	status, _, body = apiV1Client(t, seedUser(t, CreateUserOptions{})).doRaw(http.MethodGet, apiV1RepoPath(repo, "/raw/feature/raw/docs/guide.md"), "", nil)
	assert.Equal(t, http.StatusNotFound, status, "private repository of another user, response: %s", body)
}

func TestAPIV1Archive(t *testing.T) {
	t.Parallel()

	owner := seedUser(t, CreateUserOptions{})
	repo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
	api := apiV1Client(t, owner)

	status, header, body := api.doRaw(http.MethodGet, apiV1RepoPath(repo, "/archive/main.zip"), "", nil)
	require.Equal(t, http.StatusOK, status, "response: %s", body)
	assert.Equal(t, "application/octet-stream", header.Get("Content-Type"))
	assert.Contains(t, header.Get("Content-Disposition"), ".zip")
	assert.Equal(t, "PK\x03\x04", string(body[:4]), "not a ZIP file")

	status, _, body = api.doRaw(http.MethodGet, apiV1RepoPath(repo, "/archive/main.tar.gz"), "", nil)
	require.Equal(t, http.StatusOK, status, "response: %s", body)
	assert.Equal(t, []byte{0x1f, 0x8b}, body[:2], "not a gzip file")

	status, _, body = api.doRaw(http.MethodGet, apiV1RepoPath(repo, "/archive/missing.zip"), "", nil)
	assert.Equal(t, http.StatusNotFound, status, "response: %s", body)
}

func TestAPIV1Editorconfig(t *testing.T) {
	t.Parallel()

	owner := seedUser(t, CreateUserOptions{})
	repo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
	api := apiV1Client(t, owner)

	status, body := api.do(http.MethodGet, apiV1RepoPath(repo, "/editorconfig/main.go"), nil)
	assert.Equal(t, http.StatusNotFound, status, "no .editorconfig, response: %s", body)

	apiV1PutFile(t, api, repo, ".editorconfig", "root = true\n\n[*.go]\nindent_style = tab\nindent_size = 4\n", "")
	var def map[string]any
	api.doJSON(http.MethodGet, apiV1RepoPath(repo, "/editorconfig/main.go"), nil, http.StatusOK, &def)
	assert.Equal(t, map[string]any{"indent_style": "tab", "indent_size": "4"}, def)
}

func TestAPIV1BranchesAndCommits(t *testing.T) {
	t.Parallel()

	owner := seedUser(t, CreateUserOptions{})
	repo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
	api := apiV1Client(t, owner)
	mainSHA := apiV1PutFile(t, api, repo, "main.txt", "Main", "")
	featureSHA := apiV1PutFile(t, api, repo, "feature.txt", "Feature", "feature/x")

	type branch struct {
		Name   string `json:"name"`
		Commit struct {
			ID      string `json:"id"`
			Message string `json:"message"`
			Author  struct {
				UserName string `json:"username"`
			} `json:"author"`
		} `json:"commit"`
	}
	t.Run("branches", func(t *testing.T) {
		var branches []branch
		api.doJSON(http.MethodGet, apiV1RepoPath(repo, "/branches"), nil, http.StatusOK, &branches)
		got := make(map[string]string)
		for _, b := range branches {
			got[b.Name] = b.Commit.ID
		}
		assert.Equal(t, map[string]string{"main": mainSHA, "feature/x": featureSHA}, got)

		var b branch
		api.doJSON(http.MethodGet, apiV1RepoPath(repo, "/branches/feature/x"), nil, http.StatusOK, &b)
		assert.Equal(t, "feature/x", b.Name)
		assert.Equal(t, featureSHA, b.Commit.ID)
		assert.Equal(t, "Update feature.txt\n", b.Commit.Message)
		assert.Equal(t, owner.Name, b.Commit.Author.UserName)

		status, body := api.do(http.MethodGet, apiV1RepoPath(repo, "/branches/missing"), nil)
		assert.Equal(t, http.StatusNotFound, status, "response: %s", body)
	})

	type commit struct {
		SHA    string `json:"sha"`
		Commit struct {
			Message string `json:"message"`
		} `json:"commit"`
		Author  *apiV1User `json:"author"`
		Parents []struct {
			SHA string `json:"sha"`
		} `json:"parents"`
	}
	t.Run("commits", func(t *testing.T) {
		var commits []commit
		api.doJSON(http.MethodGet, apiV1RepoPath(repo, "/commits"), nil, http.StatusOK, &commits)
		require.Len(t, commits, 2, "commits of the default branch")
		assert.Equal(t, mainSHA, commits[0].SHA)
		assert.Equal(t, "Update main.txt", commits[0].Commit.Message)
		require.NotNil(t, commits[0].Author)
		assert.Equal(t, owner.Name, commits[0].Author.UserName)
		require.Len(t, commits[0].Parents, 1)
		assert.Equal(t, commits[1].SHA, commits[0].Parents[0].SHA)

		var c commit
		api.doJSON(http.MethodGet, apiV1RepoPath(repo, "/commits/"+featureSHA), nil, http.StatusOK, &c)
		assert.Equal(t, featureSHA, c.SHA)
		assert.Equal(t, "Update feature.txt", c.Commit.Message)
		require.Len(t, c.Parents, 1)
		assert.Equal(t, mainSHA, c.Parents[0].SHA)

		status, body := api.do(http.MethodGet, apiV1RepoPath(repo, "/commits/"+UniqueName("missing")), nil)
		assert.Equal(t, http.StatusNotFound, status, "response: %s", body)
	})

	t.Run("reference SHA", func(t *testing.T) {
		withAccept := apiV1Client(t, owner)
		withAccept.header.Set("Accept", "application/vnd.gogs.sha")
		for ref, want := range map[string]string{
			"main":                 mainSHA,
			"feature/x":            featureSHA,
			"refs/heads/feature/x": featureSHA,
		} {
			status, body := withAccept.do(http.MethodGet, apiV1RepoPath(repo, "/commits/"+ref), nil)
			require.Equal(t, http.StatusOK, status, "ref %q, response: %s", ref, body)
			assert.Equal(t, want, string(body), "ref %q", ref)
		}

		status, body := withAccept.do(http.MethodGet, apiV1RepoPath(repo, "/commits/missing/branch"), nil)
		assert.Equal(t, http.StatusNotFound, status, "response: %s", body)

		t.Run("missing full reference", func(t *testing.T) {
			status, body := withAccept.do(http.MethodGet, apiV1RepoPath(repo, "/commits/refs/heads/missing"), nil)
			assert.Equal(t, http.StatusNotFound, status, "response: %s", body)
		})
	})
}

func TestAPIV1Tags(t *testing.T) {
	t.Parallel()

	owner := seedUser(t, CreateUserOptions{})
	repo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
	api := apiV1Client(t, owner)
	mainSHA := apiV1PutFile(t, api, repo, "main.txt", "Main", "")

	gitRepo, err := git.Open(database.RepoPath(repo.Owner, repo.Name))
	require.NoError(t, err)
	err = database.NewRelease(context.WithoutCancel(t.Context()), logger, gitRepo, &database.Release{
		RepoID:      repo.ID,
		PublisherID: owner.ID,
		TagName:     "v1.0.0",
		Target:      "main",
		Title:       "v1.0.0",
	}, nil)
	require.NoError(t, err)

	type tag struct {
		Name   string `json:"name"`
		Commit struct {
			ID string `json:"id"`
		} `json:"commit"`
	}
	var tags []tag
	api.doJSON(http.MethodGet, apiV1RepoPath(repo, "/tags"), nil, http.StatusOK, &tags)
	require.Len(t, tags, 1)
	assert.Equal(t, "v1.0.0", tags[0].Name)
	assert.Equal(t, mainSHA, tags[0].Commit.ID)

	status, body := newClient(t).do(http.MethodGet, apiV1RepoPath(repo, "/tags"), nil)
	assert.Equal(t, http.StatusUnauthorized, status, "response: %s", body)
}
