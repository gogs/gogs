package repo

import (
	"encoding/base64"
	"fmt"
	"net/url"

	"github.com/gogs/git-module"

	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/db/errors"
)

type repoContents struct {
	Type            string `json:"type"`
	Target          string `json:"target,omitempty"`
	SubmoduleGitURL string `json:"submodule_git_url,omitempty"`
	Encoding        string `json:"encoding,omitempty"`
	Size            int64  `json:"size"`
	Name            string `json:"name"`
	Path            string `json:"path"`
	Content         string `json:"content,omitempty"`
	Sha             string `json:"sha"`
	URL             string `json:"url"`
	GitURL          string `json:"git_url"`
	HTMLURL         string `json:"html_url"`
	DownloadURL     string `json:"download_url"`
	Links           Links  `json:"_links"`
}

type Links struct {
	Git  string `json:"git"`
	Self string `json:"self"`
	HTML string `json:"html"`
}

func GetContents(c *context.APIContext) {
	treeEntry, err := c.Repo.Commit.GetTreeEntryByPath(c.Repo.TreePath)
	if err != nil {
		c.NotFoundOrServerError("GetTreeEntryByPath", git.IsErrNotExist, err)
		return
	}

	// TODO: figure out the best way to do this
	// :base-url/:username/:project/raw/:refs/:path
	templateDownloadURL := "%s/%s/%s/raw/%s"
	// :base-url/repos/:username/:project/contents/:path
	templateSelfLink := "%s/repos/%s/%s/contents/%s"
	// :baseurl/repos/:username/:project/git/trees/:sha
	templateGitURLLink := "%s/repos/%s/%s/trees/%s"
	// :baseurl/repos/:username/:project/tree/:sha
	templateHTMLLLink := "%s/repos/%s/%s/tree/%s"

	gitURL := fmt.Sprintf(templateGitURLLink, c.BaseURL, c.Params(":username"), c.Params(":reponame"), treeEntry.ID.String())
	htmlURL := fmt.Sprintf(templateHTMLLLink, c.BaseURL, c.Params(":username"), c.Params(":reponame"), treeEntry.ID.String())
	selfURL := fmt.Sprintf(templateSelfLink, c.BaseURL, c.Params(":username"), c.Params(":reponame"), c.Repo.TreePath)

	gr := &repoContents{
		DownloadURL: fmt.Sprintf(templateDownloadURL, c.BaseURL, c.Params(":username"), c.Params(":reponame"), c.Repo.TreePath),
		Size:        treeEntry.Size(),
		Name:        treeEntry.Name(),
		Path:        c.Repo.TreePath,
		Sha:         treeEntry.ID.String(),
		Links: Links{
			Git:  gitURL,
			Self: selfURL,
			HTML: htmlURL,
		},
		URL:     selfURL,
		GitURL:  gitURL,
		HTMLURL: htmlURL,
	}
	// A tree entry can only be one of the following types:
	// 1. Tree (Directory)
	// 2. SubModule
	// 3. SymLink
	// 4. Blob
	if treeEntry.IsSubModule() {
		gr.Type = "submodule"
		parsedURL, err := url.Parse(c.BaseURL)
		if err != nil {
			c.ServerError("ErrorURLParse", err)
		}
		host := parsedURL.Host
		submoduleURL := fmt.Sprintf("git://%s/%s/%s.git", host, c.Params(":name"), c.Params(":reponame"))
		gr.SubmoduleGitURL = submoduleURL
		c.JSONSuccess(gr)
		return

	} else if treeEntry.IsLink() {
		gr.Type = "symlink"
		gr.Target = c.Repo.TreePath
		c.JSONSuccess(gr)
		return

	} else if gr.Type == "blob" { // tree entry is a blob
		gr.Type = "blob"
		b, err := getBase64EncodedBlob(c)

		if err != nil {
			c.ServerError("GetBlobContent", err)
			return
		}

		gr.Content = b
		c.JSONSuccess(gr)
		return
	} else { // treeEntry is a directory
		dirTree, err := c.Repo.GitRepo.GetTree(treeEntry.ID.String())
		if err != nil {
			c.NotFoundOrServerError("GetGitDirTree", git.IsErrNotExist, err)
			return
		}

		entries, err := dirTree.ListEntries()
		if err != nil {
			c.NotFoundOrServerError("ListDirTreeEntries", git.IsErrNotExist, err)
			return
		}

		results, err := AppendDirTreeEntries(entries, c)

		if err != nil {
			c.NotFoundOrServerError("AppendDirTreeEntries", git.IsErrNotExist, err)
			return

		}
		c.JSONSuccess(results)
		return
	}
}

func getBase64EncodedBlob(c *context.APIContext) (string, error) {
	if c.Repo.Repository.IsBare {
		return "", errors.New("RepositoryIsBare")
	}

	blob, err := c.Repo.Commit.GetBlobByPath(c.Repo.TreePath)
	if err != nil {
		return "", errors.New("ErrorGetBlobByPath")
	}
	buf := make([]byte, 1024)
	b, err := blob.Data()
	if err != nil {
		return "", err
	}
	n, err := b.Read(buf)

	if err != nil {
		return "", err
	}
	if n >= 0 {
		buf = buf[:n]
	}
	return base64.StdEncoding.EncodeToString(buf), nil
}

func AppendDirTreeEntries(entries git.Entries, c *context.APIContext) ([]*repoContents, error) {
	var results = make([]*repoContents, 0, len(entries))
	if len(entries) == 0 {
		c.JSONSuccess(&repoGitTree{})
	}

	// TODO: figure out the best way to do this
	// :base-url/:username/:project/raw/:refs/:path
	templateDownloadURL := "%s/%s/%s/raw/%s"
	// :base-url/repos/:username/:project/contents/:path
	templateSelfLink := "%s/repos/%s/%s/contents/%s"
	// :baseurl/repos/:username/:project/git/trees/:sha
	templateGitURLLink := "%s/repos/%s/%s/trees/%s"
	// :baseurl/repos/:username/:project/tree/:sha
	templateHTMLLLink := "%s/repos/%s/%s/tree/%s"

	for _, entry := range entries {

		gitURL := fmt.Sprintf(templateGitURLLink, c.BaseURL, c.Params(":username"), c.Params(":reponame"), entry.ID.String())
		htmlURL := fmt.Sprintf(templateHTMLLLink, c.BaseURL, c.Params(":username"), c.Params(":reponame"), entry.ID.String())
		selfurl := fmt.Sprintf(templateSelfLink, c.BaseURL, c.Params(":username"), c.Params(":reponame"), c.Repo.TreePath)

		var contentType string
		if entry.IsDir() {
			contentType = "tree"
		} else if entry.IsSubModule() {
			contentType = "submodule"
		} else if entry.IsLink() {
			contentType = "symlink"
		} else {
			contentType = "blob"
		}

		results = append(results, &repoContents{
			DownloadURL: fmt.Sprintf(templateDownloadURL, c.BaseURL, c.Params(":username"), c.Params(":reponame"), c.Repo.TreePath),
			Type:        contentType,
			Size:        entry.Size(),
			Name:        entry.Name(),
			Path:        c.Repo.TreePath,
			Sha:         entry.ID.String(),
			Links: Links{
				Git:  gitURL,
				Self: selfurl,
				HTML: htmlURL,
			},
			URL:     selfurl,
			GitURL:  gitURL,
			HTMLURL: htmlURL,
		})
	}
	return results, nil
}
