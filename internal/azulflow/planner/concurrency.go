package planner

import (
	"fmt"
	"maps"
	"slices"
	"strings"
)

// maxPending is how many claims may wait for a group with `queue: max`.
const maxPending = 100

// A Claim is what holds or waits for a concurrency group: a run, a call of a
// reusable workflow in a run, or a job of a run.
type Claim struct {
	Run int64 `json:"run"`
	// Key is empty for the run, the key prefix of a call, such as
	// "deploy[0]/", or the key of a job, as Run.CancelClaim takes them.
	Key string `json:"key,omitempty"`
}

// contains reports whether c is a run or call that contains o.
func (c Claim) contains(o Claim) bool {
	return c.Run == o.Run && c != o && (c.Key == "" || strings.HasSuffix(c.Key, "/") && strings.HasPrefix(o.Key, c.Key))
}

// Groups arbitrates the concurrency groups of a repository as GitHub does.
// Group names are case-insensitive and shared by runs and jobs of every
// workflow. At most one claim holds a group, and the others wait in FIFO
// order. It does no I/O: the scheduler persists it, and acts on its
// decisions with Run.CancelClaim and Run.Discard.
type Groups struct {
	Groups map[string]*Group `json:"groups,omitempty"`
}

// Group is the state of one concurrency group.
type Group struct {
	Holder  *Claim  `json:"holder,omitempty"`
	Pending []Claim `json:"pending,omitempty"`
}

// Cancellation is a claim the scheduler must cancel, and why, in GitHub's
// words.
type Cancellation struct {
	Claim Claim
	// Started is true for a claim that holds its group: the scheduler cancels
	// it with Run.CancelClaim. A claim that was waiting never started, and the
	// scheduler cancels it with Run.Discard.
	Started bool
	Reason  string
}

// Request asks for the group of c, which describes itself as name in
// messages. It returns whether c holds the group and may start, and the
// claims to cancel. Asking again for a group c holds or waits for changes
// nothing.
func (g *Groups) Request(c Claim, name string, concurrency *Concurrency) (start bool, cancel []Cancellation) {
	if concurrency == nil {
		return true, nil
	}
	if g.Groups == nil {
		g.Groups = map[string]*Group{}
	}
	key := strings.ToLower(concurrency.Group)
	group := g.Groups[key]
	if group == nil {
		group = &Group{}
		g.Groups[key] = group
	}
	switch {
	case group.Holder != nil && *group.Holder == c:
		return true, nil
	case slices.Contains(group.Pending, c):
		return false, nil
	}

	// A claim would wait forever for a group that the run or call containing
	// it holds or waits for.
	for _, other := range append(slices.Clip(group.Pending), holderOf(group)...) {
		if other.contains(c) {
			outer := "top level workflow"
			if other.Key != "" {
				outer = strings.TrimSuffix(other.Key, "/")
			}
			return false, []Cancellation{{Claim: c, Reason: fmt.Sprintf(
				"Canceling since a deadlock for concurrency group '%s' was detected between '%s' and '%s'",
				concurrency.Group, outer, name)}}
		}
	}

	if group.Holder == nil {
		group.Holder = &c
		return true, nil
	}
	replaced := fmt.Sprintf("Canceling since a higher priority waiting request for '%s' exists", concurrency.Group)
	if concurrency.CancelInProgress {
		cancel = append(cancel, Cancellation{Claim: *group.Holder, Started: true, Reason: replaced})
	}
	if concurrency.Queue == "max" {
		if len(group.Pending) >= maxPending {
			return false, append(cancel, Cancellation{Claim: c, Reason: fmt.Sprintf(
				"Canceling since the queue for concurrency group '%s' is full", concurrency.Group)})
		}
	} else {
		for _, p := range group.Pending {
			cancel = append(cancel, Cancellation{Claim: p, Reason: replaced})
		}
		group.Pending = nil
	}
	group.Pending = append(group.Pending, c)
	return false, cancel
}

func holderOf(g *Group) []Claim {
	if g.Holder == nil {
		return nil
	}
	return []Claim{*g.Holder}
}

// Release removes c from every group it holds or waits for, once it
// finished or was cancelled. It returns the claims that now hold a group, in
// FIFO order, which start once they hold all of theirs.
func (g *Groups) Release(c Claim) []Claim {
	var granted []Claim
	for _, key := range slices.Sorted(maps.Keys(g.Groups)) {
		group := g.Groups[key]
		group.Pending = slices.DeleteFunc(group.Pending, func(p Claim) bool { return p == c })
		if group.Holder != nil && *group.Holder == c {
			group.Holder = nil
			if len(group.Pending) > 0 {
				next := group.Pending[0]
				group.Holder, group.Pending = &next, group.Pending[1:]
				granted = append(granted, next)
			}
		}
		if group.Holder == nil && len(group.Pending) == 0 {
			delete(g.Groups, key)
		}
	}
	return granted
}
