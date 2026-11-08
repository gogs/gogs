package git

import (
	"bytes"
	"context"
	"strings"

	"github.com/cockroachdb/errors"
)

// parseTag parses tag information from the (uncompressed) raw data of the tag
// object. It assumes "\n\n" separates the header from the rest of the message.
func parseTag(data []byte) (*Tag, error) {
	tag := new(Tag)
	// We now have the contents of the tag object. Let's investigate.
	nextline := 0
l:
	for {
		eol := bytes.IndexByte(data[nextline:], '\n')
		switch {
		case eol > 0:
			line := data[nextline : nextline+eol]
			spacepos := bytes.IndexByte(line, ' ')
			reftype := line[:spacepos]
			switch string(reftype) {
			case "object":
				id, err := NewIDFromString(string(line[spacepos+1:]))
				if err != nil {
					return nil, err
				}
				tag.commitID = id
			case "type":
			}
			nextline += eol + 1
		case eol == 0:
			tag.message = string(data[nextline+1:])
			break l
		default:
			break l
		}
	}
	return tag, nil
}

// getTag returns a tag by given SHA1 hash.
func (r *Repository) getTag(ctx context.Context, id *SHA1) (*Tag, error) {
	t, ok := r.cachedTags.Get(id.String())
	if ok {
		logf("Cached tag hit: %s", id)
		return t.(*Tag), nil
	}

	// Check tag type
	typ, err := r.CatFileType(ctx, id.String())
	if err != nil {
		return nil, err
	}

	var tag *Tag
	switch typ {
	case ObjectCommit: // Tag is a commit
		tag = &Tag{
			typ:      ObjectCommit,
			id:       id,
			commitID: id,
			repo:     r,
		}

	case ObjectTag: // Tag is an annotation
		data, err := exec(ctx, r.path, []string{"cat-file", "-p", id.String()}, nil)
		if err != nil {
			return nil, err
		}

		tag, err = parseTag(data)
		if err != nil {
			return nil, err
		}
		tag.typ = ObjectTag
		tag.id = id
		tag.repo = r
	default:
		return nil, errors.Newf("unsupported tag type: %s", typ)
	}

	r.cachedTags.Set(id.String(), tag)
	return tag, nil
}

// TagOptions contains optional arguments for getting a tag.
//
// Docs: https://git-scm.com/docs/git-cat-file
type TagOptions struct {
	CommandOptions
}

// Tag returns a Git tag by given name, e.g., "v1.0.0".
func (r *Repository) Tag(ctx context.Context, name string, opts ...TagOptions) (*Tag, error) {
	var opt TagOptions
	if len(opts) > 0 {
		opt = opts[0]
	}

	refspec := RefsTags + name
	refs, err := r.ShowRef(ctx, ShowRefOptions{
		Tags:           true,
		Patterns:       []string{refspec},
		CommandOptions: opt.CommandOptions,
	})
	if err != nil {
		return nil, err
	} else if len(refs) == 0 {
		return nil, ErrReferenceNotExist
	}

	id, err := NewIDFromString(refs[0].ID)
	if err != nil {
		return nil, err
	}

	tag, err := r.getTag(ctx, id)
	if err != nil {
		return nil, err
	}
	return tag, nil
}

// TagsOptions contains optional arguments for listing tags.
//
// Docs: https://git-scm.com/docs/git-tag#Documentation/git-tag.txt---list
type TagsOptions struct {
	// SortKey sorts tags with provided tag key, optionally prefixed with '-' to sort tags in descending order.
	SortKey string
	// Pattern filters tags matching the specified pattern.
	Pattern string
	CommandOptions
}

// Tags returns a list of tags of the repository.
func (r *Repository) Tags(ctx context.Context, opts ...TagsOptions) ([]string, error) {
	var opt TagsOptions
	if len(opts) > 0 {
		opt = opts[0]
	}

	args := []string{"tag", "--list"}
	if opt.SortKey != "" {
		args = append(args, "--sort="+opt.SortKey)
	} else {
		args = append(args, "--sort=-creatordate")
	}
	args = append(args, "--end-of-options")
	if opt.Pattern != "" {
		args = append(args, opt.Pattern)
	}

	stdout, err := exec(ctx, r.path, args, opt.Envs)
	if err != nil {
		return nil, err
	}

	tags := strings.Split(string(stdout), "\n")
	tags = tags[:len(tags)-1]

	return tags, nil
}

// RepoTags is like Repository.Tags for the repository in given path.
func RepoTags(ctx context.Context, repoPath string, opts ...TagsOptions) ([]string, error) {
	r, err := Open(repoPath)
	if err != nil {
		return nil, err
	}
	return r.Tags(ctx, opts...)
}

// CreateTagOptions contains optional arguments for creating a tag.
//
// Docs: https://git-scm.com/docs/git-tag
type CreateTagOptions struct {
	// Annotated marks a tag as annotated rather than lightweight.
	Annotated bool
	// Message specifies a tagging message for the annotated tag. It is ignored when tag is not annotated.
	Message string
	// Author is the tagger of an annotated tag. It is required when the tag is
	// annotated and ignored for a lightweight tag.
	Author *Signature
	CommandOptions
}

// CreateTag creates a new tag on given revision.
func (r *Repository) CreateTag(ctx context.Context, name, rev string, opts ...CreateTagOptions) error {
	var opt CreateTagOptions
	if len(opts) > 0 {
		opt = opts[0]
	}

	args := []string{"tag"}
	var envs []string
	if opt.Annotated {
		if opt.Author == nil {
			return errors.New("author is required")
		}
		args = append(args, "-a", name)
		args = append(args, "--message", opt.Message)
		envs = committerEnvs(opt.Author)
		args = append(args, "--end-of-options")
	} else {
		args = append(args, "--end-of-options", name)
	}
	args = append(args, rev)
	envs = append(envs, opt.Envs...)
	_, err := exec(ctx, r.path, args, envs)
	return err
}

// DeleteTagOptions contains optional arguments for deleting a tag.
//
// Docs: https://git-scm.com/docs/git-tag#Documentation/git-tag.txt---delete
type DeleteTagOptions struct {
	CommandOptions
}

// DeleteTag deletes a tag from the repository.
func (r *Repository) DeleteTag(ctx context.Context, name string, opts ...DeleteTagOptions) error {
	var opt DeleteTagOptions
	if len(opts) > 0 {
		opt = opts[0]
	}

	args := []string{"tag", "--delete", "--end-of-options", name}
	_, err := exec(ctx, r.path, args, opt.Envs)
	return err
}

// RepoDeleteTag is like Repository.DeleteTag for the repository in given path.
func RepoDeleteTag(ctx context.Context, repoPath, name string, opts ...DeleteTagOptions) error {
	r, err := Open(repoPath)
	if err != nil {
		return err
	}
	return r.DeleteTag(ctx, name, opts...)
}

// TagsPage contains a list of tags and pagination information.
type TagsPage struct {
	// List of tags in the current page.
	Tags []string
	// Whether the results include the latest tag.
	HasLatest bool
	// When results do not include the latest tag, an indicator of 'after' to go back.
	PreviousAfter string
	// Whether there are more tags in the next page.
	HasNext bool
}

// TagsAfter returns a page of at most limit tags "after" (exclusive) the given
// tag. A limit of 0 means no limit.
func (r *Repository) TagsAfter(ctx context.Context, after string, limit int) (*TagsPage, error) {
	all, err := r.Tags(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "get tags")
	}
	return tagsAfter(all, after, limit), nil
}

func tagsAfter(all []string, after string, limit int) *TagsPage {
	total := len(all)

	if limit < 0 {
		limit = 0
	}

	// Returns everything when no filter and no limit.
	if after == "" && limit == 0 {
		return &TagsPage{
			Tags:      all,
			HasLatest: true,
		}
	}

	// No filter but has a limit, returns first X tags.
	if after == "" && limit > 0 {
		endIdx := min(limit, total)
		return &TagsPage{
			Tags:      all[:endIdx],
			HasLatest: true,
			HasNext:   limit < total,
		}
	}

	previousAfter := ""
	found := false
	tags := make([]string, 0, len(all))
	for i := range all {
		if all[i] != after {
			continue
		}

		found = true
		if limit > 0 && i-limit >= 0 {
			previousAfter = all[i-limit]
		}

		// In case filter is the oldest one.
		if i+1 < total {
			tags = all[i+1:]
		}
		break
	}

	if !found {
		tags = all
	}

	// If all tags after match is equal to the limit, it reaches the oldest tag as well.
	if limit == 0 || len(tags) <= limit {
		return &TagsPage{
			Tags:          tags,
			HasLatest:     !found,
			PreviousAfter: previousAfter,
		}
	}

	return &TagsPage{
		Tags:          tags[:limit],
		HasLatest:     !found,
		PreviousAfter: previousAfter,
		HasNext:       true,
	}
}
