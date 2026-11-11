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

func (ta *Tag) GetCommit(ctx context.Context) (*git.Commit, error) {
	return git.RepoTagCommit(ctx, ta.RepoPath, ta.Name)
}

func GetTagsByPath(ctx context.Context, path string) ([]*Tag, error) {
	names, err := git.RepoTags(ctx, path)
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

func (r *Repository) GetTags(ctx context.Context) ([]*Tag, error) {
	return GetTagsByPath(ctx, r.RepoPath())
}
