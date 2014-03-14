package models

import (
	"path"
	"time"

	git "github.com/speedata/gogit"
)

type RepoFile struct {
	Id      *git.Oid
	Type    int
	Name    string
	Path    string
	Message string
	Created time.Time
}

func (f *RepoFile) IsFile() bool {
	return f.Type == git.FileModeBlob || f.Type == git.FileModeBlobExec
}

func (f *RepoFile) IsDir() bool {
	return f.Type == git.FileModeTree
}

func GetReposFiles(userName, reposName, branchName, rpath string) ([]*RepoFile, error) {
	f := RepoPath(userName, reposName)

	repo, err := git.OpenRepository(f)
	if err != nil {
		return nil, err
	}

	ref, err := repo.LookupReference("refs/heads/" + branchName)
	if err != nil {
		return nil, err
	}

	lastCommit, err := repo.LookupCommit(ref.Oid)
	if err != nil {
		return nil, err
	}

	var repofiles []*RepoFile
	lastCommit.Tree.Walk(func(dirname string, entry *git.TreeEntry) int {
		if dirname == rpath {
			repofiles = append(repofiles, &RepoFile{
				entry.Id,
				entry.Filemode,
				entry.Name,
				path.Join(dirname, entry.Name),
				lastCommit.Message(),
				lastCommit.Committer.When,
			})
		}
		return 0
	})

	return repofiles, nil
}
