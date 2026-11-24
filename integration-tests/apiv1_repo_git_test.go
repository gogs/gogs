package integrationtests

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
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

func TestAPIV1EmptyRepository(t *testing.T) {
	t.Parallel()

	owner := seedUser(t, CreateUserOptions{})
	api := apiV1Client(t, owner)

	type commit struct {
		Parents []struct {
			SHA string `json:"sha"`
		} `json:"parents"`
	}
	t.Run("default branch", func(t *testing.T) {
		repo := seedRepository(t, owner.ID, CreateRepositoryOptions{})

		var branches []struct {
			Name string `json:"name"`
		}
		api.doJSON(http.MethodGet, apiV1RepoPath(repo, "/branches"), nil, http.StatusOK, &branches)
		assert.Empty(t, branches)
		status, body := api.do(http.MethodGet, apiV1RepoPath(repo, "/commits"), nil)
		require.Equal(t, http.StatusOK, status, "response: %s", body)
		assert.JSONEq(t, `[]`, string(body))

		firstSHA := apiV1PutFile(t, api, repo, "README.md", "First", "")
		var first commit
		api.doJSON(http.MethodGet, apiV1RepoPath(repo, "/commits/"+firstSHA), nil, http.StatusOK, &first)
		assert.Empty(t, first.Parents)
		assert.ElementsMatch(t, []gitRefAction{
			{OpType: database.ActionCreateBranch, RefName: "main"},
			{OpType: database.ActionCommitRepo, RefName: "main"},
		}, gitNewRefActions(t, owner.ID, repo.ID, 1))

		var got apiV1Repo
		api.doJSON(http.MethodGet, apiV1RepoPath(repo, ""), nil, http.StatusOK, &got)
		assert.False(t, got.Empty)
		assert.Equal(t, "main", got.DefaultBranch)

		branches = nil
		api.doJSON(http.MethodGet, apiV1RepoPath(repo, "/branches"), nil, http.StatusOK, &branches)
		require.Len(t, branches, 1)
		assert.Equal(t, "main", branches[0].Name)

		secondSHA := apiV1PutFile(t, api, repo, "README.md", "Second", "")
		var second commit
		api.doJSON(http.MethodGet, apiV1RepoPath(repo, "/commits/"+secondSHA), nil, http.StatusOK, &second)
		require.Len(t, second.Parents, 1)
		assert.Equal(t, firstSHA, second.Parents[0].SHA, "second commit is not on top of the first")
	})

	t.Run("other branch", func(t *testing.T) {
		repo := seedRepository(t, owner.ID, CreateRepositoryOptions{})

		// Like a first push, the first commit to another branch makes it the
		// default branch.
		apiV1PutFile(t, api, repo, "README.md", "Dev", "dev")
		assert.ElementsMatch(t, []gitRefAction{
			{OpType: database.ActionCreateBranch, RefName: "dev"},
			{OpType: database.ActionCommitRepo, RefName: "dev"},
		}, gitNewRefActions(t, owner.ID, repo.ID, 1))

		var got apiV1Repo
		api.doJSON(http.MethodGet, apiV1RepoPath(repo, ""), nil, http.StatusOK, &got)
		assert.False(t, got.Empty)
		assert.Equal(t, "dev", got.DefaultBranch)
		assert.Equal(t, "dev", repoSettingsDefaultBranch(t, repo))

		var file apiV1Content
		api.doJSON(http.MethodGet, apiV1RepoPath(repo, "/contents/README.md"), nil, http.StatusOK, &file)
		content, err := base64.StdEncoding.DecodeString(file.Content)
		require.NoError(t, err)
		assert.Equal(t, "Dev", string(content))
	})
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

	// Executables, symlinks, and submodules are blobs or commits with modes
	// other than a regular file.
	dir := gitClone(t, gitRemoteURL(t, repo, owner.Name, owner.Password))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "run.sh"), []byte("#!/bin/sh\n"), 0o600))
	gitRun(t, dir, "add", "run.sh")
	gitRun(t, dir, "update-index", "--chmod=+x", "run.sh")
	require.NoError(t, os.Symlink("README.md", filepath.Join(dir, "link")))
	gitRun(t, dir, "add", "link")
	gitRun(t, dir, "update-index", "--add", "--cacheinfo", "160000,"+commitSHA+",sub")
	gitRun(t, dir, "commit", "-m", "Add special entries")
	gitRun(t, dir, "push", "origin", "main")
	headSHA := gitRun(t, dir, "rev-parse", "HEAD")

	type treeEntry struct {
		Path string `json:"path"`
		Mode string `json:"mode"`
		Type string `json:"type"`
		Sha  string `json:"sha"`
		URL  string `json:"url"`
	}
	var tree struct {
		Sha  string      `json:"sha"`
		Tree []treeEntry `json:"tree"`
	}
	api.doJSON(http.MethodGet, apiV1RepoPath(repo, "/git/trees/"+headSHA), nil, http.StatusOK, &tree)
	assert.Equal(t, headSHA, tree.Sha)
	gitURL := serverURL + apiV1RepoPath(repo, "/git")
	entry := func(path, mode, typ, sha string) treeEntry {
		kind := "/blobs/"
		if typ != "blob" {
			kind = "/trees/"
		}
		return treeEntry{Path: path, Mode: mode, Type: typ, Sha: sha, URL: gitURL + kind + sha}
	}
	runSHA := gitRun(t, dir, "rev-parse", "HEAD:run.sh")
	linkSHA := gitRun(t, dir, "rev-parse", "HEAD:link")
	assert.ElementsMatch(t, []treeEntry{
		entry("README.md", "100644", "blob", readme.Sha),
		entry("run.sh", "100755", "blob", runSHA),
		entry("link", "120000", "blob", linkSHA),
		entry("docs", "040000", "tree", docs.Sha),
		entry("sub", "160000", "commit", commitSHA),
	}, tree.Tree)

	t.Run("entry URLs", func(t *testing.T) {
		for _, e := range tree.Tree {
			// The commit of a submodule is in another repository.
			if e.Type == "commit" {
				continue
			}
			var got struct {
				Sha string `json:"sha"`
			}
			api.doJSON(http.MethodGet, strings.TrimPrefix(e.URL, serverURL), nil, http.StatusOK, &got)
			assert.Equal(t, e.Sha, got.Sha, "entry %q", e.Path)
		}
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

func TestAPIV1CommitsPages(t *testing.T) {
	t.Parallel()

	owner := seedUser(t, CreateUserOptions{})
	repo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
	api := apiV1Client(t, owner)

	// 55 commits on the default branch, more than [api] MAX_RESPONSE_ITEMS = 50.
	repoPath := database.RepoPath(repo.Owner, repo.Name)
	tree := repoSettingsGit(t, repoPath, "rev-parse", "main^{tree}")
	head := repoSettingsGit(t, repoPath, "rev-parse", "main")
	for i := range 54 {
		head = repoSettingsGit(t, repoPath, "commit-tree", tree, "-p", head, "-m", "Commit "+strconv.Itoa(i))
	}
	repoSettingsGit(t, repoPath, "update-ref", "refs/heads/main", head)
	all := strings.Fields(repoSettingsGit(t, repoPath, "rev-list", "main"))
	require.Len(t, all, 55)
	repoSettingsGit(t, repoPath, "branch", "old", all[50])
	repoSettingsGit(t, repoPath, "tag", "-a", "annotated", "-m", "Annotated", all[52])

	list := func(path string) (shas []string, links map[string]string) {
		t.Helper()
		status, header, body := api.doRaw(http.MethodGet, path, "", nil)
		require.Equal(t, http.StatusOK, status, "%s, response: %s", path, body)
		var commits []struct {
			SHA string `json:"sha"`
		}
		require.NoError(t, json.Unmarshal(body, &commits), "%s, response: %s", path, body)
		shas = []string{}
		for _, c := range commits {
			shas = append(shas, c.SHA)
		}
		return shas, apiV1Links(t, header)
	}
	commits := func(query string) string {
		return apiV1RepoPath(repo, "/commits?"+query)
	}

	t.Run("default branch", func(t *testing.T) {
		got, links := list(apiV1RepoPath(repo, "/commits"))
		assert.Equal(t, all[:30], got, "default page size")
		assert.Equal(t, map[string]string{"next": commits("page=2"), "last": commits("page=2")}, links)

		got, _ = list(commits("page=2"))
		assert.Equal(t, all[30:], got)

		got, links = list(commits("pageSize=1000"))
		assert.Equal(t, all[:50], got, "page size above MAX_RESPONSE_ITEMS")
		assert.Equal(t, commits("page=2&pageSize=1000"), links["next"])

		got, links = list(commits("limit=20"))
		assert.Equal(t, all[:20], got, "limit as the page size")
		assert.Equal(t, commits("limit=20&page=2"), links["next"])
	})

	// Follows the "next" links like an API client. Starting from another branch
	// with another page size, a link that drops either leads to other commits.
	t.Run("next links", func(t *testing.T) {
		got := []string{}
		pages := 0
		for next := commits("sha=old&pageSize=2"); next != ""; pages++ {
			require.Less(t, pages, 3, "more pages than the commits fill")
			shas, links := list(next)
			got = append(got, shas...)
			next = links["next"]
		}
		assert.Equal(t, 3, pages)
		assert.Equal(t, all[50:], got)
	})

	t.Run("starting points", func(t *testing.T) {
		for sha, want := range map[string][]string{
			"old":       all[50:],
			"annotated": all[52:],
			all[53]:     all[53:],
			"main~54":   all[54:],
		} {
			got, _ := list(commits("sha=" + sha))
			assert.Equal(t, want, got, "sha %q", sha)
		}

		for _, sha := range []string{"missing", "--all", "main^{tree}", "old..main"} {
			status, body := api.do(http.MethodGet, commits("sha="+url.QueryEscape(sha)), nil)
			assert.Equal(t, http.StatusNotFound, status, "sha %q, response: %s", sha, body)
		}
	})

	t.Run("past the last page", func(t *testing.T) {
		// A page whose offset overflows an int is past the last page too.
		for _, n := range []string{"10", "99999999999999999999"} {
			got, links := list(commits("pageSize=20&page=" + n))
			assert.Empty(t, got, "page %s", n)
			assert.Equal(t, map[string]string{
				"first": commits("page=1&pageSize=20"),
				"prev":  commits("page=3&pageSize=20"),
				"last":  commits("page=3&pageSize=20"),
			}, links, "page %s", n)
		}
	})
}

func TestAPIV1BranchesPages(t *testing.T) {
	t.Parallel()

	owner := seedUser(t, CreateUserOptions{})
	repo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
	api := apiV1Client(t, owner)
	repoPath := database.RepoPath(repo.Owner, repo.Name)
	for i := range 5 {
		repoSettingsGit(t, repoPath, "branch", "b"+strconv.Itoa(i), "main")
	}

	list := func(path string) (names []string, links map[string]string) {
		t.Helper()
		status, header, body := api.doRaw(http.MethodGet, path, "", nil)
		require.Equal(t, http.StatusOK, status, "%s, response: %s", path, body)
		var branches []struct {
			Name string `json:"name"`
		}
		require.NoError(t, json.Unmarshal(body, &branches), "%s, response: %s", path, body)
		names = []string{}
		for _, b := range branches {
			names = append(names, b.Name)
		}
		return names, apiV1Links(t, header)
	}
	branches := func(query string) string {
		return apiV1RepoPath(repo, "/branches?"+query)
	}

	// Without "page" or "pageSize", all branches in one response, as before.
	all, links := list(apiV1RepoPath(repo, "/branches"))
	assert.ElementsMatch(t, []string{"main", "b0", "b1", "b2", "b3", "b4"}, all)
	assert.Empty(t, links)

	t.Run("next links", func(t *testing.T) {
		got := []string{}
		pages := 0
		for next := branches("pageSize=4"); next != ""; pages++ {
			require.Less(t, pages, 2, "more pages than the branches fill")
			names, links := list(next)
			require.LessOrEqual(t, len(names), 4)
			got = append(got, names...)
			next = links["next"]
		}
		assert.Equal(t, 2, pages)
		assert.Equal(t, all, got)
	})

	t.Run("limit", func(t *testing.T) {
		got, links := list(branches("limit=4"))
		assert.Equal(t, all[:4], got)
		assert.Equal(t, branches("limit=4&page=2"), links["next"])
	})

	t.Run("past the last page", func(t *testing.T) {
		// A page whose offset overflows an int is past the last page too.
		for _, n := range []string{"10", "99999999999999999999"} {
			got, links := list(branches("pageSize=4&page=" + n))
			assert.Empty(t, got, "page %s", n)
			assert.Equal(t, map[string]string{
				"first": branches("page=1&pageSize=4"),
				"prev":  branches("page=2&pageSize=4"),
				"last":  branches("page=2&pageSize=4"),
			}, links, "page %s", n)
		}
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
	err = database.Handle.Releases().Create(t.Context(), logger, gitRepo, &database.Release{
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

func TestAPIV1CreateBranchesAndTags(t *testing.T) {
	t.Parallel()

	owner := seedUser(t, CreateUserOptions{})
	repo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
	api := apiV1Client(t, owner)
	webhookURL, deliveries := repoSettingsWebhookReceiver(t)
	repoSettingsCreateWebhookFor(t, authenticatedClient(t, owner), repo, webhookURL, true, map[string]bool{"push": true, "create": true})

	mainSHA := apiV1PutFile(t, api, repo, "main.txt", "Main", "")
	// Each update lists its commit, so the next event comes from the endpoints.
	assert.Equal(t, "push", repoSettingsNextEvent(t, deliveries).Event)
	featureSHA := apiV1PutFile(t, api, repo, "feature.txt", "Feature", "feature/base")
	for range 2 {
		repoSettingsNextEvent(t, deliveries)
	}
	repoPath := database.RepoPath(repo.Owner, repo.Name)
	repoSettingsGit(t, repoPath, "tag", "-a", "annotated", "-m", "Annotated", featureSHA)

	type ref struct {
		Name   string `json:"name"`
		Commit struct {
			ID string `json:"id"`
		} `json:"commit"`
	}
	create := func(kind, name, target string) ref {
		t.Helper()
		var got ref
		api.doJSON(http.MethodPost, apiV1RepoPath(repo, "/"+kind), map[string]any{"name": name, "target": target}, http.StatusCreated, &got)
		return got
	}

	t.Run("branches", func(t *testing.T) {
		before := len(gitRepoActions(t, owner.ID, repo.ID))
		for name, c := range map[string]struct{ target, want string }{
			"from-default":       {"", mainSHA},
			"from-sha":           {featureSHA, featureSHA},
			"nested/from-branch": {"feature/base", featureSHA},
		} {
			got := create("branches", name, c.target)
			assert.Equal(t, name, got.Name)
			assert.Equal(t, c.want, got.Commit.ID, "branch %q", name)
			assert.Equal(t, c.want, repoSettingsGit(t, repoPath, "rev-parse", "refs/heads/"+name), "branch %q on disk", name)

			// The branch is pushed, so it fires webhooks like a "git push".
			assert.Equal(t, repoSettingsEvent{Event: "create", Ref: name, RefType: "branch"}, repoSettingsNextEvent(t, deliveries))
			push := repoSettingsNextEvent(t, deliveries)
			assert.Equal(t, "push", push.Event)
			assert.Equal(t, "refs/heads/"+name, push.Ref)
			assert.Equal(t, c.want, push.After)
		}

		var b ref
		api.doJSON(http.MethodGet, apiV1RepoPath(repo, "/branches/nested/from-branch"), nil, http.StatusOK, &b)
		assert.Equal(t, featureSHA, b.Commit.ID)

		var want []gitRefAction
		for _, name := range []string{"from-default", "from-sha", "nested/from-branch"} {
			want = append(want,
				gitRefAction{OpType: database.ActionCreateBranch, RefName: name},
				gitRefAction{OpType: database.ActionCommitRepo, RefName: name},
			)
		}
		assert.ElementsMatch(t, want, gitNewRefActions(t, owner.ID, repo.ID, before))
	})

	t.Run("tags", func(t *testing.T) {
		before := len(gitRepoActions(t, owner.ID, repo.ID))
		for name, c := range map[string]struct{ target, want string }{
			"v1":           {"", mainSHA},
			"from-tag":     {"annotated", featureSHA},
			"feature/base": {mainSHA, mainSHA},
		} {
			got := create("tags", name, c.target)
			assert.Equal(t, name, got.Name)
			assert.Equal(t, c.want, got.Commit.ID, "tag %q", name)
			// Tags are lightweight, on the commit even when the target is an
			// annotated tag.
			assert.Equal(t, "commit", repoSettingsGit(t, repoPath, "cat-file", "-t", "refs/tags/"+name), "tag %q", name)
			assert.Equal(t, c.want, repoSettingsGit(t, repoPath, "rev-parse", "refs/tags/"+name), "tag %q on disk", name)
			assert.Equal(t, repoSettingsEvent{Event: "create", Ref: name, RefType: "tag"}, repoSettingsNextEvent(t, deliveries))
		}

		var tags []ref
		api.doJSON(http.MethodGet, apiV1RepoPath(repo, "/tags"), nil, http.StatusOK, &tags)
		got := make(map[string]string)
		for _, tag := range tags {
			got[tag.Name] = tag.Commit.ID
		}
		assert.Equal(t, mainSHA, got["v1"])
		assert.Equal(t, featureSHA, got["from-tag"])
		assert.Equal(t, mainSHA, got["feature/base"])

		assert.ElementsMatch(t, []gitRefAction{
			{OpType: database.ActionPushTag, RefName: "v1"},
			{OpType: database.ActionPushTag, RefName: "from-tag"},
			{OpType: database.ActionPushTag, RefName: "feature/base"},
		}, gitNewRefActions(t, owner.ID, repo.ID, before))
	})

	t.Run("invalid requests", func(t *testing.T) {
		pwned := filepath.Join(t.TempDir(), "pwned")
		for _, c := range []struct {
			kind, name, target string
		}{
			{"branches", "from-default", ""},
			{"tags", "v1", ""},
			// Git cannot store these next to "feature/base".
			{"branches", "feature", ""},
			{"branches", "feature/base/x", ""},
			{"tags", "feature", ""},
			{"branches", "-x", ""},
			{"branches", "a..b", ""},
			{"branches", "HEAD", ""},
			{"branches", "x.lock", ""},
			{"branches", "@{-1}", ""},
			{"tags", "-x", ""},
			{"tags", "HEAD", ""},
			{"tags", "v1/", ""},
			{"tags", "a b", ""},
			{"branches", "missing-target", "nope"},
			{"branches", "range-target", "main..feature/base"},
			{"tags", "missing-target", "--output=" + pwned},
			{"branches", "missing-sha", strings.Repeat("1", len(mainSHA))},
		} {
			status, body := api.do(http.MethodPost, apiV1RepoPath(repo, "/"+c.kind), map[string]any{"name": c.name, "target": c.target})
			assert.Equal(t, http.StatusUnprocessableEntity, status, "%s %q at %q, response: %s", c.kind, c.name, c.target, body)
		}
		assert.NoFileExists(t, pwned)
		assert.Empty(t, repoSettingsGit(t, repoPath, "for-each-ref", "refs/heads/missing-target", "refs/heads/missing-sha", "refs/tags/missing-target"))

		status, body := api.do(http.MethodPost, apiV1RepoPath(repo, "/branches"), map[string]any{})
		assert.Equal(t, http.StatusUnprocessableEntity, status, "no name, response: %s", body)
	})

	t.Run("ambiguous target", func(t *testing.T) {
		// The tag "feature/base" points at main, the branch at feature.
		got := create("branches", "from-ambiguous", "feature/base")
		assert.Equal(t, featureSHA, got.Commit.ID, "target resolved to the tag")
	})

	t.Run("empty repository", func(t *testing.T) {
		empty := seedRepository(t, owner.ID, CreateRepositoryOptions{})
		status, body := api.do(http.MethodPost, apiV1RepoPath(empty, "/branches"), map[string]any{"name": "feature"})
		assert.Equal(t, http.StatusUnprocessableEntity, status, "response: %s", body)
	})

	t.Run("mirror", func(t *testing.T) {
		u := seedUser(t, CreateUserOptions{})
		mirror, _ := repoSettingsMigrate(t, authenticatedClient(t, u), u, repo, true)
		for _, kind := range []string{"branches", "tags"} {
			status, body := apiV1Client(t, u).do(http.MethodPost, apiV1RepoPath(mirror, "/"+kind), map[string]any{"name": "new"})
			assert.Equal(t, http.StatusForbidden, status, "%s, response: %s", kind, body)
		}
	})

	t.Run("collaborator", func(t *testing.T) {
		collaborator := seedUser(t, CreateUserOptions{})
		collaboratorAPI := apiV1Client(t, collaborator)
		path := apiV1RepoPath(repo, "/collaborators/"+collaborator.Name)
		status, body := api.do(http.MethodPut, path, map[string]any{"permission": "read"})
		require.Equal(t, http.StatusNoContent, status, "response: %s", body)
		for _, kind := range []string{"branches", "tags"} {
			status, body = collaboratorAPI.do(http.MethodPost, apiV1RepoPath(repo, "/"+kind), map[string]any{"name": "by-reader"})
			assert.Equal(t, http.StatusForbidden, status, "%s with read access, response: %s", kind, body)
		}

		status, body = api.do(http.MethodPut, path, map[string]any{"permission": "write"})
		require.Equal(t, http.StatusNoContent, status, "response: %s", body)
		for _, kind := range []string{"branches", "tags"} {
			var got ref
			collaboratorAPI.doJSON(http.MethodPost, apiV1RepoPath(repo, "/"+kind), map[string]any{"name": "by-writer"}, http.StatusCreated, &got)
			assert.Equal(t, mainSHA, got.Commit.ID, kind)
		}
	})
}
