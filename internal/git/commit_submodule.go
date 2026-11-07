package git

import (
	"bufio"
	"bytes"
	"context"
	"strings"
)

// Submodule contains information of a Git submodule.
type Submodule struct {
	// The name of the submodule.
	Name string
	// The URL of the submodule.
	URL string
	// The commit ID of the subproject.
	Commit string
}

// Submodules contains information of submodules.
type Submodules = *objectCache

// Submodules returns submodules found in this commit. Successful results are
// cached; failed attempts are not cached, allowing retries with a fresh context.
func (c *Commit) Submodules(ctx context.Context) (Submodules, error) {
	c.submodulesMu.Lock()
	defer c.submodulesMu.Unlock()

	if c.submodulesSet {
		return c.submodules, nil
	}

	e, err := c.TreeEntry(ctx, ".gitmodules")
	if err != nil {
		return nil, err
	}

	p, err := e.Blob().Bytes(ctx)
	if err != nil {
		return nil, err
	}

	scanner := bufio.NewScanner(bytes.NewReader(p))
	submodules := newObjectCache()
	var inSection bool
	var path string
	var url string
	for scanner.Scan() {
		if strings.HasPrefix(scanner.Text(), "[submodule") {
			inSection = true
			path = ""
			url = ""
			continue
		} else if !inSection {
			continue
		}

		key, value, ok := strings.Cut(scanner.Text(), "=")
		if !ok {
			continue
		}

		switch strings.TrimSpace(key) {
		case "path":
			path = strings.TrimSpace(value)
		case "url":
			url = strings.TrimSpace(value)
		}

		if len(path) > 0 && len(url) > 0 {
			mod := &Submodule{
				Name: path,
				URL:  url,
			}

			mod.Commit, err = c.repo.RevParse(ctx, c.id.String()+":"+mod.Name)
			if err != nil {
				return nil, err
			}

			submodules.Set(path, mod)
			inSection = false
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	c.submodules = submodules
	c.submodulesSet = true
	return c.submodules, nil
}

// Submodule returns submodule by given name. It returns an ErrSubmoduleNotExist
// if the path does not exist as a submodule.
func (c *Commit) Submodule(ctx context.Context, path string) (*Submodule, error) {
	mods, err := c.Submodules(ctx)
	if err != nil {
		return nil, err
	}

	m, has := mods.Get(path)
	if has {
		return m.(*Submodule), nil
	}
	return nil, ErrSubmoduleNotExist
}
