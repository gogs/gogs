package planner

import (
	"regexp"
	"slices"
	"strings"

	"github.com/cockroachdb/errors"

	"gogs.io/gogs/internal/azulflow/model"
)

// Event is a repository event, as the trigger describes it to the planner.
type Event struct {
	// Name is the event name, such as "push" or "pull_request".
	Name string
	// Action is the activity type, such as "opened" for pull_request. It is
	// empty for events without types.
	Action string
	// Ref is the full ref that branch and tag filters match: the pushed ref
	// for push, the base ref for pull_request and pull_request_target, and the
	// head branch of the triggering run for workflow_run.
	Ref string
	// ChangedFiles lists the paths that path filters match: from the before
	// to the after commit for push, and from the merge base for pull_request.
	// Nil means the list is unknown.
	ChangedFiles []string
	// ChangedFilesTruncated is true when ChangedFiles holds only part of the
	// changes, such as for a very large push.
	ChangedFilesTruncated bool
	// Cron and Timezone are the schedule that fired a schedule event. An
	// empty Timezone is UTC.
	Cron     string
	Timezone string
	// Workflow is the name of the workflow whose run triggered a workflow_run
	// event.
	Workflow string
}

// Activity types that pull_request and pull_request_target match when a
// workflow lists none.
var defaultPullRequestTypes = []string{"opened", "synchronize", "reopened"}

// Match reports whether the workflow runs for the event. Path filters pass
// when the changed files are unknown or truncated, so that a workflow runs
// rather than silently not running.
func Match(w *model.Workflow, ev *Event) (bool, error) {
	t := w.Events.Get(ev.Name)
	if t == nil || ev.Name == "workflow_call" {
		return false, nil
	}

	types, _ := t.Filter("types")
	if len(types) == 0 && (ev.Name == "pull_request" || ev.Name == "pull_request_target") {
		types = defaultPullRequestTypes
	}
	if len(types) > 0 && ev.Action != "" && !slices.Contains(types, ev.Action) {
		return false, nil
	}

	switch ev.Name {
	case "schedule":
		return slices.ContainsFunc(t.Schedule, func(s model.Schedule) bool {
			return s.Cron == ev.Cron && s.Timezone == ev.Timezone
		}), nil
	case "workflow_run":
		if workflows, _ := t.Filter("workflows"); !slices.Contains(workflows, ev.Workflow) {
			return false, nil
		}
	}

	ok, isTag, err := matchRef(t, ev.Ref)
	if err != nil || !ok {
		return false, err
	}
	// Path filters are not evaluated for tag pushes.
	if isTag {
		return true, nil
	}
	return matchPaths(t, ev)
}

func matchRef(t *model.Event, ref string) (ok, isTag bool, err error) {
	branches, _ := t.Filter("branches")
	branchesIgnore, _ := t.Filter("branches-ignore")
	tags, _ := t.Filter("tags")
	tagsIgnore, _ := t.Filter("tags-ignore")
	branchFilter := len(branches) > 0 || len(branchesIgnore) > 0
	tagFilter := len(tags) > 0 || len(tagsIgnore) > 0
	if name, ok := strings.CutPrefix(ref, "refs/tags/"); ok {
		// A workflow with only branch filters does not run for tags.
		if !tagFilter {
			return !branchFilter, true, nil
		}
		ok, err := matchFilter(name, tags, tagsIgnore)
		return ok, true, err
	}
	name := strings.TrimPrefix(ref, "refs/heads/")
	if !branchFilter {
		return !tagFilter, false, nil
	}
	ok, err = matchFilter(name, branches, branchesIgnore)
	return ok, false, err
}

func matchFilter(name string, patterns, ignore []string) (bool, error) {
	if len(patterns) > 0 {
		return matchPatterns(patterns, name)
	}
	ignored, err := matchPatterns(ignore, name)
	return !ignored, err
}

func matchPaths(t *model.Event, ev *Event) (bool, error) {
	paths, _ := t.Filter("paths")
	pathsIgnore, _ := t.Filter("paths-ignore")
	if len(paths) == 0 && len(pathsIgnore) == 0 {
		return true, nil
	}
	if ev.ChangedFiles == nil || ev.ChangedFilesTruncated {
		return true, nil
	}
	for _, file := range ev.ChangedFiles {
		if len(paths) > 0 {
			ok, err := matchPatterns(paths, file)
			if err != nil || ok {
				return ok, err
			}
			continue
		}
		// With paths-ignore, the workflow runs unless every file is ignored.
		ignored, err := matchPatterns(pathsIgnore, file)
		if err != nil || !ignored {
			return !ignored, err
		}
	}
	return false, nil
}

// matchPatterns reports whether name matches a list of filter patterns. The
// last pattern that matches decides, so a pattern starting with "!" excludes
// names that earlier patterns included.
func matchPatterns(patterns []string, name string) (bool, error) {
	matched := false
	for _, pattern := range patterns {
		negated := strings.HasPrefix(pattern, "!")
		re, err := compileGlob(strings.TrimPrefix(pattern, "!"))
		if err != nil {
			return false, err
		}
		if re.MatchString(name) {
			matched = !negated
		}
	}
	return matched, nil
}

// compileGlob compiles a filter pattern with GitHub's syntax: "*" matches
// anything but "/", "**" matches anything, "**/" also matches no directory,
// "?" and "+" make the previous character optional or repeatable, "[...]"
// matches a character class, and "\" escapes the next character.
func compileGlob(pattern string) (*regexp.Regexp, error) {
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(pattern); i++ {
		c := pattern[i]
		switch c {
		case '*':
			if i+1 < len(pattern) && pattern[i+1] == '*' {
				i++
				if i+1 < len(pattern) && pattern[i+1] == '/' {
					i++
					b.WriteString("(?:.*/)?")
				} else {
					b.WriteString(".*")
				}
			} else {
				b.WriteString("[^/]*")
			}
		case '?', '+':
			if i == 0 {
				return nil, errors.Newf("filter pattern %q starts with %q", pattern, c)
			}
			b.WriteByte(c)
		case '[':
			end := strings.IndexByte(pattern[i+1:], ']')
			if end < 0 {
				return nil, errors.Newf("filter pattern %q has an unclosed '['", pattern)
			}
			class := pattern[i+1 : i+1+end]
			if class == "" {
				return nil, errors.Newf("filter pattern %q has an empty '[]'", pattern)
			}
			b.WriteByte('[')
			for _, r := range class {
				if r == '-' {
					b.WriteRune(r)
				} else {
					b.WriteString(regexp.QuoteMeta(string(r)))
				}
			}
			b.WriteByte(']')
			i += end + 1
		case '\\':
			if i+1 < len(pattern) {
				i++
				c = pattern[i]
			}
			b.WriteString(regexp.QuoteMeta(string(c)))
		default:
			b.WriteString(regexp.QuoteMeta(string(c)))
		}
	}
	b.WriteString("$")
	re, err := regexp.Compile(b.String())
	if err != nil {
		return nil, errors.Wrapf(err, "compile filter pattern %q", pattern)
	}
	return re, nil
}

// Source is the commit whose workflow files apply to an event.
type Source int

const (
	// SourceDefaultBranch is the latest commit on the default branch.
	SourceDefaultBranch Source = iota
	// SourceEventCommit is the commit the event is about, such as the pushed
	// commit, or the commit a tag, release, or deployment points to.
	SourceEventCommit
	// SourceMergeCommit is the test merge commit of a pull request's head into
	// its base.
	SourceMergeCommit
	// SourceBaseBranch is the latest commit on a pull request's base branch.
	SourceBaseBranch
	// SourceDispatchRef is the ref the user picked for workflow_dispatch.
	SourceDispatchRef
)

// WorkflowSource returns the commit whose workflow files apply to an event,
// with GitHub's rules. A workflow_call uses its caller's commit and is never
// looked up this way.
func WorkflowSource(event string) Source {
	switch event {
	case "push", "create", "release", "deployment", "deployment_status", "merge_group":
		return SourceEventCommit
	case "pull_request", "pull_request_review", "pull_request_review_comment":
		return SourceMergeCommit
	case "pull_request_target":
		return SourceBaseBranch
	case "workflow_dispatch":
		return SourceDispatchRef
	}
	return SourceDefaultBranch
}
