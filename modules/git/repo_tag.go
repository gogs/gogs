// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package git

import (
	"errors"
	"strings"

	"github.com/Unknwon/com"
)

func IsTagExist(repoPath, tagName string) bool {
	_, _, err := com.ExecCmdDir(repoPath, "git", "show-ref", "--verify", "refs/tags/"+tagName)
	return err == nil
}

func (repo *Repository) IsTagExist(tagName string) bool {
	return IsTagExist(repo.Path, tagName)
}

// GetTags returns all tags of given repository.
func (repo *Repository) GetTags() ([]string, error) {
	if gitVer.AtLeast(MustParseVersion("2.0.0")) {
		return repo.getTagsReversed()
	}
	stdout, stderr, err := com.ExecCmdDir(repo.Path, "git", "tag", "-l")
	if err != nil {
		return nil, errors.New(stderr)
	}
	tags := strings.Split(stdout, "\n")
	return tags[:len(tags)-1], nil
}

func (repo *Repository) getTagsReversed() ([]string, error) {
	stdout, stderr, err := com.ExecCmdDir(repo.Path, "git", "tag", "-l", "--sort=-v:refname")
	if err != nil {
		return nil, errors.New(stderr)
	}
	tags := strings.Split(stdout, "\n")
	return tags[:len(tags)-1], nil
}

func (repo *Repository) CreateTag(tagName, idStr string) error {
	_, stderr, err := com.ExecCmdDir(repo.Path, "git", "tag", tagName, idStr)
	if err != nil {
		return errors.New(stderr)
	}
	return nil
}

func (repo *Repository) getTag(id sha1) (*Tag, error) {
	if repo.tagCache != nil {
		if t, ok := repo.tagCache[id]; ok {
			return t, nil
		}
	} else {
		repo.tagCache = make(map[sha1]*Tag, 10)
	}

	// Get tag type.
	tp, stderr, err := com.ExecCmdDir(repo.Path, "git", "cat-file", "-t", id.String())
	if err != nil {
		return nil, errors.New(stderr)
	}
	tp = strings.TrimSpace(tp)

	// Tag is a commit.
	if ObjectType(tp) == COMMIT {
		tag := &Tag{
			Id:     id,
			Object: id,
			Type:   string(COMMIT),
			repo:   repo,
		}
		repo.tagCache[id] = tag
		return tag, nil
	}

	// Tag with message.
	data, bytErr, err := com.ExecCmdDirBytes(repo.Path, "git", "cat-file", "-p", id.String())
	if err != nil {
		return nil, errors.New(string(bytErr))
	}

	tag, err := parseTagData(data)
	if err != nil {
		return nil, err
	}

	tag.Id = id
	tag.repo = repo

	repo.tagCache[id] = tag
	return tag, nil
}

// GetTag returns a Git tag by given name.
func (repo *Repository) GetTag(tagName string) (*Tag, error) {
	stdout, stderr, err := com.ExecCmdDir(repo.Path, "git", "show-ref", "--tags", tagName)
	if err != nil {
		return nil, errors.New(stderr)
	}

	id, err := NewIdFromString(strings.Split(stdout, " ")[0])
	if err != nil {
		return nil, err
	}

	tag, err := repo.getTag(id)
	if err != nil {
		return nil, err
	}
	tag.Name = tagName
	return tag, nil
}
