package integrationtests

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type codeSearchLine struct {
	Line    int    `json:"line"`
	Text    string `json:"text"`
	IsMatch bool   `json:"isMatch"`
}

type codeSearchFile struct {
	Repo  string           `json:"repo"`
	Ref   string           `json:"ref"`
	Path  string           `json:"path"`
	Lines []codeSearchLine `json:"lines"`
}

func codeSearchPaths(files []codeSearchFile) []string {
	paths := make([]string, 0, len(files))
	for _, f := range files {
		if f.Repo != "" {
			paths = append(paths, f.Repo+":"+f.Path)
		} else {
			paths = append(paths, f.Path)
		}
	}
	return paths
}

func TestCodeSearch(t *testing.T) {
	owner := seedUser(t, CreateUserOptions{})
	other := seedUser(t, CreateUserOptions{})
	public := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
	private := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true, Private: true})

	ownerClient := newClient(t)
	ownerClient.signIn(owner)
	otherClient := newClient(t)
	otherClient.signIn(other)
	anonymous := newClient(t)

	// The keyword is unique to this test, so a search of all repositories
	// only finds the files below.
	keyword := UniqueName("needle")
	first := repoCodeGit(t, public, "rev-parse", "refs/heads/main")
	repoCodeNewFile(t, ownerClient, public, "src/main.go", "package main\n\nfunc find() string {\n\treturn \""+keyword+"\"\n}\n", "Add main")
	repoCodeNewFile(t, ownerClient, public, "docs/notes.md", "# Notes\n\nSearch for "+keyword+".\n", "Add notes")
	repoCodeNewFile(t, ownerClient, private, "secret.go", "package secret\n\nconst token = \""+keyword+"\"\n", "Add secret")

	type repoSearchResponse struct {
		Ref        string           `json:"ref"`
		RefType    string           `json:"refType"`
		Query      string           `json:"query"`
		Keyword    string           `json:"keyword"`
		Exhaustive bool             `json:"exhaustive"`
		Files      []codeSearchFile `json:"files"`
		Branches   []string         `json:"branches"`
	}
	repoSearch := func(t *testing.T, c *client, repo *Repository, ref, q string) repoSearchResponse {
		t.Helper()
		path := repoCodeAPI(repo) + "/search"
		if ref != "" {
			path += "/" + ref
		}
		var resp repoSearchResponse
		c.doJSON(http.MethodGet, path+"?q="+url.QueryEscape(q), nil, http.StatusOK, &resp)
		return resp
	}

	t.Run("repository", func(t *testing.T) {
		resp := repoSearch(t, anonymous, public, "", keyword)
		assert.Equal(t, "main", resp.Ref)
		assert.Equal(t, "branch", resp.RefType)
		assert.Equal(t, keyword, resp.Keyword)
		assert.Equal(t, []string{"main"}, resp.Branches)
		require.Equal(t, []string{"docs/notes.md", "src/main.go"}, codeSearchPaths(resp.Files))
		// One line of context on each side of the match.
		assert.Equal(t, []codeSearchLine{
			{Line: 3, Text: "func find() string {"},
			{Line: 4, Text: "\treturn \"" + keyword + "\"", IsMatch: true},
			{Line: 5, Text: "}"},
		}, resp.Files[1].Lines)
		assert.Empty(t, resp.Files[1].Repo)

		// Matching ignores case.
		upper := repoSearch(t, anonymous, public, "main", "NEEDLE")
		assert.Contains(t, codeSearchPaths(upper.Files), "src/main.go")
	})

	t.Run("repository path qualifier", func(t *testing.T) {
		resp := repoSearch(t, anonymous, public, "", "path:src "+keyword)
		assert.Equal(t, "path:src "+keyword, resp.Query)
		assert.Equal(t, keyword, resp.Keyword)
		assert.Equal(t, []string{"src/main.go"}, codeSearchPaths(resp.Files))

		resp = repoSearch(t, anonymous, public, "", "path:*.md "+keyword)
		assert.Equal(t, []string{"docs/notes.md"}, codeSearchPaths(resp.Files))
	})

	t.Run("repository at a commit", func(t *testing.T) {
		// The first commit has neither file yet.
		resp := repoSearch(t, anonymous, public, first, keyword)
		assert.Equal(t, "commit", resp.RefType)
		assert.Empty(t, resp.Files)

		status, body := anonymous.do(http.MethodGet, repoCodeAPI(public)+"/search/no-such-ref?q="+keyword, nil)
		assert.Equal(t, http.StatusNotFound, status, "response: %s", body)
	})

	t.Run("repository exhaustive", func(t *testing.T) {
		// A keyword of its own, so other searches do not find this file.
		many := UniqueName("many")
		repoCodeNewFile(t, ownerClient, public, "many.txt", strings.Repeat(many+"\n", 15), "Add many")
		matches := func(resp repoSearchResponse) int {
			n := 0
			for _, f := range resp.Files {
				for _, l := range f.Lines {
					if l.IsMatch {
						n++
					}
				}
			}
			return n
		}

		resp := repoSearch(t, anonymous, public, "", many)
		assert.False(t, resp.Exhaustive)
		assert.Equal(t, 10, matches(resp), "a search lists at most 10 lines of a file")

		var all repoSearchResponse
		anonymous.doJSON(http.MethodGet, repoCodeAPI(public)+"/search?exhaustive=true&q="+url.QueryEscape(many), nil, http.StatusOK, &all)
		assert.True(t, all.Exhaustive)
		assert.Equal(t, 15, matches(all))
	})

	t.Run("repository without keyword", func(t *testing.T) {
		resp := repoSearch(t, anonymous, public, "", "path:src")
		assert.Empty(t, resp.Keyword)
		assert.NotNil(t, resp.Files)
		assert.Empty(t, resp.Files)
	})

	t.Run("private repository", func(t *testing.T) {
		// 🚨 SECURITY: Viewers who cannot read the repository get nothing.
		for name, c := range map[string]*client{"anonymous": anonymous, "other": otherClient} {
			status, body := c.do(http.MethodGet, repoCodeAPI(private)+"/search?q="+keyword, nil)
			assert.Equal(t, http.StatusNotFound, status, "%s viewer, response: %s", name, body)
			assert.NotContains(t, string(body), keyword, "%s viewer", name)
		}
		resp := repoSearch(t, ownerClient, private, "", keyword)
		assert.Equal(t, []string{"secret.go"}, codeSearchPaths(resp.Files))
	})

	type codeSearchResponse struct {
		Query         string           `json:"query"`
		Keyword       string           `json:"keyword"`
		Files         []codeSearchFile `json:"files"`
		ReposSearched int              `json:"reposSearched"`
	}
	globalSearch := func(t *testing.T, c *client, q string) codeSearchResponse {
		t.Helper()
		var resp codeSearchResponse
		c.doJSON(http.MethodGet, "/api/web/search/code?q="+url.QueryEscape(q), nil, http.StatusOK, &resp)
		return resp
	}
	publicName := owner.Name + "/" + public.Name
	privateName := owner.Name + "/" + private.Name

	t.Run("all repositories", func(t *testing.T) {
		want := []string{publicName + ":docs/notes.md", publicName + ":src/main.go"}
		// 🚨 SECURITY: Only the owner finds the code of the private repository.
		for name, c := range map[string]*client{"anonymous": anonymous, "other": otherClient} {
			resp := globalSearch(t, c, keyword)
			assert.Equal(t, want, codeSearchPaths(resp.Files), "%s viewer", name)
		}
		resp := globalSearch(t, ownerClient, keyword)
		assert.ElementsMatch(t, append(want, privateName+":secret.go"), codeSearchPaths(resp.Files))
		for _, f := range resp.Files {
			assert.Equal(t, "main", f.Ref)
		}
	})

	t.Run("all repositories with qualifiers", func(t *testing.T) {
		resp := globalSearch(t, otherClient, "user:"+owner.Name+" path:src "+keyword)
		assert.Equal(t, keyword, resp.Keyword)
		assert.Equal(t, []string{publicName + ":src/main.go"}, codeSearchPaths(resp.Files))

		resp = globalSearch(t, ownerClient, "repo:"+privateName+" "+keyword)
		assert.Equal(t, []string{privateName + ":secret.go"}, codeSearchPaths(resp.Files))
		assert.Equal(t, 1, resp.ReposSearched)

		// 🚨 SECURITY: Naming a private repository does not reveal its code.
		resp = globalSearch(t, otherClient, "repo:"+privateName+" "+keyword)
		assert.Empty(t, resp.Files)
		assert.Zero(t, resp.ReposSearched)

		resp = globalSearch(t, otherClient, "user:"+UniqueName("nobody-")+" "+keyword)
		assert.Empty(t, resp.Files)
	})

	t.Run("all repositories without keyword", func(t *testing.T) {
		resp := globalSearch(t, anonymous, "org:"+owner.Name)
		assert.Empty(t, resp.Keyword)
		assert.NotNil(t, resp.Files)
		assert.Empty(t, resp.Files)
	})
}
