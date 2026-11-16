package planner

import (
	"slices"
	"strings"

	"gogs.io/gogs/internal/azulflow/model"
)

// Graph is the overview of a run, or of one call of a reusable workflow in
// it: the jobs of the workflow file in file order, the jobs each one needs,
// and their status. It has every job of the file, including those whose
// matrix or called workflow is not expanded yet, so that it can be drawn as
// GitHub draws the graph of a run.
type Graph struct {
	// Key is the prefix of the keys of the jobs of a called workflow, such
	// as "deploy[0]/", and empty for the run.
	Key string `json:"key,omitempty"`
	// Name is the run-name of the run, or the display name, within its
	// graph, of the job that called the workflow.
	Name string `json:"name,omitempty"`
	// Status is StatusQueued while the run or the call is in progress.
	Status Status       `json:"status"`
	Nodes  []*GraphNode `json:"nodes"`
}

// GraphNode is a job of a workflow file. A job with a matrix has a Job per
// combination, and a job that calls a reusable workflow has a Graph per
// combination.
type GraphNode struct {
	// Key is the Group of the jobs of the node, such as "deploy[0]/test".
	Key   string `json:"key"`
	JobID string `json:"jobId"`
	// Name is the literal name of the job, or its ID when it has no name or
	// its name is an expression.
	Name string `json:"name"`
	// Needs are the keys of the nodes of the same graph that this node
	// needs.
	Needs []string `json:"needs,omitempty"`
	// Status is StatusWaiting until the needs of the node finish, and
	// StatusQueued while its jobs run.
	Status Status `json:"status"`
	// Matrix is true for a job with a matrix, even before it is expanded.
	Matrix bool `json:"matrix,omitempty"`
	// Reusable is true for a job that calls a reusable workflow.
	Reusable bool        `json:"reusable,omitempty"`
	Jobs     []*GraphJob `json:"jobs,omitempty"`
	Calls    []*Graph    `json:"calls,omitempty"`
}

// GraphJob is a job of a node, as in Run.Jobs.
type GraphJob struct {
	Key string `json:"key"`
	// Name is the display name of the job within its graph, without the
	// name of the job that called its workflow.
	Name            string `json:"name"`
	Status          Status `json:"status"`
	Error           string `json:"error,omitempty"`
	CancelRequested bool   `json:"cancelRequested,omitempty"`
}

// Graph returns the overview of the run in its current state.
func (r *Run) Graph() *Graph {
	g := r.scopeGraph(r.root)
	g.Name = r.Name
	return g
}

func (r *Run) scopeGraph(s *scope) *Graph {
	g := &Graph{Key: s.key, Name: s.name, Status: s.result, Nodes: []*GraphNode{}}
	if !s.done {
		g.Status = StatusWaiting
		if slices.ContainsFunc(s.nodes, func(n *node) bool { return n.released }) {
			g.Status = StatusQueued
		}
	}
	for _, n := range s.nodes {
		g.Nodes = append(g.Nodes, r.nodeGraph(n))
	}
	return g
}

func (r *Run) nodeGraph(n *node) *GraphNode {
	gn := &GraphNode{
		Key:      n.key,
		JobID:    n.id,
		Name:     defaultName(n.def),
		Status:   n.result,
		Reusable: n.def.Type == model.JobTypeReusable,
		Matrix:   hasMatrix(n.def),
	}
	switch {
	case !n.released:
		gn.Status = StatusWaiting
	case !n.done:
		gn.Status = StatusQueued
	}
	for _, dep := range n.needs {
		gn.Needs = append(gn.Needs, dep.key)
	}
	// The jobs of a node are its matrix jobs, and the jobs that explain why
	// the outputs of its called workflows failed.
	for _, j := range r.Jobs {
		if j.node != n {
			continue
		}
		name := j.Name
		if n.scope.name != "" {
			name = strings.TrimPrefix(name, n.scope.name+" / ")
		}
		gn.Jobs = append(gn.Jobs, &GraphJob{
			Key: j.Key, Name: name, Status: j.Status, Error: j.Error, CancelRequested: j.CancelRequested,
		})
	}
	for _, c := range n.children {
		cg := r.scopeGraph(c)
		if n.scope.name != "" {
			cg.Name = strings.TrimPrefix(cg.Name, n.scope.name+" / ")
		}
		gn.Calls = append(gn.Calls, cg)
	}
	return gn
}

// hasMatrix reports whether a job defines a matrix, or a strategy that is an
// expression and so may.
func hasMatrix(def *model.Job) bool {
	s := def.Strategy
	return s != nil && (s.IsExpression() || s.Get("matrix") != nil)
}
