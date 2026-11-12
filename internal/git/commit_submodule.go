package git

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"net/url"
	"strings"

	"gogs.io/gogs/internal/lazyregexp"
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
// cached. Failed attempts are not cached, allowing retries with a fresh context.
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

var scpSyntax = lazyregexp.New(`^([a-zA-Z0-9_]+@)?([a-zA-Z0-9._-]+):(.*)$`)

// InferSubmoduleURL returns the inferred external URL of the submodule at best effort.
// The `baseURL` should be the URL of the current repository. If the submodule URL looks
// like a relative path, it assumes that the submodule is another repository on the same
// Gogs instance by appending it to the `baseURL` with the commit.
func InferSubmoduleURL(baseURL string, mod *Submodule) string {
	if !strings.HasSuffix(baseURL, "/") {
		baseURL += "/"
	}

	raw := strings.TrimSuffix(mod.URL, "/")
	raw = strings.TrimSuffix(raw, ".git")

	if strings.HasPrefix(raw, "../") {
		return fmt.Sprintf("%s%s/commit/%s", baseURL, raw, mod.Commit)
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		// Try parsing as SCP syntax.
		match := scpSyntax.FindAllStringSubmatch(raw, -1)
		if len(match) == 0 {
			return mod.URL
		}
		parsed = &url.URL{
			Scheme: "http",
			Host:   match[0][2],
			Path:   match[0][3],
		}
	}

	switch parsed.Scheme {
	case "http", "https":
		raw = parsed.String()
	case "ssh":
		raw = fmt.Sprintf("http://%s%s", parsed.Hostname(), parsed.Path)
	default:
		return raw
	}

	return fmt.Sprintf("%s/commit/%s", raw, mod.Commit)
}
