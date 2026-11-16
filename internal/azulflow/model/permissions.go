package model

import (
	"maps"
	"slices"
	"strings"

	"gogs.io/gogs/internal/azulflow/template"
)

// Level is the access a token permission grants.
type Level int

const (
	LevelNone Level = iota
	LevelRead
	LevelWrite
)

func (l Level) String() string {
	switch l {
	case LevelRead:
		return "read"
	case LevelWrite:
		return "write"
	}
	return "none"
}

// Permissions are the permissions of the GITHUB_TOKEN, by scope, such as
// "contents". A nil value means that none were declared.
type Permissions map[string]Level

// DefaultScopes are the scopes that "read-all" and "write-all" grant, in the
// order GitHub lists them in errors.
var DefaultScopes = []string{
	"actions", "artifact-metadata", "attestations", "checks", "contents", "deployments",
	"discussions", "issues", "packages", "pages", "pull-requests", "repository-projects",
	"statuses", "security-events", "id-token", "models", "vulnerability-alerts",
}

// readOnlyScopes are the scopes that never grant more than read access.
var readOnlyScopes = map[string]bool{"models": true, "vulnerability-alerts": true}

func (p Permissions) clone() Permissions {
	if p == nil {
		return nil
	}
	return maps.Clone(p)
}

// all returns the permissions that grant every scope the level, as
// "read-all" and "write-all" do.
func allPermissions(scopes []string, level Level, includeIDToken bool) Permissions {
	p := Permissions{}
	for _, s := range scopes {
		l := level
		if s == "id-token" && !includeIDToken {
			l = LevelNone
		}
		if readOnlyScopes[s] && l == LevelWrite {
			l = LevelRead
		}
		p[s] = l
	}
	return p
}

func (p Permissions) json() jsonValue {
	o := &jsonObject{}
	for _, scope := range slices.Sorted(maps.Keys(p)) {
		if p[scope] != LevelNone {
			o.set(scope, p[scope].String())
		}
	}
	return o
}

// violations lists the scopes where p grants more than max allows, in the
// order of scopes, then of the other scopes by name.
func (p Permissions) violations(max Permissions, scopes []string) (requested, allowed []string) {
	order := slices.Clone(scopes)
	for _, s := range slices.Sorted(maps.Keys(p)) {
		if !slices.Contains(order, s) {
			order = append(order, s)
		}
	}
	for _, s := range order {
		if p[s] > max[s] {
			requested = append(requested, s+": "+p[s].String())
			allowed = append(allowed, s+": "+max[s].String())
		}
	}
	return requested, allowed
}

func (c *converter) permissions(t *template.Token) Permissions {
	if t.Kind == template.String {
		switch t.Str {
		case "read-all":
			return allPermissions(c.opts.Scopes, LevelRead, true)
		case "write-all":
			return allPermissions(c.opts.Scopes, LevelWrite, true)
		}
		throwUnexpectedValue(t.Str, t)
	}
	mustOK(t.AssertMapping("permissions"))
	p := Permissions{}
	for _, pair := range t.Map {
		mustOK(pair.Key.AssertString("permissions.key"))
		mustOK(pair.Value.AssertString("permissions.value"))
		var l Level
		switch pair.Value.Str {
		case "none":
		case "read":
			l = LevelRead
		case "write":
			l = LevelWrite
		default:
			throwUnexpectedValue(pair.Value.Str, pair.Value)
		}
		if readOnlyScopes[pair.Key.Str] && l == LevelWrite {
			l = LevelRead
		}
		p[pair.Key.Str] = l
	}
	return p
}

// policyPermissions returns the most a reusable workflow may request when
// its caller does not limit permissions, from the repository's policy.
func (c *converter) policyPermissions(trusted bool) Permissions {
	if strings.EqualFold(c.opts.PermissionsPolicy, PolicyLimitedRead) {
		p := allPermissions(c.opts.Scopes, LevelNone, false)
		p["contents"] = LevelRead
		p["packages"] = LevelRead
		return p
	}
	return allPermissions(c.opts.Scopes, LevelWrite, trusted)
}

// validateEmbeddedPermissions reports when a reusable workflow, or one of its
// jobs, requests more permissions than its caller allows.
func (c *converter) validateEmbeddedPermissions(caller *Job, embedded *Job, requested, explicitMax Permissions, trusted bool) {
	if requested == nil {
		return
	}
	max := explicitMax
	if max == nil {
		max = c.policyPermissions(trusted)
	}
	req, allowed := requested.violations(max, c.opts.Scopes)
	if len(req) == 0 {
		return
	}
	ref := caller.Ref.String()
	who := "The workflow '" + ref + "'"
	if embedded != nil {
		who = "The nested job '" + embedded.Name.String() + "'"
	}
	c.ctx.Error(caller.ID, "Error calling workflow '"+ref+"'. "+who+" is requesting '"+
		strings.Join(req, ", ")+"', but is only allowed '"+strings.Join(allowed, ", ")+"'.")
}
