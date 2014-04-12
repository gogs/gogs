// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

import (
	"bufio"
	"bytes"
	"container/list"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"strings"

	"github.com/Unknwon/com"

	"github.com/gogits/git"

	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/log"
)

// RepoFile represents a file object in git repository.
type RepoFile struct {
	*git.TreeEntry
	Path   string
	Size   int64
	Repo   *git.Repository
	Commit *git.Commit
}

// LookupBlob returns the content of an object.
func (file *RepoFile) LookupBlob() (*git.Blob, error) {
	if file.Repo == nil {
		return nil, ErrRepoFileNotLoaded
	}

	return file.Repo.LookupBlob(file.Id)
}

// GetBranches returns all branches of given repository.
func GetBranches(userName, repoName string) ([]string, error) {
	repo, err := git.OpenRepository(RepoPath(userName, repoName))
	if err != nil {
		return nil, err
	}

	refs, err := repo.AllReferences()
	if err != nil {
		return nil, err
	}

	brs := make([]string, len(refs))
	for i, ref := range refs {
		brs[i] = ref.BranchName()
	}
	return brs, nil
}

// GetTags returns all tags of given repository.
func GetTags(userName, repoName string) ([]string, error) {
	repo, err := git.OpenRepository(RepoPath(userName, repoName))
	if err != nil {
		return nil, err
	}

	refs, err := repo.AllTags()
	if err != nil {
		return nil, err
	}

	tags := make([]string, len(refs))
	for i, ref := range refs {
		tags[i] = ref.Name
	}
	return tags, nil
}

func IsBranchExist(userName, repoName, branchName string) bool {
	repo, err := git.OpenRepository(RepoPath(userName, repoName))
	if err != nil {
		return false
	}
	return repo.IsBranchExist(branchName)
}

func GetTargetFile(userName, repoName, branchName, commitId, rpath string) (*RepoFile, error) {
	repo, err := git.OpenRepository(RepoPath(userName, repoName))
	if err != nil {
		return nil, err
	}

	commit, err := repo.GetCommitOfBranch(branchName)
	if err != nil {
		commit, err = repo.GetCommit(commitId)
		if err != nil {
			return nil, err
		}
	}

	parts := strings.Split(path.Clean(rpath), "/")

	var entry *git.TreeEntry
	tree := commit.Tree
	for i, part := range parts {
		if i == len(parts)-1 {
			entry = tree.EntryByName(part)
			if entry == nil {
				return nil, ErrRepoFileNotExist
			}
		} else {
			tree, err = repo.SubTree(tree, part)
			if err != nil {
				return nil, err
			}
		}
	}

	size, err := repo.ObjectSize(entry.Id)
	if err != nil {
		return nil, err
	}

	repoFile := &RepoFile{
		entry,
		rpath,
		size,
		repo,
		commit,
	}

	return repoFile, nil
}

// GetReposFiles returns a list of file object in given directory of repository.
// func GetReposFilesOfBranch(userName, repoName, branchName, rpath string) ([]*RepoFile, error) {
// 	return getReposFiles(userName, repoName, commitId, rpath)
// }

// GetReposFiles returns a list of file object in given directory of repository.
func GetReposFiles(userName, repoName, commitId, rpath string) ([]*RepoFile, error) {
	return getReposFiles(userName, repoName, commitId, rpath)
}

func getReposFiles(userName, repoName, commitId string, rpath string) ([]*RepoFile, error) {
	repopath := RepoPath(userName, repoName)
	repo, err := git.OpenRepository(repopath)
	if err != nil {
		return nil, err
	}

	commit, err := repo.GetCommit(commitId)
	if err != nil {
		return nil, err
	}

	var repodirs []*RepoFile
	var repofiles []*RepoFile
	commit.Tree.Walk(func(dirname string, entry *git.TreeEntry) int {
		if dirname == rpath {
			// TODO: size get method shoule be improved
			size, err := repo.ObjectSize(entry.Id)
			if err != nil {
				return 0
			}

			stdout, _, err := com.ExecCmdDir(repopath, "git", "log", "-1", "--pretty=format:%H", commitId, "--", path.Join(dirname, entry.Name))
			if err != nil {
				return 0
			}
			filecm, err := repo.GetCommit(string(stdout))
			if err != nil {
				return 0
			}

			rp := &RepoFile{
				entry,
				path.Join(dirname, entry.Name),
				size,
				repo,
				filecm,
			}

			if entry.IsFile() {
				repofiles = append(repofiles, rp)
			} else if entry.IsDir() {
				repodirs = append(repodirs, rp)
			}
		}
		return 0
	})

	return append(repodirs, repofiles...), nil
}

func GetCommit(userName, repoName, commitId string) (*git.Commit, error) {
	repo, err := git.OpenRepository(RepoPath(userName, repoName))
	if err != nil {
		return nil, err
	}

	return repo.GetCommit(commitId)
}

// GetCommitsByBranch returns all commits of given branch of repository.
func GetCommitsByBranch(userName, repoName, branchName string) (*list.List, error) {
	repo, err := git.OpenRepository(RepoPath(userName, repoName))
	if err != nil {
		return nil, err
	}
	r, err := repo.LookupReference(fmt.Sprintf("refs/heads/%s", branchName))
	if err != nil {
		return nil, err
	}
	return r.AllCommits()
}

// GetCommitsByCommitId returns all commits of given commitId of repository.
func GetCommitsByCommitId(userName, repoName, commitId string) (*list.List, error) {
	repo, err := git.OpenRepository(RepoPath(userName, repoName))
	if err != nil {
		return nil, err
	}
	oid, err := git.NewOidFromString(commitId)
	if err != nil {
		return nil, err
	}
	return repo.CommitsBefore(oid)
}

// Diff line types.
const (
	DIFF_LINE_PLAIN = iota + 1
	DIFF_LINE_ADD
	DIFF_LINE_DEL
	DIFF_LINE_SECTION
)

const (
	DIFF_FILE_ADD = iota + 1
	DIFF_FILE_CHANGE
	DIFF_FILE_DEL
)

type DiffLine struct {
	LeftIdx  int
	RightIdx int
	Type     int
	Content  string
}

func (d DiffLine) GetType() int {
	return d.Type
}

type DiffSection struct {
	Name  string
	Lines []*DiffLine
}

type DiffFile struct {
	Name               string
	Addition, Deletion int
	Type               int
	Sections           []*DiffSection
}

type Diff struct {
	TotalAddition, TotalDeletion int
	Files                        []*DiffFile
}

func (diff *Diff) NumFiles() int {
	return len(diff.Files)
}

const DIFF_HEAD = "diff --git "

func ParsePatch(reader io.Reader) (*Diff, error) {
	scanner := bufio.NewScanner(reader)
	var (
		curFile    *DiffFile
		curSection = &DiffSection{
			Lines: make([]*DiffLine, 0, 10),
		}

		leftLine, rightLine int
	)

	diff := &Diff{Files: make([]*DiffFile, 0)}
	var i int
	for scanner.Scan() {
		line := scanner.Text()
		// fmt.Println(i, line)
		if strings.HasPrefix(line, "+++ ") || strings.HasPrefix(line, "--- ") {
			continue
		}

		i = i + 1

		// Diff data too large.
		if i == 5000 {
			log.Warn("Diff data too large")
			return &Diff{}, nil
		}

		if line == "" {
			continue
		}
		if line[0] == ' ' {
			diffLine := &DiffLine{Type: DIFF_LINE_PLAIN, Content: line, LeftIdx: leftLine, RightIdx: rightLine}
			leftLine++
			rightLine++
			curSection.Lines = append(curSection.Lines, diffLine)
			continue
		} else if line[0] == '@' {
			curSection = &DiffSection{}
			curFile.Sections = append(curFile.Sections, curSection)
			ss := strings.Split(line, "@@")
			diffLine := &DiffLine{Type: DIFF_LINE_SECTION, Content: line}
			curSection.Lines = append(curSection.Lines, diffLine)

			// Parse line number.
			ranges := strings.Split(ss[len(ss)-2][1:], " ")
			leftLine, _ = base.StrTo(strings.Split(ranges[0], ",")[0][1:]).Int()
			rightLine, _ = base.StrTo(strings.Split(ranges[1], ",")[0]).Int()
			continue
		} else if line[0] == '+' {
			curFile.Addition++
			diff.TotalAddition++
			diffLine := &DiffLine{Type: DIFF_LINE_ADD, Content: line, RightIdx: rightLine}
			rightLine++
			curSection.Lines = append(curSection.Lines, diffLine)
			continue
		} else if line[0] == '-' {
			curFile.Deletion++
			diff.TotalDeletion++
			diffLine := &DiffLine{Type: DIFF_LINE_DEL, Content: line, LeftIdx: leftLine}
			if leftLine > 0 {
				leftLine++
			}
			curSection.Lines = append(curSection.Lines, diffLine)
			continue
		}

		// Get new file.
		if strings.HasPrefix(line, DIFF_HEAD) {
			fs := strings.Split(line[len(DIFF_HEAD):], " ")
			a := fs[0]

			curFile = &DiffFile{
				Name:     a[strings.Index(a, "/")+1:],
				Type:     DIFF_FILE_CHANGE,
				Sections: make([]*DiffSection, 0, 10),
			}
			diff.Files = append(diff.Files, curFile)

			// Check file diff type.
			for scanner.Scan() {
				switch {
				case strings.HasPrefix(scanner.Text(), "new file"):
					curFile.Type = DIFF_FILE_ADD
				case strings.HasPrefix(scanner.Text(), "deleted"):
					curFile.Type = DIFF_FILE_DEL
				case strings.HasPrefix(scanner.Text(), "index"):
					curFile.Type = DIFF_FILE_CHANGE
				}
				if curFile.Type > 0 {
					break
				}
			}
		}
	}

	return diff, nil
}

func GetDiff(repoPath, commitid string) (*Diff, error) {
	repo, err := git.OpenRepository(repoPath)
	if err != nil {
		return nil, err
	}

	commit, err := repo.GetCommit(commitid)
	if err != nil {
		return nil, err
	}

	// First commit of repository.
	if commit.ParentCount() == 0 {
		rd, wr := io.Pipe()
		go func() {
			cmd := exec.Command("git", "show", commitid)
			cmd.Dir = repoPath
			cmd.Stdout = wr
			cmd.Stdin = os.Stdin
			cmd.Stderr = os.Stderr
			cmd.Run()
			wr.Close()
		}()
		defer rd.Close()
		return ParsePatch(rd)
	}

	rd, wr := io.Pipe()
	go func() {
		cmd := exec.Command("git", "diff", commit.Parent(0).Oid.String(), commitid)
		cmd.Dir = repoPath
		cmd.Stdout = wr
		cmd.Stdin = os.Stdin
		cmd.Stderr = os.Stderr
		cmd.Run()
		wr.Close()
	}()
	defer rd.Close()
	return ParsePatch(rd)
}

const prettyLogFormat = `--pretty=format:%H%n%an <%ae> %at%n%s`

func parsePrettyFormatLog(logByts []byte) (*list.List, error) {
	l := list.New()
	buf := bytes.NewBuffer(logByts)
	if buf.Len() == 0 {
		return l, nil
	}

	idx := 0
	var commit *git.Commit

	for {
		line, err := buf.ReadString('\n')
		if err != nil && err != io.EOF {
			return nil, err
		}
		line = strings.TrimSpace(line)
		// fmt.Println(line)

		var parseErr error
		switch idx {
		case 0: // SHA1.
			commit = &git.Commit{}
			commit.Oid, parseErr = git.NewOidFromString(line)
		case 1: // Signature.
			commit.Author, parseErr = git.NewSignatureFromCommitline([]byte(line + " "))
		case 2: // Commit message.
			commit.CommitMessage = line
			l.PushBack(commit)
			idx = -1
		}

		if parseErr != nil {
			return nil, parseErr
		}

		idx++

		if err == io.EOF {
			break
		}
	}

	return l, nil
}

// SearchCommits searches commits in given branch and keyword of repository.
func SearchCommits(repoPath, branch, keyword string) (*list.List, error) {
	stdout, stderr, err := com.ExecCmdDirBytes(repoPath, "git", "log", branch, "-100",
		"-i", "--grep="+keyword, prettyLogFormat)
	if err != nil {
		return nil, err
	} else if len(stderr) > 0 {
		return nil, errors.New(string(stderr))
	}
	return parsePrettyFormatLog(stdout)
}

// GetCommitsByRange returns certain number of commits with given page of repository.
func GetCommitsByRange(repoPath, branch string, page int) (*list.List, error) {
	stdout, stderr, err := com.ExecCmdDirBytes(repoPath, "git", "log", branch,
		"--skip="+base.ToStr((page-1)*50), "--max-count=50", prettyLogFormat)
	if err != nil {
		return nil, err
	} else if len(stderr) > 0 {
		return nil, errors.New(string(stderr))
	}
	return parsePrettyFormatLog(stdout)
}

// GetCommitsCount returns the commits count of given branch of repository.
func GetCommitsCount(repoPath, branch string) (int, error) {
	stdout, stderr, err := com.ExecCmdDir(repoPath, "git", "rev-list", "--count", branch)
	if err != nil {
		return 0, err
	} else if len(stderr) > 0 {
		return 0, errors.New(stderr)
	}
	return base.StrTo(strings.TrimSpace(stdout)).Int()
}
