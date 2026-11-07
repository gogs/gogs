package git

import (
	"context"
	"strings"
)

// MergeBaseOptions contains optional arguments for getting merge base.
//
// Docs: https://git-scm.com/docs/git-merge-base
type MergeBaseOptions struct {
	CommandOptions
}

// MergeBase returns merge base between base and head revisions of the
// repository.
func (r *Repository) MergeBase(ctx context.Context, base, head string, opts ...MergeBaseOptions) (string, error) {
	var opt MergeBaseOptions
	if len(opts) > 0 {
		opt = opts[0]
	}

	args := []string{"merge-base", "--end-of-options", base, head}
	stdout, err := exec(ctx, r.path, args, opt.Envs)
	if err != nil {
		if isExitStatus(err, 1) {
			return "", ErrNoMergeBase
		}
		return "", err
	}
	return strings.TrimSpace(string(stdout)), nil
}
