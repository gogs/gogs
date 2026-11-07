package git

import (
	"bytes"
	"context"
	"strings"
)

// LsRemoteOptions contains arguments for listing references in a remote
// repository.
//
// Docs: https://git-scm.com/docs/git-ls-remote
type LsRemoteOptions struct {
	// Indicates whether include heads.
	Heads bool
	// Indicates whether include tags.
	Tags bool
	// Indicates whether to not show peeled tags or pseudo refs.
	Refs bool
	// The list of patterns to filter results.
	Patterns []string
	CommandOptions
}

// LsRemote returns a list of references in the remote repository.
func LsRemote(ctx context.Context, url string, opts ...LsRemoteOptions) ([]*Reference, error) {
	var opt LsRemoteOptions
	if len(opts) > 0 {
		opt = opts[0]
	}

	args := []string{"ls-remote", "--quiet"}
	if opt.Heads {
		args = append(args, "--heads")
	}
	if opt.Tags {
		args = append(args, "--tags")
	}
	if opt.Refs {
		args = append(args, "--refs")
	}
	args = append(args, "--end-of-options", url)
	if len(opt.Patterns) > 0 {
		args = append(args, opt.Patterns...)
	}

	stdout, err := exec(ctx, "", args, opt.Envs)
	if err != nil {
		return nil, err
	}

	lines := bytes.Split(stdout, []byte("\n"))
	refs := make([]*Reference, 0, len(lines))
	for i := range lines {
		fields := bytes.Fields(lines[i])
		if len(fields) < 2 {
			continue
		}

		refs = append(refs, &Reference{
			ID:      string(fields[0]),
			Refspec: string(fields[1]),
		})
	}
	return refs, nil
}

// IsURLAccessible returns true if given remote URL is accessible via Git. The
// caller should use context.WithTimeout to control the timeout.
func IsURLAccessible(ctx context.Context, url string) bool {
	_, err := LsRemote(ctx, url, LsRemoteOptions{
		Patterns: []string{"HEAD"},
	})
	return err == nil
}

// RemoteAddOptions contains options to add a remote address.
//
// Docs: https://git-scm.com/docs/git-remote#Documentation/git-remote.txt-emaddem
type RemoteAddOptions struct {
	// Indicates whether to execute git fetch after the remote information is set
	// up.
	Fetch bool
	// Indicates whether to add remote as mirror with --mirror=fetch.
	MirrorFetch bool
	CommandOptions
}

// RemoteAdd adds a new remote to the repository.
func (r *Repository) RemoteAdd(ctx context.Context, name, url string, opts ...RemoteAddOptions) error {
	var opt RemoteAddOptions
	if len(opts) > 0 {
		opt = opts[0]
	}

	args := []string{"remote", "add"}
	if opt.Fetch {
		args = append(args, "-f")
	}
	if opt.MirrorFetch {
		args = append(args, "--mirror=fetch")
	}
	args = append(args, "--end-of-options", name, url)

	_, err := exec(ctx, r.path, args, opt.Envs)
	return err
}

// RemoteRemoveOptions contains arguments for removing a remote from the
// repository.
//
// Docs: https://git-scm.com/docs/git-remote#Documentation/git-remote.txt-emremoveem
type RemoteRemoveOptions struct {
	CommandOptions
}

// RemoteRemove removes a remote from the repository.
func (r *Repository) RemoteRemove(ctx context.Context, name string, opts ...RemoteRemoveOptions) error {
	var opt RemoteRemoveOptions
	if len(opts) > 0 {
		opt = opts[0]
	}

	args := []string{"remote", "remove", "--end-of-options", name}
	_, err := exec(ctx, r.path, args, opt.Envs)
	if err != nil {
		if strings.Contains(err.Error(), "error: No such remote") ||
			strings.Contains(err.Error(), "fatal: No such remote") {
			return ErrRemoteNotExist
		}
		return err
	}
	return nil
}

// RemotesOptions contains arguments for listing remotes of the repository.
// /
// Docs: https://git-scm.com/docs/git-remote#_commands
type RemotesOptions struct {
	CommandOptions
}

// Remotes lists remotes of the repository.
func (r *Repository) Remotes(ctx context.Context, opts ...RemotesOptions) ([]string, error) {
	var opt RemotesOptions
	if len(opts) > 0 {
		opt = opts[0]
	}

	args := []string{"remote", "--end-of-options"}
	stdout, err := exec(ctx, r.path, args, opt.Envs)
	if err != nil {
		return nil, err
	}

	return bytesToStrings(stdout), nil
}

// RemoteGetURLOptions contains arguments for retrieving URL(s) of a remote of
// the repository.
//
// Docs: https://git-scm.com/docs/git-remote#Documentation/git-remote.txt-emget-urlem
type RemoteGetURLOptions struct {
	// Indicates whether to get push URLs instead of fetch URLs.
	Push bool
	// Indicates whether to get all URLs, including lists that are not part of main
	// URLs. This option is independent of the Push option.
	All bool
	CommandOptions
}

// RemoteGetURL retrieves URL(s) of a remote of the repository.
func (r *Repository) RemoteGetURL(ctx context.Context, name string, opts ...RemoteGetURLOptions) ([]string, error) {
	var opt RemoteGetURLOptions
	if len(opts) > 0 {
		opt = opts[0]
	}

	args := []string{"remote", "get-url"}
	if opt.Push {
		args = append(args, "--push")
	}
	if opt.All {
		args = append(args, "--all")
	}
	args = append(args, "--end-of-options", name)

	stdout, err := exec(ctx, r.path, args, opt.Envs)
	if err != nil {
		return nil, err
	}
	return bytesToStrings(stdout), nil
}

// RemoteSetURLOptions contains arguments for setting an URL of a remote of the
// repository.
//
// Docs: https://git-scm.com/docs/git-remote#Documentation/git-remote.txt-emset-urlem
type RemoteSetURLOptions struct {
	// Indicates whether to get push URLs instead of fetch URLs.
	Push bool
	// The regex to match existing URLs to replace (instead of first).
	Regex string
	CommandOptions
}

// RemoteSetURL sets the first URL of the remote with given name of the
// repository.
func (r *Repository) RemoteSetURL(ctx context.Context, name, newurl string, opts ...RemoteSetURLOptions) error {
	var opt RemoteSetURLOptions
	if len(opts) > 0 {
		opt = opts[0]
	}

	args := []string{"remote", "set-url"}
	if opt.Push {
		args = append(args, "--push")
	}
	args = append(args, "--end-of-options", name, newurl)
	if opt.Regex != "" {
		args = append(args, opt.Regex)
	}

	_, err := exec(ctx, r.path, args, opt.Envs)
	if err != nil {
		if strings.Contains(err.Error(), "No such URL found") {
			return ErrURLNotExist
		} else if strings.Contains(err.Error(), "No such remote") {
			return ErrRemoteNotExist
		}
		return err
	}
	return nil
}

// RemoteSetURLAddOptions contains arguments for appending an URL to a remote
// of the repository.
//
// Docs: https://git-scm.com/docs/git-remote#Documentation/git-remote.txt-emset-urlem
type RemoteSetURLAddOptions struct {
	// Indicates whether to get push URLs instead of fetch URLs.
	Push bool
	CommandOptions
}

// RemoteSetURLAdd appends an URL to the remote with given name of the
// repository. Use RemoteSetURL to overwrite the URL(s) instead.
func (r *Repository) RemoteSetURLAdd(ctx context.Context, name, newurl string, opts ...RemoteSetURLAddOptions) error {
	var opt RemoteSetURLAddOptions
	if len(opts) > 0 {
		opt = opts[0]
	}

	args := []string{"remote", "set-url", "--add"}
	if opt.Push {
		args = append(args, "--push")
	}
	args = append(args, "--end-of-options", name, newurl)

	_, err := exec(ctx, r.path, args, opt.Envs)
	if err != nil && strings.Contains(err.Error(), "Will not delete all non-push URLs") {
		return ErrNotDeleteNonPushURLs
	}
	return err
}

// RemoteSetURLDeleteOptions contains arguments for deleting an URL of a remote
// of the repository.
//
// Docs: https://git-scm.com/docs/git-remote#Documentation/git-remote.txt-emset-urlem
type RemoteSetURLDeleteOptions struct {
	// Indicates whether to get push URLs instead of fetch URLs.
	Push bool
	CommandOptions
}

// RemoteSetURLDelete deletes all URLs matching regex of the remote with given
// name of the repository.
func (r *Repository) RemoteSetURLDelete(ctx context.Context, name, regex string, opts ...RemoteSetURLDeleteOptions) error {
	var opt RemoteSetURLDeleteOptions
	if len(opts) > 0 {
		opt = opts[0]
	}

	args := []string{"remote", "set-url", "--delete"}
	if opt.Push {
		args = append(args, "--push")
	}
	args = append(args, "--end-of-options", name, regex)

	_, err := exec(ctx, r.path, args, opt.Envs)
	if err != nil && strings.Contains(err.Error(), "Will not delete all non-push URLs") {
		return ErrNotDeleteNonPushURLs
	}
	return err
}
