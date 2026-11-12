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
	return git.RepoTagCommit(context.TODO(), ta.RepoPath, ta.Name)
}

func GetTagsByPath(path string) ([]*Tag, error) {
	names, err := git.RepoTags(context.TODO(), path)
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
