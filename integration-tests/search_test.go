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
	t.Parallel()

	owner := seedUser(t, CreateUserOptions{})
	other := seedUser(t, CreateUserOptions{})
	public := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
	private := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true, Private: true})

	ownerClient := authenticatedClient(t, owner)
	otherClient := authenticatedClient(t, other)
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
		MatchCase  bool             `json:"matchCase"`
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

	t.Run("repository match case", func(t *testing.T) {
		// A keyword of its own in three cases, so other searches do not find
		// this file.
		cased := UniqueName("Cased")
		repoCodeNewFile(t, ownerClient, public, "case.txt", "1 "+cased+"\n2 "+strings.ToLower(cased)+"\n3 "+strings.ToUpper(cased)+"\n", "Add case")

		resp := repoSearch(t, anonymous, public, "", strings.ToLower(cased))
		assert.False(t, resp.MatchCase)
		require.Equal(t, []string{"case.txt"}, codeSearchPaths(resp.Files))
		assert.Len(t, resp.Files[0].Lines, 3, "case is ignored by default")

		var matched repoSearchResponse
		anonymous.doJSON(http.MethodGet, repoCodeAPI(public)+"/search?matchCase=true&q="+url.QueryEscape(cased), nil, http.StatusOK, &matched)
		assert.True(t, matched.MatchCase)
		require.Equal(t, []string{"case.txt"}, codeSearchPaths(matched.Files))
		// The lines around the match contain the keyword in other cases, and
		// are context.
		assert.Equal(t, []codeSearchLine{
			{Line: 1, Text: "1 " + cased, IsMatch: true},
			{Line: 2, Text: "2 " + strings.ToLower(cased)},
		}, matched.Files[0].Lines)

		// Match case also applies to an exhaustive search.
		anonymous.doJSON(http.MethodGet, repoCodeAPI(public)+"/search?matchCase=true&exhaustive=true&q="+url.QueryEscape(strings.ToUpper(cased)), nil, http.StatusOK, &matched)
		require.Equal(t, []string{"case.txt"}, codeSearchPaths(matched.Files))
		assert.Equal(t, []codeSearchLine{
			{Line: 2, Text: "2 " + strings.ToLower(cased)},
			{Line: 3, Text: "3 " + strings.ToUpper(cased), IsMatch: true},
		}, matched.Files[0].Lines)
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
		MatchCase     bool             `json:"matchCase"`
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
		// Limited to the owner, because a search of every repository on the
		// server under test can run out of time before it reaches these.
		q := "user:" + owner.Name + " " + keyword
		want := []string{publicName + ":docs/notes.md", publicName + ":src/main.go"}
		// 🚨 SECURITY: Only the owner finds the code of the private repository.
		for name, c := range map[string]*client{"anonymous": anonymous, "other": otherClient} {
			resp := globalSearch(t, c, q)
			assert.Equal(t, want, codeSearchPaths(resp.Files), "%s viewer", name)
		}
		resp := globalSearch(t, ownerClient, q)
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

	t.Run("all repositories match case", func(t *testing.T) {
		var resp codeSearchResponse
		// Scoped to the owner, so the search does not read every repository
		// other tests create at the same time.
		scope := "user:" + owner.Name + " "
		anonymous.doJSON(http.MethodGet, "/api/web/search/code?matchCase=true&q="+url.QueryEscape(scope+strings.ToUpper(keyword)), nil, http.StatusOK, &resp)
		assert.True(t, resp.MatchCase)
		assert.Empty(t, resp.Files)

		anonymous.doJSON(http.MethodGet, "/api/web/search/code?matchCase=true&q="+url.QueryEscape(scope+keyword), nil, http.StatusOK, &resp)
		assert.Equal(t, []string{publicName + ":docs/notes.md", publicName + ":src/main.go"}, codeSearchPaths(resp.Files))
	})

	t.Run("all repositories without keyword", func(t *testing.T) {
		resp := globalSearch(t, anonymous, "org:"+owner.Name)
		assert.Empty(t, resp.Keyword)
		assert.NotNil(t, resp.Files)
		assert.Empty(t, resp.Files)
	})
}

func TestIssueSearch(t *testing.T) {
	t.Parallel()

	owner := seedUser(t, CreateUserOptions{})
	other := seedUser(t, CreateUserOptions{})
	public := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
	private := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true, Private: true})

	ownerClient := authenticatedClient(t, owner)
	otherClient := authenticatedClient(t, other)
	anonymous := newClient(t)

	// The keyword is unique to this test, so a search of all repositories
	// only finds the issues below.
	keyword := UniqueName("zebra")
	open := issueCreate(t, ownerClient, public, "Open "+keyword, "")
	closed := issueCreate(t, ownerClient, public, "Closed "+keyword, "")
	status, body := ownerClient.do(http.MethodPut, issueAPIPath(public, "issues", closed, "closed"), map[string]any{})
	require.Equal(t, http.StatusNoContent, status, "response: %s", body)
	// Update times are in seconds, so the closed issue is moved a minute ahead
	// to be the most recently updated one, or the order of the search would
	// not tell open first from newest first.
	err := gormDB.Exec("UPDATE issue SET updated_unix = updated_unix + 60 WHERE id = ?", issueFromDatabase(t, public, closed).ID).Error
	require.NoError(t, err)
	secret := issueCreate(t, ownerClient, private, "Secret "+keyword, "")

	branch := UniqueName("topic-")
	pullCommitToNewBranch(t, ownerClient, public, branch, "topic.md", "# Topic\n")
	pull := pullCreate(t, ownerClient, public, branch, "Pull "+keyword)

	type item struct {
		Repo     string `json:"repo"`
		Index    int64  `json:"index"`
		Title    string `json:"title"`
		IsPull   bool   `json:"isPull"`
		IsClosed bool   `json:"isClosed"`
	}
	search := func(t *testing.T, c *client, q, repo string) []item {
		t.Helper()
		path := "/api/web/search/issues?q=" + url.QueryEscape(q)
		if repo != "" {
			path += "&repo=" + url.QueryEscape(repo)
		}
		var resp struct {
			Items []item `json:"items"`
		}
		c.doJSON(http.MethodGet, path, nil, http.StatusOK, &resp)
		require.NotNil(t, resp.Items)
		return resp.Items
	}
	titles := func(items []item) []string {
		titles := make([]string, 0, len(items))
		for _, it := range items {
			titles = append(titles, it.Title)
		}
		return titles
	}
	publicName := owner.Name + "/" + public.Name
	privateName := owner.Name + "/" + private.Name

	t.Run("by title, open first", func(t *testing.T) {
		items := search(t, anonymous, strings.ToUpper(keyword), "")
		// The closed issue is the most recently updated, and still comes after
		// the open ones.
		require.Len(t, items, 3)
		assert.ElementsMatch(t, []item{
			{Repo: publicName, Index: pull, Title: "Pull " + keyword, IsPull: true},
			{Repo: publicName, Index: open, Title: "Open " + keyword},
		}, items[:2])
		assert.Equal(t, item{Repo: publicName, Index: closed, Title: "Closed " + keyword, IsClosed: true}, items[2])
	})

	t.Run("by number", func(t *testing.T) {
		for _, q := range []string{strconv.FormatInt(closed, 10), "#" + strconv.FormatInt(closed, 10)} {
			items := search(t, anonymous, q, publicName)
			require.NotEmpty(t, items, q)
			assert.Equal(t, closed, items[0].Index, q)
		}
	})

	t.Run("without a keyword", func(t *testing.T) {
		assert.Empty(t, search(t, anonymous, "  ", ""))
	})

	t.Run("private repository", func(t *testing.T) {
		// 🚨 SECURITY: Viewers who cannot read the repository find nothing of
		// it, even when they name it.
		for name, c := range map[string]*client{"anonymous": anonymous, "other": otherClient} {
			assert.NotContains(t, titles(search(t, c, keyword, "")), "Secret "+keyword, "%s viewer", name)
			assert.Empty(t, search(t, c, keyword, privateName), "%s viewer", name)
			assert.Empty(t, search(t, c, strconv.FormatInt(secret, 10), privateName), "%s viewer", name)
		}
		assert.Contains(t, titles(search(t, ownerClient, keyword, "")), "Secret "+keyword)
		assert.Equal(t, []string{"Secret " + keyword}, titles(search(t, ownerClient, keyword, privateName)))
	})

	t.Run("pull requests turned off", func(t *testing.T) {
		status, body := ownerClient.do(http.MethodPut, repoSettingsAPIPath(public, "/settings/advanced"), map[string]any{
			"enableIssues":       true,
			"allowPublicIssues":  true,
			"enablePulls":        false,
			"pullsAllowMerge":    true,
			"pullsSquashMessage": "title_commits",
		})
		require.Equal(t, http.StatusNoContent, status, "response: %s", body)

		assert.Equal(t, []string{"Open " + keyword, "Closed " + keyword}, titles(search(t, anonymous, keyword, publicName)))
		// The number also matches titles with it, so look for the pull request
		// itself.
		for _, it := range search(t, anonymous, strconv.FormatInt(pull, 10), publicName) {
			assert.False(t, it.IsPull, "found %+v", it)
		}
	})
}
