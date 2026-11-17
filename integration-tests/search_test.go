package integrationtests

import (
	"net/http"
	"net/url"
	"strconv"
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
		Truncated  bool             `json:"truncated"`
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
		exhaustiveSearch := func(t *testing.T, c *client, q string) repoSearchResponse {
			t.Helper()
			var resp repoSearchResponse
			c.doJSON(http.MethodGet, repoCodeAPI(public)+"/search?exhaustive=true&q="+url.QueryEscape(q), nil, http.StatusOK, &resp)
			return resp
		}
		matches := func(f codeSearchFile) int {
			n := 0
			for _, l := range f.Lines {
				if l.IsMatch {
					n++
				}
			}
			return n
		}

		// Keywords of their own, so other searches do not find these files. Each
		// match is followed by a line without it, which is its context.
		many := UniqueName("many")
		var content strings.Builder
		for i := 1; i <= 15; i++ {
			content.WriteString(many + " " + strconv.Itoa(i) + "\nfiller\n")
		}
		repoCodeNewFile(t, ownerClient, public, "docs/many.txt", content.String(), "Add many")
		repoCodeNewFile(t, ownerClient, public, "src/many.txt", many+"\n", "Add one more")

		t.Run("lists at most 10 lines of a file by default", func(t *testing.T) {
			for _, q := range []string{"?q=", "?exhaustive=false&q="} {
				var resp repoSearchResponse
				anonymous.doJSON(http.MethodGet, repoCodeAPI(public)+"/search"+q+url.QueryEscape(many), nil, http.StatusOK, &resp)
				assert.False(t, resp.Exhaustive, q)
				require.Equal(t, []string{"docs/many.txt", "src/many.txt"}, codeSearchPaths(resp.Files), q)
				assert.Equal(t, 10, matches(resp.Files[0]), q)
			}
		})

		t.Run("lists every line of a file with context", func(t *testing.T) {
			resp := exhaustiveSearch(t, anonymous, many)
			assert.True(t, resp.Exhaustive)
			assert.False(t, resp.Truncated)
			require.Equal(t, []string{"docs/many.txt", "src/many.txt"}, codeSearchPaths(resp.Files))
			lines := resp.Files[0].Lines
			require.Len(t, lines, 30)
			for i, l := range lines {
				assert.Equal(t, i+1, l.Line)
				if i%2 == 0 {
					assert.Equal(t, codeSearchLine{Line: i + 1, Text: many + " " + strconv.Itoa(i/2+1), IsMatch: true}, l)
				} else {
					assert.Equal(t, codeSearchLine{Line: i + 1, Text: "filler"}, l)
				}
			}
		})

		t.Run("keeps the path qualifier", func(t *testing.T) {
			resp := exhaustiveSearch(t, anonymous, "path:docs "+many)
			assert.Equal(t, many, resp.Keyword)
			assert.Equal(t, []string{"docs/many.txt"}, codeSearchPaths(resp.Files))
		})

		t.Run("lists at most 5,000 lines", func(t *testing.T) {
			huge := UniqueName("huge")
			repoCodeNewFile(t, ownerClient, public, "huge.txt", strings.Repeat(huge+"\n", 5001), "Add huge")

			resp := exhaustiveSearch(t, anonymous, huge)
			assert.True(t, resp.Truncated)
			require.Equal(t, []string{"huge.txt"}, codeSearchPaths(resp.Files))
			assert.Equal(t, 5000, matches(resp.Files[0]))
		})

		t.Run("private repository", func(t *testing.T) {
			// 🚨 SECURITY: Higher limits do not skip the read check.
			for name, c := range map[string]*client{"anonymous": anonymous, "other": otherClient} {
				status, body := c.do(http.MethodGet, repoCodeAPI(private)+"/search?exhaustive=true&q="+keyword, nil)
				assert.Equal(t, http.StatusNotFound, status, "%s viewer, response: %s", name, body)
				assert.NotContains(t, string(body), keyword, "%s viewer", name)
			}
			var resp repoSearchResponse
			ownerClient.doJSON(http.MethodGet, repoCodeAPI(private)+"/search?exhaustive=true&q="+keyword, nil, http.StatusOK, &resp)
			assert.Equal(t, []string{"secret.go"}, codeSearchPaths(resp.Files))
		})
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
