package models

import (
	"github.com/gogits/git-module"
)

type Branch struct {
	Path		string
	Name      	string
}

func GetBranchesByPath(path string) ([]*Branch, error) {
	gitRepo, err := git.OpenRepository(path)
	if err != nil {
		return nil, err
	}

	brs, err := gitRepo.GetBranches()
	if err != nil {
		return nil, err
	}

	Branches := make([]*Branch, len(brs))
	for i := range brs {
		Branches[i] = &Branch{
			Path: path,
			Name: brs[i],
		}
	}
	return Branches, nil
}

func GetBranchesByRepo(user,repo string) ([]*Branch, error) {
	return GetBranchesByPath(RepoPath(user, repo))
}

func (br *Branch) GetCommit() (*git.Commit, error) {
	gitRepo, err := git.OpenRepository(br.Path)
	if err != nil {
		return nil, err
	}
	return gitRepo.GetBranchCommit(br.Name)
}
