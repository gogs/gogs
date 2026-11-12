package database

import (
	"context"

	"github.com/cockroachdb/errors"

	"gogs.io/gogs/internal/git"
)

type Tag struct {
	RepoPath string
	Name     string

	IsProtected bool
	Commit      *git.Commit
}

func (ta *Tag) GetCommit() (*git.Commit, error) {
	gitRepo, err := git.Open(ta.RepoPath)
	if err != nil {
		return nil, errors.Newf("open repository: %v", err)
	}
	return gitRepo.TagCommit(context.TODO(), ta.Name)
}

func GetTagsByPath(path string) ([]*Tag, error) {
	gitRepo, err := git.Open(path)
	if err != nil {
		return nil, errors.Newf("open repository: %v", err)
	}

	names, err := gitRepo.Tags(context.TODO())
	if err != nil {
		return nil, errors.Newf("list tags: %v", err)
	}

	tags := make([]*Tag, len(names))
	for i := range names {
		tags[i] = &Tag{
			RepoPath: path,
			Name:     names[i],
		}
	}
	return tags, nil
}

func (r *Repository) GetTags() ([]*Tag, error) {
	return GetTagsByPath(r.RepoPath())
}
