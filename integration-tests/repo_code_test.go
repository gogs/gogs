package integrationtests

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/html"
	"gorm.io/gorm"

	"gogs.io/gogs/internal/database"
)

func repoCodeAPI(repo *Repository) string {
	return "/api/web/" + repo.Owner + "/" + repo.Name
}

func repoCodePage(repo *Repository) string {
	return "/" + repo.Owner + "/" + repo.Name
}

// repoCodeGit runs Git in the repository on disk and returns its output
// without the trailing newline, so tests check what the server did against
// Git itself.
func repoCodeGit(t *testing.T, repo *Repository, args ...string) string {
	t.Helper()

	cmd := exec.Command("git", append([]string{"-C", database.RepoPath(repo.Owner, repo.Name)}, args...)...)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v: %s", args, out)
	return strings.TrimSuffix(string(out), "\n")
}

// repoCodeNewFile commits a new file to the main branch through the editor
// and returns the commit the branch is at afterwards.
func repoCodeNewFile(t *testing.T, c *client, repo *Repository, path, content, summary string) string {
	t.Helper()

	status, body := c.do(http.MethodPost, repoCodeAPI(repo)+"/editor/files", map[string]any{
		"branch":  "main",
		"path":    path,
		"content": content,
		"summary": summary,
	})
	require.Equal(t, http.StatusOK, status, "response: %s", body)
	return repoCodeGit(t, repo, "rev-parse", "refs/heads/main")
}

// repoCodeMultipart returns a multipart form with the file as the "file"
// field.
func repoCodeMultipart(t *testing.T, filename string, content []byte) (string, io.Reader) {
	t.Helper()

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreateFormFile("file", filename)
	require.NoError(t, err)
	_, err = part.Write(content)
	require.NoError(t, err)
	require.NoError(t, w.Close())
	return w.FormDataContentType(), &buf
}

// repoCodeGet returns the body of a GET request that must succeed.
func repoCodeGet(t *testing.T, c *client, path string) string {
	t.Helper()

	status, body := c.do(http.MethodGet, path, nil)
	require.Equal(t, http.StatusOK, status, "GET %s, response: %s", path, body)
	return string(body)
}

type repoCodeCommitItem struct {
	SHA         string `json:"sha"`
	SubjectHTML string `json:"subjectHTML"`
}

func repoCodeSHAs(items []repoCodeCommitItem) []string {
	shas := make([]string, 0, len(items))
	for _, item := range items {
		shas = append(shas, item.SHA)
	}
	return shas
}

func TestRepoCodeBrowse(t *testing.T) {
	t.Parallel()

	owner := seedUser(t, CreateUserOptions{})
	other := seedUser(t, CreateUserOptions{})
	repo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
	private := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true, Private: true})

	ownerClient := authenticatedClient(t, owner)
	otherClient := authenticatedClient(t, other)
	anonymous := newClient(t)

	first := repoCodeGit(t, repo, "rev-parse", "refs/heads/main")
	second := repoCodeNewFile(t, ownerClient, repo, "docs/guide.md", "# Guide\n\nRead **this** first.\n", "Add guide")
	api := repoCodeAPI(repo)

	t.Run("header", func(t *testing.T) {
		var header struct {
			ID          int64  `json:"id"`
			Owner       string `json:"owner"`
			Name        string `json:"name"`
			Visibility  string `json:"visibility"`
			WikiEnabled bool   `json:"wikiEnabled"`
		}
		anonymous.doJSON(http.MethodGet, api+"/header", nil, http.StatusOK, &header)
		assert.Equal(t, repo.ID, header.ID)
		assert.Equal(t, repo.Owner, header.Owner)
		assert.Equal(t, repo.Name, header.Name)
		assert.Equal(t, "public", header.Visibility)
		assert.True(t, header.WikiEnabled)

		ownerClient.doJSON(http.MethodGet, repoCodeAPI(private)+"/header", nil, http.StatusOK, &header)
		assert.Equal(t, "private", header.Visibility)
		for name, c := range map[string]*client{"anonymous": anonymous, "other": otherClient} {
			status, body := c.do(http.MethodGet, repoCodeAPI(private)+"/header", nil)
			assert.Equal(t, http.StatusNotFound, status, "%s viewer, response: %s", name, body)
		}
	})

	type entry struct {
		Name   string `json:"name"`
		Type   string `json:"type"`
		Commit struct {
			SHA string `json:"sha"`
		} `json:"commit"`
	}
	type srcResponse struct {
		Ref      string  `json:"ref"`
		RefType  string  `json:"refType"`
		CommitID string  `json:"commitID"`
		Path     string  `json:"path"`
		Kind     string  `json:"kind"`
		Entries  []entry `json:"entries"`
		File     *struct {
			Name    string  `json:"name"`
			Kind    string  `json:"kind"`
			Content *string `json:"content"`
			HTML    string  `json:"html"`
		} `json:"file"`
		Summary *struct {
			CommitCount int64 `json:"commitCount"`
		} `json:"summary"`
		Branches []string `json:"branches"`
		Overview []struct {
			Kind string `json:"kind"`
			HTML string `json:"html"`
		} `json:"overview"`
	}

	t.Run("src", func(t *testing.T) {
		var root srcResponse
		anonymous.doJSON(http.MethodGet, api+"/src", nil, http.StatusOK, &root)
		assert.Equal(t, "main", root.Ref)
		assert.Equal(t, "branch", root.RefType)
		assert.Equal(t, second, root.CommitID)
		assert.Equal(t, "dir", root.Kind)
		require.Len(t, root.Entries, 2)
		assert.Equal(t, "docs", root.Entries[0].Name)
		assert.Equal(t, "dir", root.Entries[0].Type)
		assert.Equal(t, second, root.Entries[0].Commit.SHA)
		assert.Equal(t, "README.md", root.Entries[1].Name)
		assert.Equal(t, first, root.Entries[1].Commit.SHA)
		require.NotNil(t, root.Summary)
		assert.Equal(t, repoCodeGit(t, repo, "rev-list", "--count", "main"), "2")
		assert.EqualValues(t, 2, root.Summary.CommitCount)
		assert.Equal(t, []string{"main"}, root.Branches)
		// The page links headings to themselves by these ids. The text of the
		// heading may vary, e.g., the hex of the name can link to a commit.
		require.Len(t, root.Overview, 1)
		assert.Equal(t, "readme", root.Overview[0].Kind)
		assert.Contains(t, root.Overview[0].HTML, `<h1 id="user-content-`+repo.Name+`">`)

		var dir srcResponse
		anonymous.doJSON(http.MethodGet, api+"/src/main/docs", nil, http.StatusOK, &dir)
		assert.Equal(t, "docs", dir.Path)
		assert.Equal(t, "dir", dir.Kind)
		require.Len(t, dir.Entries, 1)
		assert.Equal(t, "guide.md", dir.Entries[0].Name)
		assert.Equal(t, "file", dir.Entries[0].Type)

		var file srcResponse
		anonymous.doJSON(http.MethodGet, api+"/src/main/docs/guide.md", nil, http.StatusOK, &file)
		assert.Equal(t, "file", file.Kind)
		require.NotNil(t, file.File)
		assert.Equal(t, "guide.md", file.File.Name)
		assert.Equal(t, "markup", file.File.Kind)
		require.NotNil(t, file.File.Content)
		assert.Equal(t, "# Guide\n\nRead **this** first.\n", *file.File.Content)
		assert.Contains(t, file.File.HTML, `<h1 id="user-content-guide">Guide</h1>`)
		assert.Contains(t, file.File.HTML, "<strong>this</strong>")

		// The first commit has no guide yet.
		status, body := anonymous.do(http.MethodGet, api+"/src/"+first+"/docs/guide.md", nil)
		assert.Equal(t, http.StatusNotFound, status, "response: %s", body)
		status, body = anonymous.do(http.MethodGet, api+"/src/main/missing.txt", nil)
		assert.Equal(t, http.StatusNotFound, status, "response: %s", body)
	})

	t.Run("paths", func(t *testing.T) {
		type pathsResponse struct {
			Ref      string   `json:"ref"`
			CommitID string   `json:"commitID"`
			Paths    []string `json:"paths"`
		}
		var paths pathsResponse
		anonymous.doJSON(http.MethodGet, api+"/paths", nil, http.StatusOK, &paths)
		assert.Equal(t, "main", paths.Ref)
		assert.Equal(t, second, paths.CommitID)
		assert.ElementsMatch(t, []string{"README.md", "docs/guide.md"}, paths.Paths)

		anonymous.doJSON(http.MethodGet, api+"/paths/"+first, nil, http.StatusOK, &paths)
		assert.Equal(t, first, paths.CommitID)
		assert.Equal(t, []string{"README.md"}, paths.Paths)

		status, body := otherClient.do(http.MethodGet, repoCodeAPI(private)+"/paths", nil)
		assert.Equal(t, http.StatusNotFound, status, "response: %s", body)
	})

	t.Run("commits", func(t *testing.T) {
		type commitsResponse struct {
			Ref     string               `json:"ref"`
			Path    string               `json:"path"`
			Search  bool                 `json:"search"`
			Commits []repoCodeCommitItem `json:"commits"`
		}
		var all commitsResponse
		anonymous.doJSON(http.MethodGet, api+"/commits/main", nil, http.StatusOK, &all)
		assert.Equal(t, "main", all.Ref)
		assert.Equal(t, strings.Fields(repoCodeGit(t, repo, "rev-list", "main")), repoCodeSHAs(all.Commits))
		assert.Equal(t, []string{second, first}, repoCodeSHAs(all.Commits))
		assert.Contains(t, all.Commits[0].SubjectHTML, "Add guide")

		var docs commitsResponse
		anonymous.doJSON(http.MethodGet, api+"/commits/main/docs", nil, http.StatusOK, &docs)
		assert.Equal(t, "docs", docs.Path)
		assert.Equal(t, []string{second}, repoCodeSHAs(docs.Commits))

		var search commitsResponse
		anonymous.doJSON(http.MethodGet, api+"/commits/main/search?q=guide", nil, http.StatusOK, &search)
		assert.True(t, search.Search)
		assert.Equal(t, []string{second}, repoCodeSHAs(search.Commits))

		status, body := otherClient.do(http.MethodGet, repoCodeAPI(private)+"/commits/main", nil)
		assert.Equal(t, http.StatusNotFound, status, "response: %s", body)
	})

	t.Run("commit", func(t *testing.T) {
		var commit struct {
			SHA     string   `json:"sha"`
			Subject string   `json:"subject"`
			Parents []string `json:"parents"`
			Author  struct {
				Name       string `json:"name"`
				Email      string `json:"email"`
				ProfileURL string `json:"profileURL"`
			} `json:"author"`
		}
		anonymous.doJSON(http.MethodGet, api+"/commit/"+second, nil, http.StatusOK, &commit)
		assert.Equal(t, second, commit.SHA)
		assert.Equal(t, "Add guide", commit.Subject)
		assert.Equal(t, []string{first}, commit.Parents)
		assert.Equal(t, repoCodeGit(t, repo, "log", "-1", "--format=%an", second), commit.Author.Name)
		assert.Equal(t, owner.Email, commit.Author.Email)
		assert.Equal(t, "/"+owner.Name, commit.Author.ProfileURL)

		status, body := anonymous.do(http.MethodGet, api+"/commit/"+strings.Repeat("0", 40), nil)
		assert.Equal(t, http.StatusNotFound, status, "response: %s", body)
		privateHead := repoCodeGit(t, private, "rev-parse", "refs/heads/main")
		status, body = otherClient.do(http.MethodGet, repoCodeAPI(private)+"/commit/"+privateHead, nil)
		assert.Equal(t, http.StatusNotFound, status, "response: %s", body)
	})

	t.Run("markdown", func(t *testing.T) {
		var md struct {
			HTML string `json:"html"`
		}
		anonymous.doJSON(http.MethodGet, api+"/markdown/"+second+"/docs/guide.md", nil, http.StatusOK, &md)
		assert.Contains(t, md.HTML, "Guide</h1>")
		assert.Contains(t, md.HTML, "<strong>this</strong>")

		status, body := anonymous.do(http.MethodGet, api+"/markdown/"+first+"/docs/guide.md", nil)
		assert.Equal(t, http.StatusNotFound, status, "response: %s", body)
	})

	t.Run("compare", func(t *testing.T) {
		var compare struct {
			Before       string               `json:"before"`
			After        string               `json:"after"`
			Base         string               `json:"base"`
			Commits      []repoCodeCommitItem `json:"commits"`
			TotalCommits int64                `json:"totalCommits"`
		}
		anonymous.doJSON(http.MethodGet, api+"/compare/"+first+"..."+second, nil, http.StatusOK, &compare)
		assert.Equal(t, first, compare.Base)
		assert.Equal(t, second, compare.After)
		assert.Equal(t, []string{second}, repoCodeSHAs(compare.Commits))
		assert.EqualValues(t, 1, compare.TotalCommits)

		status, body := anonymous.do(http.MethodGet, api+"/compare/"+first+"..."+strings.Repeat("a", 40), nil)
		assert.Equal(t, http.StatusNotFound, status, "response: %s", body)
	})
}

// TestRepoCodeNotebookAndPDF checks what the code browser gets for a Jupyter
// notebook and a PDF file, which it shows without vendored libraries.
func TestRepoCodeNotebookAndPDF(t *testing.T) {
	t.Parallel()

	owner := seedUser(t, CreateUserOptions{})
	repo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
	ownerClient := authenticatedClient(t, owner)
	anonymous := newClient(t)
	api := repoCodeAPI(repo)

	notebook := `{
 "nbformat": 4,
 "nbformat_minor": 5,
 "metadata": {"language_info": {"file_extension": ".py"}},
 "cells": [
  {"cell_type": "markdown", "source": ["# Report\n", "See the **[handbook](handbook.md)**."]},
  {"cell_type": "code", "execution_count": 1, "source": "print(1)", "outputs": [
   {"output_type": "stream", "name": "stdout", "text": ["1\n"]}
  ]},
  {"cell_type": "code", "execution_count": 2, "source": "df", "outputs": [
   {"output_type": "execute_result", "execution_count": 2, "data": {
    "text/plain": "table",
    "text/html": "<table><tr><td>Q1</td></tr></table><script>alert(1)</script>"
   }},
   {"output_type": "display_data", "data": {"image/png": "iVBORw0KGgo=\n", "text/plain": "<Figure>"}},
   {"output_type": "display_data", "data": {"application/javascript": "alert(2)"}},
   {"output_type": "error", "ename": "KeyError", "evalue": "'x'", "traceback": ["KeyError: 'x'"]}
  ]},
  {"cell_type": "raw", "source": "raw text"}
 ]
}`
	repoCodeNewFile(t, ownerClient, repo, "docs/report.ipynb", notebook, "Add notebook")
	repoCodeNewFile(t, ownerClient, repo, "docs/broken.ipynb", `{"cells": [ not json`, "Add broken notebook")
	repoCodeNewFile(t, ownerClient, repo, "docs/manual.pdf", "%PDF-1.4\n1 0 obj\n<< /Type /Catalog >>\nendobj\n%%EOF\n", "Add manual")

	type output struct {
		Type           string `json:"type"`
		ExecutionCount *int   `json:"executionCount"`
		Text           string `json:"text"`
		Src            string `json:"src"`
		HTML           string `json:"html"`
	}
	type srcResponse struct {
		File struct {
			Kind     string  `json:"kind"`
			Content  *string `json:"content"`
			Notebook *struct {
				FileExtension string `json:"fileExtension"`
				Cells         []struct {
					Type           string   `json:"type"`
					HTML           string   `json:"html"`
					Source         string   `json:"source"`
					ExecutionCount *int     `json:"executionCount"`
					Outputs        []output `json:"outputs"`
				} `json:"cells"`
			} `json:"notebook"`
		} `json:"file"`
	}

	t.Run("notebook", func(t *testing.T) {
		var resp srcResponse
		anonymous.doJSON(http.MethodGet, api+"/src/main/docs/report.ipynb", nil, http.StatusOK, &resp)
		assert.Equal(t, "notebook", resp.File.Kind)
		// The code view of the notebook shows its JSON.
		require.NotNil(t, resp.File.Content)
		assert.Equal(t, notebook, *resp.File.Content)

		nb := resp.File.Notebook
		require.NotNil(t, nb)
		assert.Equal(t, ".py", nb.FileExtension)
		require.Len(t, nb.Cells, 4)

		markdown := nb.Cells[0]
		assert.Equal(t, "markdown", markdown.Type)
		assert.Contains(t, markdown.HTML, "<h1")
		assert.Contains(t, markdown.HTML, "<strong>")
		assert.Contains(t, markdown.HTML, `href="/`+repo.Owner+"/"+repo.Name+`/src/main/docs/handbook.md"`)

		assert.Equal(t, "code", nb.Cells[1].Type)
		assert.Equal(t, "print(1)", nb.Cells[1].Source)
		require.NotNil(t, nb.Cells[1].ExecutionCount)
		assert.Equal(t, 1, *nb.Cells[1].ExecutionCount)
		require.Len(t, nb.Cells[1].Outputs, 1)
		assert.Equal(t, output{Type: "text", Text: "1\n"}, nb.Cells[1].Outputs[0])

		// The JavaScript output has nothing a browser shows safely, so it is
		// left out.
		outputs := nb.Cells[2].Outputs
		require.Len(t, outputs, 3)
		assert.Equal(t, "html", outputs[0].Type)
		require.NotNil(t, outputs[0].ExecutionCount)
		assert.Equal(t, 2, *outputs[0].ExecutionCount)
		assert.Contains(t, outputs[0].HTML, "<td>Q1</td>")
		assert.NotContains(t, outputs[0].HTML, "<script")
		assert.Equal(t, output{Type: "image", Src: "data:image/png;base64,iVBORw0KGgo="}, outputs[1])
		assert.Equal(t, output{Type: "error", Text: "KeyError: 'x'"}, outputs[2])

		assert.Equal(t, "raw", nb.Cells[3].Type)
		assert.Equal(t, "raw text", nb.Cells[3].Source)
	})

	t.Run("invalid notebook shows as text", func(t *testing.T) {
		var resp srcResponse
		anonymous.doJSON(http.MethodGet, api+"/src/main/docs/broken.ipynb", nil, http.StatusOK, &resp)
		assert.Equal(t, "text", resp.File.Kind)
		assert.Nil(t, resp.File.Notebook)
		require.NotNil(t, resp.File.Content)
		assert.Equal(t, `{"cells": [ not json`, *resp.File.Content)
	})

	t.Run("pdf", func(t *testing.T) {
		var resp srcResponse
		anonymous.doJSON(http.MethodGet, api+"/src/main/docs/manual.pdf", nil, http.StatusOK, &resp)
		assert.Equal(t, "pdf", resp.File.Kind)
		assert.Nil(t, resp.File.Content)
		raw := repoCodeGet(t, anonymous, repoCodePage(repo)+"/raw/main/docs/manual.pdf")
		assert.True(t, strings.HasPrefix(raw, "%PDF-1.4"), raw)
	})
}

// TestRepoCodeLinkedImages checks that images inside links, e.g., README
// badges, load from the repository and keep the author's link, both in
// Markdown without raw HTML and in Markdown with it, which render differently.
func TestRepoCodeLinkedImages(t *testing.T) {
	t.Parallel()

	owner := seedUser(t, CreateUserOptions{})
	repo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
	ownerClient := authenticatedClient(t, owner)
	anonymous := newClient(t)
	api := repoCodeAPI(repo)
	page := repoCodePage(repo)

	const badge = `<svg xmlns="http://www.w3.org/2000/svg" width="20" height="20"><rect width="20" height="20" fill="red"/></svg>` + "\n"
	repoCodeNewFile(t, ownerClient, repo, "img/badge.svg", badge, "Add badge")
	repoCodeNewFile(t, ownerClient, repo, "badges.md", `Raw HTML badges:

<a href="https://ci.example.com/raw"><img src="img/badge.svg" alt="raw"></a>
<a href="https://ci.example.com/self-closing"><img src="img/badge.svg" alt="self-closing" /></a>
[![markdown](img/badge.svg)](https://ci.example.com/markdown)
`, "Add badges")
	status, body := ownerClient.do(http.MethodPost, api+"/editor/files", map[string]any{
		"branch":     "main",
		"oldPath":    "README.md",
		"path":       "README.md",
		"content":    "[![relative](img/badge.svg)](https://ci.example.com/relative) [![root](/img/badge.svg)](https://ci.example.com/root) [![absolute](https://img.example.com/badge.svg)](https://ci.example.com/absolute)\n",
		"lastCommit": repoCodeGit(t, repo, "rev-parse", "refs/heads/main"),
		"summary":    "Add badges to README",
	})
	require.Equal(t, http.StatusOK, status, "response: %s", body)

	type linkedImage struct {
		href, src string
	}
	// linkedImages returns the images in rendered HTML by their alternative
	// text, with the link around each.
	linkedImages := func(t *testing.T, rendered string) map[string]linkedImage {
		t.Helper()

		images := map[string]linkedImage{}
		var href string
		z := html.NewTokenizer(strings.NewReader(rendered))
		for z.Next() != html.ErrorToken {
			tok := z.Token()
			attrs := map[string]string{}
			for _, a := range tok.Attr {
				attrs[a.Key] = a.Val
			}
			switch {
			case tok.Type == html.StartTagToken && tok.Data == "a":
				href = attrs["href"]
			case tok.Type == html.EndTagToken && tok.Data == "a":
				href = ""
			case (tok.Type == html.StartTagToken || tok.Type == html.SelfClosingTagToken) && tok.Data == "img":
				images[attrs["alt"]] = linkedImage{href: href, src: attrs["src"]}
			}
		}
		return images
	}
	resolved := page + "/raw/main/img/badge.svg"

	t.Run("README without raw HTML", func(t *testing.T) {
		var root struct {
			Overview []struct {
				Kind string `json:"kind"`
				HTML string `json:"html"`
			} `json:"overview"`
		}
		anonymous.doJSON(http.MethodGet, api+"/src", nil, http.StatusOK, &root)
		require.Len(t, root.Overview, 1)
		require.Equal(t, "readme", root.Overview[0].Kind)
		assert.Equal(t, map[string]linkedImage{
			"relative": {href: "https://ci.example.com/relative", src: resolved},
			"root":     {href: "https://ci.example.com/root", src: resolved},
			"absolute": {href: "https://ci.example.com/absolute", src: "https://img.example.com/badge.svg"},
		}, linkedImages(t, root.Overview[0].HTML))
	})

	t.Run("file with raw HTML", func(t *testing.T) {
		var file struct {
			File struct {
				HTML string `json:"html"`
			} `json:"file"`
		}
		anonymous.doJSON(http.MethodGet, api+"/src/main/badges.md", nil, http.StatusOK, &file)
		assert.Equal(t, map[string]linkedImage{
			"raw":          {href: "https://ci.example.com/raw", src: resolved},
			"self-closing": {href: "https://ci.example.com/self-closing", src: resolved},
			"markdown":     {href: "https://ci.example.com/markdown", src: resolved},
		}, linkedImages(t, file.File.HTML))
	})

	t.Run("resolved source loads", func(t *testing.T) {
		assert.Equal(t, badge, repoCodeGet(t, anonymous, resolved))
	})
}

func TestRepoCodeRawAndDiffs(t *testing.T) {
	t.Parallel()

	owner := seedUser(t, CreateUserOptions{})
	repo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
	private := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true, Private: true})
	ownerClient := authenticatedClient(t, owner)
	anonymous := newClient(t)

	first := repoCodeGit(t, repo, "rev-parse", "refs/heads/main")
	second := repoCodeNewFile(t, ownerClient, repo, "notes.txt", "hello\n", "Add notes")
	third := repoCodeNewFile(t, ownerClient, repo, "todo.txt", "ship it\n", "Add todo")
	page := repoCodePage(repo)

	t.Run("raw", func(t *testing.T) {
		status, header, body := anonymous.doRaw(http.MethodGet, page+"/raw/main/notes.txt", "", nil)
		require.Equal(t, http.StatusOK, status, "response: %s", body)
		assert.Equal(t, "hello\n", string(body))
		assert.Equal(t, "text/plain; charset=utf-8", header.Get("Content-Type"))

		assert.Equal(t, repoCodeGit(t, repo, "show", first+":README.md")+"\n", repoCodeGet(t, anonymous, page+"/raw/"+first+"/README.md"))
		status, body = anonymous.do(http.MethodGet, page+"/raw/"+first+"/notes.txt", nil)
		assert.Equal(t, http.StatusNotFound, status, "response: %s", body)
		status, body = anonymous.do(http.MethodGet, page+"/raw/nope/notes.txt", nil)
		assert.Equal(t, http.StatusNotFound, status, "response: %s", body)

		status, body = anonymous.do(http.MethodGet, repoCodePage(private)+"/raw/main/README.md", nil)
		assert.Equal(t, http.StatusNotFound, status, "response: %s", body)
		repoCodeGet(t, ownerClient, repoCodePage(private)+"/raw/main/README.md")
	})

	t.Run("commit diff and patch", func(t *testing.T) {
		diff := repoCodeGet(t, anonymous, page+"/commit/"+second+".diff")
		assert.Contains(t, diff, "diff --git a/notes.txt b/notes.txt")
		assert.Contains(t, diff, "+hello")
		assert.NotContains(t, diff, "todo.txt")

		patch := repoCodeGet(t, anonymous, page+"/commit/"+second+".patch")
		assert.True(t, strings.HasPrefix(patch, "From "+second), "patch: %s", patch)
		assert.Contains(t, patch, "Subject: [PATCH] Add notes")
		assert.Contains(t, patch, "+hello")

		status, body := anonymous.do(http.MethodGet, page+"/commit/"+strings.Repeat("0", 40)+".diff", nil)
		assert.Equal(t, http.StatusNotFound, status, "response: %s", body)
		privateHead := repoCodeGit(t, private, "rev-parse", "refs/heads/main")
		status, body = anonymous.do(http.MethodGet, repoCodePage(private)+"/commit/"+privateHead+".patch", nil)
		assert.Equal(t, http.StatusNotFound, status, "response: %s", body)
	})

	t.Run("compare diff", func(t *testing.T) {
		diff := repoCodeGet(t, anonymous, page+"/compare/"+first+"..."+third+".diff")
		assert.Contains(t, diff, "diff --git a/notes.txt b/notes.txt")
		assert.Contains(t, diff, "+hello")
		assert.Contains(t, diff, "diff --git a/todo.txt b/todo.txt")
		assert.Contains(t, diff, "+ship it")

		diff = repoCodeGet(t, anonymous, page+"/compare/"+second+"..."+third+".diff")
		assert.NotContains(t, diff, "notes.txt")
		assert.Contains(t, diff, "+ship it")
	})
}

func TestRepoCodeEditor(t *testing.T) {
	t.Parallel()

	owner := seedUser(t, CreateUserOptions{})
	other := seedUser(t, CreateUserOptions{})
	repo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
	ownerClient := authenticatedClient(t, owner)
	otherClient := authenticatedClient(t, other)
	api := repoCodeAPI(repo)
	page := repoCodePage(repo)

	type commitResult struct {
		Location string `json:"location"`
	}
	type target struct {
		Branch     string  `json:"branch"`
		Path       string  `json:"path"`
		LastCommit string  `json:"lastCommit"`
		Kind       string  `json:"kind"`
		Content    *string `json:"content"`
	}

	created := repoCodeNewFile(t, ownerClient, repo, "notes.txt", "one\n", "Add notes")
	assert.Equal(t, "Add notes", repoCodeGit(t, repo, "log", "-1", "--format=%s", "main"))
	assert.Equal(t, "one\n", repoCodeGet(t, ownerClient, page+"/raw/main/notes.txt"))

	t.Run("target", func(t *testing.T) {
		var got target
		ownerClient.doJSON(http.MethodGet, api+"/editor/main/notes.txt", nil, http.StatusOK, &got)
		assert.Equal(t, "main", got.Branch)
		assert.Equal(t, "notes.txt", got.Path)
		assert.Equal(t, created, got.LastCommit)
		assert.Equal(t, "file", got.Kind)
		require.NotNil(t, got.Content)
		assert.Equal(t, "one\n", *got.Content)

		var missing target
		ownerClient.doJSON(http.MethodGet, api+"/editor/main/missing.txt", nil, http.StatusOK, &missing)
		assert.Equal(t, "none", missing.Kind)
		assert.Equal(t, created, missing.LastCommit)
		assert.Nil(t, missing.Content)

		status, body := ownerClient.do(http.MethodGet, api+"/editor/nope/notes.txt", nil)
		assert.Equal(t, http.StatusNotFound, status, "response: %s", body)
	})

	t.Run("no write access", func(t *testing.T) {
		status, body := otherClient.do(http.MethodGet, api+"/editor/main/notes.txt", nil)
		assert.Equal(t, http.StatusNotFound, status, "response: %s", body)
		status, body = otherClient.do(http.MethodPost, api+"/editor/files", map[string]any{
			"branch": "main", "path": "intruder.txt", "content": "x",
		})
		assert.Equal(t, http.StatusNotFound, status, "response: %s", body)
		status, body = newClient(t).do(http.MethodPost, api+"/editor/delete", map[string]any{"branch": "main", "path": "notes.txt"})
		assert.Equal(t, http.StatusUnauthorized, status, "response: %s", body)
		assert.Equal(t, created, repoCodeGit(t, repo, "rev-parse", "refs/heads/main"))
	})

	t.Run("update, conflict and rename", func(t *testing.T) {
		repo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
		api := repoCodeAPI(repo)
		page := repoCodePage(repo)
		base := repoCodeNewFile(t, ownerClient, repo, "notes.txt", "one\n", "Add notes")

		var result commitResult
		ownerClient.doJSON(http.MethodPost, api+"/editor/files", map[string]any{
			"branch":     "main",
			"oldPath":    "notes.txt",
			"path":       "notes.txt",
			"content":    "one\r\ntwo\r\n",
			"lastCommit": base,
			"summary":    "Update notes",
			"message":    "With a second line.",
		}, http.StatusOK, &result)
		assert.Equal(t, page+"/src/main/notes.txt", result.Location)
		updated := repoCodeGit(t, repo, "rev-parse", "refs/heads/main")
		assert.Equal(t, "one\ntwo\n", repoCodeGet(t, ownerClient, page+"/raw/main/notes.txt"))
		assert.Equal(t, "Update notes\n\nWith a second line.", strings.TrimSpace(repoCodeGit(t, repo, "log", "-1", "--format=%B", "main")))

		// A change made from the stale commit would overwrite the update.
		var conflict struct {
			Error      string `json:"error"`
			LastCommit string `json:"lastCommit"`
		}
		ownerClient.doJSON(http.MethodPost, api+"/editor/files", map[string]any{
			"branch":     "main",
			"oldPath":    "notes.txt",
			"path":       "notes.txt",
			"content":    "stale\n",
			"lastCommit": base,
		}, http.StatusConflict, &conflict)
		assert.Equal(t, updated, conflict.LastCommit)
		assert.Equal(t, updated, repoCodeGit(t, repo, "rev-parse", "refs/heads/main"))

		ownerClient.doJSON(http.MethodPost, api+"/editor/files", map[string]any{
			"branch":     "main",
			"oldPath":    "notes.txt",
			"path":       "docs/notes.txt",
			"content":    "one\ntwo\n",
			"lastCommit": updated,
			"summary":    "Move notes",
		}, http.StatusOK, &result)
		assert.Equal(t, page+"/src/main/docs/notes.txt", result.Location)
		assert.Equal(t, "README.md\ndocs/notes.txt", repoCodeGit(t, repo, "ls-tree", "-r", "--name-only", "main"))

		status, body := ownerClient.do(http.MethodPost, api+"/editor/files", map[string]any{
			"branch": "main", "path": "README.md", "content": "x",
		})
		assert.Equal(t, http.StatusUnprocessableEntity, status, "response: %s", body)

		status, body = ownerClient.do(http.MethodPost, api+"/editor/files", map[string]any{
			"branch": "main", "path": ".git/config", "content": "x",
		})
		assert.Equal(t, http.StatusUnprocessableEntity, status, "response: %s", body)
		assert.Contains(t, string(body), "is not allowed as a file path")
	})

	t.Run("new branch", func(t *testing.T) {
		repo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
		mainHead := repoCodeGit(t, repo, "rev-parse", "refs/heads/main")

		var result commitResult
		ownerClient.doJSON(http.MethodPost, repoCodeAPI(repo)+"/editor/files", map[string]any{
			"branch":    "main",
			"newBranch": "feature",
			"path":      "feature.txt",
			"content":   "new\n",
			"summary":   "Add feature",
		}, http.StatusOK, &result)
		assert.Equal(t, repoCodePage(repo)+"/compare/main..."+owner.Name+":feature", result.Location)
		assert.Equal(t, mainHead, repoCodeGit(t, repo, "rev-parse", "refs/heads/main"))
		assert.Equal(t, mainHead, repoCodeGit(t, repo, "rev-parse", "refs/heads/feature~1"))
		assert.Equal(t, "new\n", repoCodeGet(t, ownerClient, repoCodePage(repo)+"/raw/feature/feature.txt"))
	})

	t.Run("delete", func(t *testing.T) {
		repo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
		repoCodeNewFile(t, ownerClient, repo, "a/b/gone.txt", "bye\n", "Add gone")

		var result commitResult
		ownerClient.doJSON(http.MethodPost, repoCodeAPI(repo)+"/editor/delete", map[string]any{
			"branch":  "main",
			"path":    "a/b/gone.txt",
			"summary": "Remove gone",
		}, http.StatusOK, &result)
		// The directories that only held the file are gone too.
		assert.Equal(t, repoCodePage(repo)+"/src/main/", result.Location)
		assert.Equal(t, "README.md", repoCodeGit(t, repo, "ls-tree", "-r", "--name-only", "main"))
		assert.Equal(t, "Remove gone", repoCodeGit(t, repo, "log", "-1", "--format=%s", "main"))

		status, body := ownerClient.do(http.MethodPost, repoCodeAPI(repo)+"/editor/delete", map[string]any{
			"branch": "main", "path": "a/b/gone.txt",
		})
		assert.Equal(t, http.StatusNotFound, status, "response: %s", body)
	})

	t.Run("upload", func(t *testing.T) {
		repo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
		api := repoCodeAPI(repo)
		upload := func(name, content string) string {
			contentType, form := repoCodeMultipart(t, name, []byte(content))
			status, _, body := ownerClient.doRaw(http.MethodPost, api+"/editor/uploads", contentType, form)
			require.Equal(t, http.StatusCreated, status, "response: %s", body)
			var resp struct {
				UUID string `json:"uuid"`
			}
			require.NoError(t, json.Unmarshal(body, &resp))
			require.NotEmpty(t, resp.UUID)
			return resp.UUID
		}
		kept := upload("kept.txt", "kept content\n")
		dropped := upload("dropped.txt", "dropped content\n")

		status, body := ownerClient.do(http.MethodDelete, api+"/editor/uploads/"+dropped, nil)
		require.Equal(t, http.StatusNoContent, status, "response: %s", body)
		_, err := database.Handle.Uploads().GetByUUID(t.Context(), dropped)
		var notExist database.ErrUploadNotExist
		assert.ErrorAs(t, err, &notExist)

		// A deleted upload cannot be committed.
		status, body = ownerClient.do(http.MethodPost, api+"/editor/upload", map[string]any{
			"branch": "main", "path": "files", "files": []string{kept, dropped},
		})
		assert.Equal(t, http.StatusBadRequest, status, "response: %s", body)

		var result commitResult
		ownerClient.doJSON(http.MethodPost, api+"/editor/upload", map[string]any{
			"branch":  "main",
			"path":    "files",
			"files":   []string{kept},
			"summary": "Upload kept",
		}, http.StatusOK, &result)
		assert.Equal(t, repoCodePage(repo)+"/src/main/files", result.Location)
		assert.Equal(t, "kept content\n", repoCodeGet(t, ownerClient, repoCodePage(repo)+"/raw/main/files/kept.txt"))
		assert.Equal(t, "Upload kept", repoCodeGit(t, repo, "log", "-1", "--format=%s", "main"))

		// A file that would go into the ".git" directory fails the whole upload.
		head := repoCodeGit(t, repo, "rev-parse", "refs/heads/main")
		ok := upload("ok.txt", "ok\n")
		dotGit := upload(".git", "x\n")
		status, body = ownerClient.do(http.MethodPost, api+"/editor/upload", map[string]any{
			"branch": "main", "path": "", "files": []string{ok, dotGit},
		})
		assert.Equal(t, http.StatusUnprocessableEntity, status, "response: %s", body)
		assert.Contains(t, string(body), "is not allowed as a file path")
		assert.Equal(t, head, repoCodeGit(t, repo, "rev-parse", "refs/heads/main"))

		contentType, form := repoCodeMultipart(t, "x.txt", []byte("x"))
		status, _, body = otherClient.doRaw(http.MethodPost, api+"/editor/uploads", contentType, form)
		assert.Equal(t, http.StatusNotFound, status, "response: %s", body)
	})

	t.Run("indent", func(t *testing.T) {
		repo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
		repoCodeNewFile(t, ownerClient, repo, ".editorconfig",
			"root = true\n\n[*.go]\nindent_style = tab\ntab_width = 8\n\n[*.py]\nindent_style = space\nindent_size = 2\n", "Add editorconfig")

		type indent struct {
			Style    string `json:"style"`
			Size     int    `json:"size"`
			TabWidth int    `json:"tabWidth"`
		}
		var got indent
		ownerClient.doJSON(http.MethodGet, repoCodeAPI(repo)+"/editor-indent?branch=main&path=cmd/main.go", nil, http.StatusOK, &got)
		assert.Equal(t, indent{Style: "tab", TabWidth: 8}, got)
		ownerClient.doJSON(http.MethodGet, repoCodeAPI(repo)+"/editor-indent?branch=main&path=app.py", nil, http.StatusOK, &got)
		assert.Equal(t, indent{Style: "space", Size: 2, TabWidth: 2}, got)
		got = indent{}
		ownerClient.doJSON(http.MethodGet, repoCodeAPI(repo)+"/editor-indent?branch=main&path=notes.txt", nil, http.StatusOK, &got)
		assert.Equal(t, indent{}, got)

		status, body := ownerClient.do(http.MethodGet, repoCodeAPI(repo)+"/editor-indent?branch=nope&path=app.py", nil)
		assert.Equal(t, http.StatusNotFound, status, "response: %s", body)
		status, body = otherClient.do(http.MethodGet, repoCodeAPI(repo)+"/editor-indent?branch=main&path=app.py", nil)
		assert.Equal(t, http.StatusNotFound, status, "response: %s", body)
	})
}

func TestRepoCodeEditorUploadDeletedTwice(t *testing.T) {
	t.Parallel()

	owner := seedUser(t, CreateUserOptions{})
	repo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
	c := authenticatedClient(t, owner)

	contentType, form := repoCodeMultipart(t, "a.txt", []byte("a"))
	status, _, body := c.doRaw(http.MethodPost, repoCodeAPI(repo)+"/editor/uploads", contentType, form)
	require.Equal(t, http.StatusCreated, status, "response: %s", body)
	var upload struct {
		UUID string `json:"uuid"`
	}
	require.NoError(t, json.Unmarshal(body, &upload))
	status, body = c.do(http.MethodDelete, repoCodeAPI(repo)+"/editor/uploads/"+upload.UUID, nil)
	require.Equal(t, http.StatusNoContent, status, "response: %s", body)

	// The browser removes a file from the form again, e.g., after a retry, so
	// deleting an upload that is gone succeeds like the first time.
	status, body = c.do(http.MethodDelete, repoCodeAPI(repo)+"/editor/uploads/"+upload.UUID, nil)
	assert.Equal(t, http.StatusNoContent, status, "response: %s", body)
}

func TestRepoCodeBranches(t *testing.T) {
	t.Parallel()

	owner := seedUser(t, CreateUserOptions{})
	repo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
	empty := seedRepository(t, owner.ID, CreateRepositoryOptions{})
	ownerClient := authenticatedClient(t, owner)
	status, body := ownerClient.do(http.MethodPost, repoCodeAPI(repo)+"/editor/files", map[string]any{
		"branch": "main", "newBranch": "feature", "path": "f.txt", "content": "f\n",
	})
	require.Equal(t, http.StatusOK, status, "response: %s", body)

	type branch struct {
		Name     string `json:"name"`
		CommitID string `json:"commitID"`
	}
	type branchesResponse struct {
		DefaultBranch string `json:"defaultBranch"`
		View          string `json:"view"`
		Sections      []struct {
			Kind     string   `json:"kind"`
			Branches []branch `json:"branches"`
		} `json:"sections"`
		Branches []branch `json:"branches"`
	}

	anonymous := newClient(t)
	var all branchesResponse
	anonymous.doJSON(http.MethodGet, repoCodeAPI(repo)+"/branches?view=all", nil, http.StatusOK, &all)
	assert.Equal(t, "main", all.DefaultBranch)
	assert.Equal(t, "all", all.View)
	assert.ElementsMatch(t, []branch{
		{Name: "main", CommitID: repoCodeGit(t, repo, "rev-parse", "refs/heads/main")},
		{Name: "feature", CommitID: repoCodeGit(t, repo, "rev-parse", "refs/heads/feature")},
	}, all.Branches)

	var overview branchesResponse
	anonymous.doJSON(http.MethodGet, repoCodeAPI(repo)+"/branches", nil, http.StatusOK, &overview)
	require.NotEmpty(t, overview.Sections)
	assert.Equal(t, "default", overview.Sections[0].Kind)
	require.Len(t, overview.Sections[0].Branches, 1)
	assert.Equal(t, "main", overview.Sections[0].Branches[0].Name)

	status, body = anonymous.do(http.MethodGet, repoCodeAPI(repo)+"/branches?view=bogus", nil)
	assert.Equal(t, http.StatusBadRequest, status, "response: %s", body)
	status, body = ownerClient.do(http.MethodGet, repoCodeAPI(empty)+"/branches", nil)
	assert.Equal(t, http.StatusNotFound, status, "response: %s", body)
}

func TestRepoCodeReleases(t *testing.T) {
	t.Parallel()

	owner := seedUser(t, CreateUserOptions{})
	other := seedUser(t, CreateUserOptions{})
	repo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
	ownerClient := authenticatedClient(t, owner)
	otherClient := authenticatedClient(t, other)
	anonymous := newClient(t)
	api := repoCodeAPI(repo)
	head := repoCodeGit(t, repo, "rev-parse", "refs/heads/main")

	var form struct {
		Branches      []string `json:"branches"`
		DefaultBranch string   `json:"defaultBranch"`
	}
	ownerClient.doJSON(http.MethodGet, api+"/releases/new", nil, http.StatusOK, &form)
	assert.Equal(t, []string{"main"}, form.Branches)
	assert.Equal(t, "main", form.DefaultBranch)
	status, body := otherClient.do(http.MethodGet, api+"/releases/new", nil)
	assert.Equal(t, http.StatusNotFound, status, "response: %s", body)

	contentType, upload := repoCodeMultipart(t, "app.bin", []byte("binary build"))
	status, _, body = ownerClient.doRaw(http.MethodPost, api+"/releases/attachments", contentType, upload)
	require.Equal(t, http.StatusCreated, status, "response: %s", body)
	var attachment struct {
		UUID string `json:"uuid"`
		URL  string `json:"url"`
	}
	require.NoError(t, json.Unmarshal(body, &attachment))
	assert.True(t, strings.HasSuffix(attachment.URL, "/attachments/"+attachment.UUID), "URL %q", attachment.URL)

	contentType, upload = repoCodeMultipart(t, "app.bin", []byte("binary build"))
	status, _, body = otherClient.doRaw(http.MethodPost, api+"/releases/attachments", contentType, upload)
	assert.Equal(t, http.StatusNotFound, status, "response: %s", body)

	status, body = ownerClient.do(http.MethodPost, api+"/releases", map[string]any{
		"tagName":     "v1.0",
		"target":      "main",
		"title":       "First release",
		"content":     "Some **notes**.",
		"attachments": []string{attachment.UUID},
	})
	require.Equal(t, http.StatusCreated, status, "response: %s", body)
	assert.Equal(t, head, repoCodeGit(t, repo, "rev-parse", "v1.0^{commit}"))

	status, body = ownerClient.do(http.MethodPost, api+"/releases", map[string]any{
		"tagName": "v1.0", "target": "main", "title": "Again",
	})
	assert.Equal(t, http.StatusUnprocessableEntity, status, "response: %s", body)
	status, body = otherClient.do(http.MethodPost, api+"/releases", map[string]any{
		"tagName": "v9.9", "target": "main", "title": "Intruder",
	})
	assert.Equal(t, http.StatusNotFound, status, "response: %s", body)

	status, body = ownerClient.do(http.MethodPost, api+"/releases", map[string]any{
		"tagName": "v2.0", "target": "main", "title": "Upcoming", "isDraft": true,
	})
	require.Equal(t, http.StatusCreated, status, "response: %s", body)

	type release struct {
		TagName     string `json:"tagName"`
		Sha         string `json:"sha"`
		Title       string `json:"title"`
		IsDraft     bool   `json:"isDraft"`
		NoteHTML    string `json:"noteHTML"`
		Attachments []struct {
			UUID string `json:"uuid"`
			Name string `json:"name"`
		} `json:"attachments"`
	}
	type releasesResponse struct {
		Releases []release `json:"releases"`
	}
	var list releasesResponse
	anonymous.doJSON(http.MethodGet, api+"/releases", nil, http.StatusOK, &list)
	require.Len(t, list.Releases, 1, "drafts are only listed to writers")
	got := list.Releases[0]
	assert.Equal(t, "v1.0", got.TagName)
	assert.Equal(t, head, got.Sha)
	assert.Equal(t, "First release", got.Title)
	assert.Contains(t, got.NoteHTML, "<strong>notes</strong>")
	require.Len(t, got.Attachments, 1)
	assert.Equal(t, attachment.UUID, got.Attachments[0].UUID)
	assert.Equal(t, "app.bin", got.Attachments[0].Name)

	ownerClient.doJSON(http.MethodGet, api+"/releases", nil, http.StatusOK, &list)
	var tags []string
	for _, r := range list.Releases {
		tags = append(tags, r.TagName)
	}
	assert.ElementsMatch(t, []string{"v1.0", "v2.0"}, tags)

	type releaseEdit struct {
		TagName string `json:"tagName"`
		Target  string `json:"target"`
		Title   string `json:"title"`
		Content string `json:"content"`
		IsDraft bool   `json:"isDraft"`
	}
	var edit releaseEdit
	ownerClient.doJSON(http.MethodGet, api+"/releases/edit/v1.0", nil, http.StatusOK, &edit)
	assert.Equal(t, releaseEdit{TagName: "v1.0", Target: "main", Title: "First release", Content: "Some **notes**."}, edit)
	status, body = otherClient.do(http.MethodGet, api+"/releases/edit/v1.0", nil)
	assert.Equal(t, http.StatusNotFound, status, "response: %s", body)

	status, body = ownerClient.do(http.MethodPatch, api+"/releases/edit/v1.0", map[string]any{
		"title":       "First release, fixed",
		"content":     "Better notes.",
		"attachments": []string{attachment.UUID},
	})
	require.Equal(t, http.StatusNoContent, status, "response: %s", body)
	ownerClient.doJSON(http.MethodGet, api+"/releases/edit/v1.0", nil, http.StatusOK, &edit)
	assert.Equal(t, "First release, fixed", edit.Title)
	assert.Equal(t, "Better notes.", edit.Content)

	// Publishing the draft makes its tag.
	status, body = ownerClient.do(http.MethodPatch, api+"/releases/edit/v2.0", map[string]any{"title": "Second"})
	require.Equal(t, http.StatusNoContent, status, "response: %s", body)
	assert.Equal(t, head, repoCodeGit(t, repo, "rev-parse", "v2.0^{commit}"))
	anonymous.doJSON(http.MethodGet, api+"/releases", nil, http.StatusOK, &list)
	assert.Len(t, list.Releases, 2)

	status, body = otherClient.do(http.MethodDelete, api+"/releases/edit/v1.0", map[string]any{})
	assert.Equal(t, http.StatusNotFound, status, "response: %s", body)
	status, body = ownerClient.do(http.MethodDelete, api+"/releases/edit/v1.0", map[string]any{})
	require.Equal(t, http.StatusNoContent, status, "response: %s", body)
	status, body = ownerClient.do(http.MethodGet, api+"/releases/edit/v1.0", nil)
	assert.Equal(t, http.StatusNotFound, status, "response: %s", body)
	assert.Equal(t, "v2.0", repoCodeGit(t, repo, "tag", "--list"))

	// A release of an existing tag is dated by the commit of the tag.
	oldSHA := releaseOldCommit(t, database.RepoPath(repo.Owner, repo.Name), head)
	repoCodeGit(t, repo, "tag", "old", oldSHA)
	status, body = ownerClient.do(http.MethodPost, api+"/releases", map[string]any{"tagName": "old", "target": "main", "title": "Old"})
	require.Equal(t, http.StatusCreated, status, "response: %s", body)
	var dated struct {
		Releases []struct {
			TagName   string    `json:"tagName"`
			Sha       string    `json:"sha"`
			CreatedAt time.Time `json:"createdAt"`
		} `json:"releases"`
	}
	ownerClient.doJSON(http.MethodGet, api+"/releases", nil, http.StatusOK, &dated)
	var found bool
	for _, r := range dated.Releases {
		if r.TagName == "old" {
			found = true
			assert.Equal(t, oldSHA, r.Sha)
			assert.Equal(t, releaseOldCommitTime.Unix(), r.CreatedAt.Unix())
		}
	}
	assert.True(t, found, "release of the tag listed")
}

// An edit or deletion of a release whose client goes away after it started
// still updates the files of the release, and publishing a draft whose tag name
// is not valid says so.
func TestRepoCodeReleaseChanges(t *testing.T) {
	t.Parallel()

	owner := seedUser(t, CreateUserOptions{})
	repo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
	c := authenticatedClient(t, owner)
	api := repoCodeAPI(repo)
	upload := func() string {
		t.Helper()
		contentType, body := repoCodeMultipart(t, "app.bin", []byte("binary build"))
		status, _, resp := c.doRaw(http.MethodPost, api+"/releases/attachments", contentType, body)
		require.Equal(t, http.StatusCreated, status, "response: %s", resp)
		var attachment struct {
			UUID string `json:"uuid"`
		}
		require.NoError(t, json.Unmarshal(resp, &attachment))
		return attachment.UUID
	}
	gone := func(uuid string) bool {
		_, err := database.Handle.Attachments().GetByUUID(t.Context(), uuid)
		_, statErr := os.Stat(database.AttachmentLocalPath(uuid))
		return database.IsErrAttachmentNotExist(err) && os.IsNotExist(statErr)
	}

	oldUUID := upload()
	status, body := c.do(http.MethodPost, api+"/releases", map[string]any{"tagName": "v1.0.0", "target": "main", "title": "First", "attachments": []string{oldUUID}})
	require.Equal(t, http.StatusCreated, status, "response: %s", body)
	rel, err := database.Handle.Releases().GetByTagName(t.Context(), repo.ID, "v1.0.0")
	require.NoError(t, err)

	newUUID := upload()
	c.goAwayWhileLocked("SELECT id FROM release WHERE id = ? FOR UPDATE", rel.ID,
		http.MethodPatch, api+"/releases/edit/v1.0.0", map[string]any{"title": "First", "attachments": []string{newUUID}, "removedAttachments": []string{oldUUID}})
	require.Eventually(t, func() bool {
		a, err := database.Handle.Attachments().GetByUUID(t.Context(), newUUID)
		return gone(oldUUID) && err == nil && a.ReleaseID == rel.ID
	}, 10*time.Second, 100*time.Millisecond, "files of the release not updated")

	c.goAwayWhileLocked("SELECT id FROM release WHERE id = ? FOR UPDATE", rel.ID,
		http.MethodDelete, api+"/releases/edit/v1.0.0", nil)
	require.Eventually(t, func() bool {
		_, err := database.Handle.Releases().GetByID(t.Context(), rel.ID)
		return database.IsErrReleaseNotExist(err) && gone(newUUID)
	}, 10*time.Second, 100*time.Millisecond, "release and its files not deleted")

	status, body = c.do(http.MethodPost, api+"/releases", map[string]any{"tagName": "v2.0.0", "target": "main", "title": "Second"})
	require.Equal(t, http.StatusCreated, status, "response: %s", body)
	rel, err = database.Handle.Releases().GetByTagName(t.Context(), repo.ID, "v2.0.0")
	require.NoError(t, err)
	lateUUID := upload()
	status, body = c.whileLocked("SELECT id FROM release WHERE id = ? FOR UPDATE", rel.ID,
		http.MethodPatch, api+"/releases/edit/v2.0.0", map[string]any{"title": "Second", "attachments": []string{lateUUID}},
		func(tx *gorm.DB, _ context.CancelFunc) {
			require.NoError(t, tx.Exec("DELETE FROM release WHERE id = ?", rel.ID).Error)
		})
	assert.Equal(t, http.StatusNotFound, status, "response: %s", body)
	var notFound struct {
		Error string `json:"error"`
	}
	require.NoError(t, json.Unmarshal(body, &notFound))
	assert.Equal(t, "release does not exist", notFound.Error)
	late, err := database.Handle.Attachments().GetByUUID(t.Context(), lateUUID)
	require.NoError(t, err)
	assert.Zero(t, late.ReleaseID, "file attached to a deleted release")

	// Drafts get their tag only when published, which is when an invalid tag
	// name is found. The tag name cannot be edited then, so the error is not
	// one of a field.
	status, body = c.do(http.MethodPost, api+"/releases", map[string]any{"tagName": "bad..name", "target": "main", "title": "Draft", "isDraft": true})
	require.Equal(t, http.StatusCreated, status, "response: %s", body)
	status, body = c.do(http.MethodPatch, api+"/releases/edit/bad..name", map[string]any{"title": "Draft", "isDraft": false})
	require.Equal(t, http.StatusBadRequest, status, "response: %s", body)
	var resp struct {
		Error  string         `json:"error"`
		Fields map[string]any `json:"fields"`
	}
	require.NoError(t, json.Unmarshal(body, &resp))
	assert.Equal(t, "Tag name is not valid.", resp.Error)
	assert.Empty(t, resp.Fields)
}

func TestRepoCodeWiki(t *testing.T) {
	t.Parallel()

	owner := seedUser(t, CreateUserOptions{})
	other := seedUser(t, CreateUserOptions{})
	repo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
	private := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true, Private: true})
	ownerClient := authenticatedClient(t, owner)
	otherClient := authenticatedClient(t, other)
	anonymous := newClient(t)
	api := repoCodeAPI(repo)

	type wikiResponse struct {
		HasWiki bool `json:"hasWiki"`
		Page    *struct {
			Name   string `json:"name"`
			HTML   string `json:"html"`
			Author struct {
				Name string `json:"name"`
			} `json:"author"`
		} `json:"page"`
		Pages          []string `json:"pages"`
		ViewerCanWrite bool     `json:"viewerCanWrite"`
	}
	var wiki wikiResponse
	anonymous.doJSON(http.MethodGet, api+"/wiki", nil, http.StatusOK, &wiki)
	assert.False(t, wiki.HasWiki)

	var created struct {
		Name string `json:"name"`
	}
	ownerClient.doJSON(http.MethodPost, api+"/wiki", map[string]any{
		"title":   "Home",
		"content": "Welcome to the **wiki**.",
		"message": "Create home",
	}, http.StatusCreated, &created)
	assert.Equal(t, "Home", created.Name)
	ownerClient.doJSON(http.MethodPost, api+"/wiki", map[string]any{
		"title":   "Getting started",
		"content": "## Install it\n\nRun the binary.",
	}, http.StatusCreated, &created)
	assert.Equal(t, "Getting started", created.Name)

	status, body := ownerClient.do(http.MethodPost, api+"/wiki", map[string]any{"title": "Home", "content": "Again."})
	assert.Equal(t, http.StatusUnprocessableEntity, status, "response: %s", body)
	status, body = otherClient.do(http.MethodPost, api+"/wiki", map[string]any{"title": "Spam", "content": "Spam."})
	assert.Equal(t, http.StatusNotFound, status, "response: %s", body)

	wikiPath := database.WikiPath(repo.Owner, repo.Name)
	wikiGit := func(args ...string) string {
		out, err := exec.Command("git", append([]string{"-C", wikiPath}, args...)...).CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, out)
		return strings.TrimSuffix(string(out), "\n")
	}
	assert.Equal(t, "Welcome to the **wiki**.", wikiGit("show", "HEAD:Home.md"))
	assert.Contains(t, wikiGit("log", "--format=%s"), "Create home")

	anonymous.doJSON(http.MethodGet, api+"/wiki", nil, http.StatusOK, &wiki)
	assert.True(t, wiki.HasWiki)
	assert.False(t, wiki.ViewerCanWrite)
	require.NotNil(t, wiki.Page)
	assert.Equal(t, "Home", wiki.Page.Name)
	assert.Contains(t, wiki.Page.HTML, "<strong>wiki</strong>")
	assert.Equal(t, wikiGit("log", "-1", "--format=%an", "--", "Home.md"), wiki.Page.Author.Name)
	assert.ElementsMatch(t, []string{"Home", "Getting started"}, wiki.Pages)

	// Links written like GitHub's reach the page with spaces.
	anonymous.doJSON(http.MethodGet, api+"/wiki?page=Getting-started", nil, http.StatusOK, &wiki)
	assert.Equal(t, "Getting started", wiki.Page.Name)
	// The page links headings to themselves by these ids.
	assert.Contains(t, wiki.Page.HTML, `<h2 id="user-content-install-it">Install it</h2>`)
	status, body = anonymous.do(http.MethodGet, api+"/wiki?page=Missing", nil)
	assert.Equal(t, http.StatusNotFound, status, "response: %s", body)

	var pages struct {
		Pages []struct {
			Name string `json:"name"`
		} `json:"pages"`
		ViewerCanWrite bool `json:"viewerCanWrite"`
	}
	ownerClient.doJSON(http.MethodGet, api+"/wiki/pages", nil, http.StatusOK, &pages)
	assert.True(t, pages.ViewerCanWrite)
	var names []string
	for _, p := range pages.Pages {
		names = append(names, p.Name)
	}
	assert.ElementsMatch(t, []string{"Home", "Getting started"}, names)

	var source struct {
		Name    string `json:"name"`
		Content string `json:"content"`
	}
	ownerClient.doJSON(http.MethodGet, api+"/wiki/source?page=Home", nil, http.StatusOK, &source)
	assert.Equal(t, "Home", source.Name)
	assert.Equal(t, "Welcome to the **wiki**.", source.Content)
	status, body = otherClient.do(http.MethodGet, api+"/wiki/source?page=Home", nil)
	assert.Equal(t, http.StatusNotFound, status, "response: %s", body)

	ownerClient.doJSON(http.MethodPatch, api+"/wiki", map[string]any{
		"oldTitle": "Getting started",
		"title":    "Installation",
		"content":  "Install it with care.",
	}, http.StatusOK, &created)
	assert.Equal(t, "Installation", created.Name)
	assert.Equal(t, "Install it with care.", wikiGit("show", "HEAD:Installation.md"))
	status, body = ownerClient.do(http.MethodGet, api+"/wiki/source?page=Getting%20started", nil)
	assert.Equal(t, http.StatusNotFound, status, "response: %s", body)

	status, body = otherClient.do(http.MethodDelete, api+"/wiki?page=Installation", map[string]any{})
	assert.Equal(t, http.StatusNotFound, status, "response: %s", body)
	status, body = ownerClient.do(http.MethodDelete, api+"/wiki?page=Installation", map[string]any{})
	require.Equal(t, http.StatusNoContent, status, "response: %s", body)
	assert.Equal(t, "Home.md", wikiGit("ls-tree", "--name-only", "HEAD"))
	status, body = ownerClient.do(http.MethodDelete, api+"/wiki?page=Installation", map[string]any{})
	assert.Equal(t, http.StatusNotFound, status, "response: %s", body)

	status, body = otherClient.do(http.MethodGet, repoCodeAPI(private)+"/wiki", nil)
	assert.Equal(t, http.StatusNotFound, status, "response: %s", body)
}

func TestRepoCodeForks(t *testing.T) {
	t.Parallel()

	owner := seedUser(t, CreateUserOptions{})
	forker := seedUser(t, CreateUserOptions{})
	repo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
	empty := seedRepository(t, owner.ID, CreateRepositoryOptions{})

	base, err := getRepository(t, repo.ID)
	require.NoError(t, err)
	forkerUser, err := database.Handle.Users().GetByID(t.Context(), forker.ID)
	require.NoError(t, err)
	forkName := UniqueName("fork-")
	_, err = database.ForkRepository(t.Context(), logger, forkerUser, forkerUser, base, forkName, "")
	require.NoError(t, err)

	type fork struct {
		Owner     string `json:"owner"`
		Name      string `json:"name"`
		IsPrivate bool   `json:"isPrivate"`
	}
	var forks struct {
		Items []fork `json:"items"`
	}
	newClient(t).doJSON(http.MethodGet, repoCodeAPI(repo)+"/forks", nil, http.StatusOK, &forks)
	assert.Equal(t, []fork{{Owner: forker.Name, Name: forkName}}, forks.Items)

	var header struct {
		ForkCount int `json:"forkCount"`
	}
	newClient(t).doJSON(http.MethodGet, repoCodeAPI(repo)+"/header", nil, http.StatusOK, &header)
	assert.Equal(t, 1, header.ForkCount)

	status, body := newClient(t).do(http.MethodGet, repoCodeAPI(empty)+"/forks", nil)
	assert.Equal(t, http.StatusNotFound, status, "response: %s", body)
}

func TestRepoCodeArchive(t *testing.T) {
	t.Parallel()

	owner := seedUser(t, CreateUserOptions{})
	repo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
	private := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true, Private: true})
	ownerClient := authenticatedClient(t, owner)
	repoCodeNewFile(t, ownerClient, repo, "src/app.txt", "app content\n", "Add app")
	readme := repoCodeGit(t, repo, "show", "main:README.md") + "\n"
	anonymous := newClient(t)

	want := map[string]string{"README.md": readme, "src/app.txt": "app content\n"}
	prefix := repo.Name + "/"

	t.Run("zip", func(t *testing.T) {
		status, header, body := anonymous.doRaw(http.MethodGet, repoCodePage(repo)+"/archive/main.zip", "", nil)
		require.Equal(t, http.StatusOK, status, "response: %s", body)
		assert.Contains(t, header.Get("Content-Disposition"), repo.Name+"-main.zip")

		zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
		require.NoError(t, err)
		got := map[string]string{}
		for _, f := range zr.File {
			if strings.HasSuffix(f.Name, "/") {
				continue
			}
			rc, err := f.Open()
			require.NoError(t, err)
			content, err := io.ReadAll(rc)
			require.NoError(t, err)
			_ = rc.Close()
			got[strings.TrimPrefix(f.Name, prefix)] = string(content)
		}
		assert.Equal(t, want, got)
	})

	t.Run("tar.gz", func(t *testing.T) {
		status, _, body := anonymous.doRaw(http.MethodGet, repoCodePage(repo)+"/archive/main.tar.gz", "", nil)
		require.Equal(t, http.StatusOK, status, "response: %s", body)

		gz, err := gzip.NewReader(bytes.NewReader(body))
		require.NoError(t, err)
		tr := tar.NewReader(gz)
		got := map[string]string{}
		for {
			h, err := tr.Next()
			if err == io.EOF {
				break
			}
			require.NoError(t, err)
			if h.Typeflag != tar.TypeReg {
				continue
			}
			content, err := io.ReadAll(tr)
			require.NoError(t, err)
			got[strings.TrimPrefix(h.Name, prefix)] = string(content)
		}
		assert.Equal(t, want, got)
	})

	t.Run("not found", func(t *testing.T) {
		for _, path := range []string{
			repoCodePage(repo) + "/archive/nope.zip",
			repoCodePage(repo) + "/archive/main.rar",
			repoCodePage(private) + "/archive/main.zip",
		} {
			status, _, body := anonymous.doRaw(http.MethodGet, path, "", nil)
			assert.Equal(t, http.StatusNotFound, status, "%s: %.300s", path, body)
		}

		status, _, body := ownerClient.doRaw(http.MethodGet, repoCodePage(private)+"/archive/main.zip", "", nil)
		require.Equal(t, http.StatusOK, status, "response: %s", body)
		_, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
		assert.NoError(t, err)
	})
}

func TestRepoCodeGoGet(t *testing.T) {
	t.Parallel()

	owner := seedUser(t, CreateUserOptions{})
	// "go get" sends no credentials, so even a private repository answers.
	repo := seedRepository(t, owner.ID, CreateRepositoryOptions{Private: true, AutoInit: true})
	importPath := strings.TrimPrefix(serverURL, "http://") + repoCodePage(repo)

	status, _, body := newClient(t).doRaw(http.MethodGet, repoCodePage(repo)+"?go-get=1", "", nil)
	require.Equal(t, http.StatusOK, status, "response: %s", body)
	assert.Contains(t, string(body), `<meta name="go-import" content="`+importPath+` git `+serverURL+repoCodePage(repo)+`.git">`)
	assert.Contains(t, string(body), "go get --insecure "+importPath)

	t.Run("page", func(t *testing.T) {
		userAssertPage(t, newClient(t), repoCodePage(repo), http.StatusOK)
	})
}

// TestRepoCodeCommitMessageLinks checks that every page showing a commit
// message escapes it and links the users and issues it mentions, with the
// links the repository's issue tracker allows.
func TestRepoCodeCommitMessageLinks(t *testing.T) {
	t.Parallel()

	owner := seedUser(t, CreateUserOptions{})
	repo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
	ownerClient := authenticatedClient(t, owner)
	anonymous := newClient(t)
	api := repoCodeAPI(repo)

	status, body := ownerClient.do(http.MethodPost, api+"/editor/files", map[string]any{
		"branch":  "main",
		"path":    "notes.txt",
		"content": "Notes.\n",
		"summary": `Fix "quoted" <b> for @` + owner.Name + `, closes #1`,
		"message": "Thanks @" + owner.Name + "\nSee #2 and <script>alert(1)</script>\n\nSigned-off-by: Alice <alice@example.com>",
	})
	require.Equal(t, http.StatusOK, status, "response: %s", body)
	sha := repoCodeGit(t, repo, "rev-parse", "refs/heads/main")

	mention := `<a href="/` + owner.Name + `" rel="nofollow">@` + owner.Name + `</a>`
	issue := func(index string) string {
		return `<a href="` + repoCodePage(repo) + `/issues/` + index + `" rel="nofollow">#` + index + `</a>`
	}

	type commitResponse struct {
		Subject     string `json:"subject"`
		SubjectHTML string `json:"subjectHTML"`
		BodyHTML    string `json:"bodyHTML"`
	}
	getCommit := func(t *testing.T) commitResponse {
		t.Helper()

		var commit commitResponse
		anonymous.doJSON(http.MethodGet, api+"/commit/"+sha, nil, http.StatusOK, &commit)
		return commit
	}
	// subjects returns the subject of the commit as each page shows it: the
	// commit page, the commit list and the file list.
	subjects := func(t *testing.T) map[string]string {
		t.Helper()

		var commits struct {
			Commits []repoCodeCommitItem `json:"commits"`
		}
		anonymous.doJSON(http.MethodGet, api+"/commits/main", nil, http.StatusOK, &commits)
		require.NotEmpty(t, commits.Commits)
		require.Equal(t, sha, commits.Commits[0].SHA)

		var src struct {
			Entries []struct {
				Name   string             `json:"name"`
				Commit repoCodeCommitItem `json:"commit"`
			} `json:"entries"`
		}
		anonymous.doJSON(http.MethodGet, api+"/src", nil, http.StatusOK, &src)
		var entry *repoCodeCommitItem
		for i := range src.Entries {
			if src.Entries[i].Name == "notes.txt" {
				entry = &src.Entries[i].Commit
			}
		}
		require.NotNil(t, entry)
		require.Equal(t, sha, entry.SHA)

		return map[string]string{
			"commit page": getCommit(t).SubjectHTML,
			"commit list": commits.Commits[0].SubjectHTML,
			"file list":   entry.SubjectHTML,
		}
	}

	t.Run("internal tracker", func(t *testing.T) {
		commit := getCommit(t)
		assert.Equal(t, `Fix "quoted" <b> for @`+owner.Name+`, closes #1`, commit.Subject)
		assert.Equal(t,
			"Thanks "+mention+"\nSee "+issue("2")+" and &lt;script&gt;alert(1)&lt;/script&gt;\n\nSigned-off-by: Alice &lt;alice@example.com&gt;",
			commit.BodyHTML,
		)
		want := `Fix &#34;quoted&#34; &lt;b&gt; for ` + mention + `, closes ` + issue("1")
		for page, got := range subjects(t) {
			assert.Equal(t, want, got, page)
		}
	})

	setTrackerFormat := func(t *testing.T, format string) {
		t.Helper()

		status, body := ownerClient.do(http.MethodPut, repoSettingsAPIPath(repo, "/settings/advanced"), map[string]any{
			"enableWiki":            true,
			"enableIssues":          true,
			"enableExternalTracker": true,
			"externalTrackerURL":    "https://tracker.example.com",
			"externalTrackerFormat": format,
			"externalTrackerStyle":  "numeric",
			"enablePulls":           true,
			"pullsAllowMerge":       true,
		})
		require.Equal(t, http.StatusNoContent, status, "response: %s", body)
	}

	t.Run("external tracker", func(t *testing.T) {
		setTrackerFormat(t, "https://tracker.example.com/{user}/{repo}/{index}")
		want := `Fix &#34;quoted&#34; &lt;b&gt; for ` + mention + `, closes <a href="https://tracker.example.com/` + repo.Owner + `/` + repo.Name + `/1" rel="nofollow">#1</a>`
		for page, got := range subjects(t) {
			assert.Equal(t, want, got, page)
		}
	})

	t.Run("unsafe external tracker", func(t *testing.T) {
		// Repository admins can set any format, but links that run script are
		// dropped and leave the reference as text.
		setTrackerFormat(t, "javascript:alert(document.domain)//{index}")
		want := `Fix &#34;quoted&#34; &lt;b&gt; for ` + mention + `, closes #1`
		for page, got := range subjects(t) {
			assert.Equal(t, want, got, page)
		}
		commit := getCommit(t)
		assert.NotContains(t, strings.ToLower(commit.BodyHTML), "javascript")
		assert.Contains(t, commit.BodyHTML, "\nSee #2 and ")
	})
}
