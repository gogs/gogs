package git

import (
	"bytes"
	"context"
	"io"
	"strings"
)

// LsRemoteOptions contains arguments for listing references in a remote
// repository.
//
// Docs: https://git-scm.com/docs/git-ls-remote
type LsRemoteOptions struct {
	// The list of patterns to filter results.
	Patterns []string
	// The additional environment variables to run the command with.
	Envs []string
}

// LsRemote returns a list of references in the remote repository.
func LsRemote(ctx context.Context, url string, opts ...LsRemoteOptions) ([]*Reference, error) {
	var opt LsRemoteOptions
	if len(opts) > 0 {
		opt = opts[0]
	}

	args := []string{"ls-remote", "--quiet"}
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

// RemoteHEADOptions contains arguments for getting the reference that HEAD of
// a remote repository points to.
//
// Docs: https://git-scm.com/docs/git-ls-remote#Documentation/git-ls-remote.txt---symref
type RemoteHEADOptions struct {
	// The additional environment variables to run the command with.
	Envs []string
}

// RemoteHEAD returns the reference that HEAD of the remote repository points
// to, e.g., "refs/heads/main".
func RemoteHEAD(ctx context.Context, url string, opts ...RemoteHEADOptions) (string, error) {
	var opt RemoteHEADOptions
	if len(opts) > 0 {
		opt = opts[0]
	}

	stdout, err := exec(ctx, "", []string{"ls-remote", "--quiet", "--symref", "--end-of-options", url, "HEAD"}, opt.Envs)
	if err != nil {
		return "", err
	}

	for line := range bytes.SplitSeq(stdout, []byte("\n")) {
		ref, ok := bytes.CutPrefix(line, []byte("ref: "))
		if !ok {
			continue
		}
		// The "HEAD" pattern also matches references like "refs/remotes/origin/HEAD".
		target, name, _ := bytes.Cut(ref, []byte("\t"))
		if string(name) != "HEAD" {
			continue
		}
		return string(target), nil
	}
	return "", ErrNotSymbolicRef
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

	_, err := exec(ctx, r.path, args, nil)
	return err
}

// RemoteRemove removes a remote from the repository.
func (r *Repository) RemoteRemove(ctx context.Context, name string) error {
	args := []string{"remote", "remove", "--end-of-options", name}
	_, err := exec(ctx, r.path, args, nil)
	if err != nil {
		if strings.Contains(err.Error(), "error: No such remote") ||
			strings.Contains(err.Error(), "fatal: No such remote") {
			return ErrRemoteNotExist
		}
		return err
	}
	return nil
}

// RemoteUpdateOptions contains optional arguments for fetching updates from all
// remotes of the repository.
//
// Docs: https://git-scm.com/docs/git-remote#Documentation/git-remote.txt-update
type RemoteUpdateOptions struct {
	// Indicates whether to remove references that no longer exist on the remotes.
	Prune bool
	// The additional environment variables to run the command with.
	Envs []string
}

// RemoteUpdate fetches updates from all remotes of the repository. It returns
// the standard error output, which is where Git reports updated references.
func (r *Repository) RemoteUpdate(ctx context.Context, opts ...RemoteUpdateOptions) (string, error) {
	var opt RemoteUpdateOptions
	if len(opts) > 0 {
		opt = opts[0]
	}

	args := []string{"remote", "update"}
	if opt.Prune {
		args = append(args, "--prune")
	}

	stderr, err := run(ctx, r.path, args, opt.Envs, io.Discard)
	if err != nil {
		return "", err
	}
	return string(stderr), nil
}

// RepoRemoteUpdate is like Repository.RemoteUpdate for the repository in given path.
func RepoRemoteUpdate(ctx context.Context, repoPath string, opts ...RemoteUpdateOptions) (string, error) {
	r, err := Open(repoPath)
	if err != nil {
		return "", err
	}
	return r.RemoteUpdate(ctx, opts...)
}

// RemoteSetURL sets the first URL of the remote with given name of the
// repository.
func (r *Repository) RemoteSetURL(ctx context.Context, name, newurl string) error {
	args := []string{"remote", "set-url", "--end-of-options", name, newurl}
	_, err := exec(ctx, r.path, args, nil)
	if err != nil {
		if strings.Contains(err.Error(), "No such remote") {
			return ErrRemoteNotExist
		}
		return err
	}
	return nil
}
