# Macaron to Flamego Migration Guide

## Executive Summary

This document provides a comprehensive guide for migrating Gogs from the Macaron web framework to Flamego. Flamego is the official successor to Macaron, created by the same author, offering improved performance, better routing capabilities, and modern Go practices.

## Table of Contents

1. [Why Migrate to Flamego?](#why-migrate-to-flamego)
2. [Framework Comparison](#framework-comparison)
3. [Middleware Availability](#middleware-availability)
4. [Migration Strategy](#migration-strategy)
5. [Code Changes Required](#code-changes-required)
6. [Potential Issues](#potential-issues)
7. [Testing Strategy](#testing-strategy)
8. [Rollback Plan](#rollback-plan)

## Why Migrate to Flamego?

### Benefits of Flamego

1. **Official Successor**: Flamego is created by the same author as Macaron and is designed as its replacement
2. **Better Performance**: Improved routing engine with O(1) lookup for static routes
3. **Modern Go**: Requires Go 1.19+, uses modern Go features and best practices
4. **Enhanced Routing**: Most powerful routing syntax in the Go ecosystem
5. **Active Development**: Regular updates and maintenance (Macaron is in maintenance mode)
6. **Better Testing**: Designed with testability in mind
7. **Same Philosophy**: Maintains the dependency injection pattern that makes Macaron great

### Risks and Considerations

1. **Breaking Changes**: Handler signatures and some middleware APIs differ
2. **Migration Scope**: ~150+ files need modification
3. **Testing Burden**: Comprehensive testing required for web functionality
4. **Learning Curve**: Team needs to understand new APIs and patterns
5. **Third-party Dependencies**: Some custom Macaron middleware may need replacement

## Framework Comparison

### Core Framework

| Feature | Macaron | Flamego | Notes |
|---------|---------|---------|-------|
| **Initialization** | `macaron.New()` | `flamego.New()` | Similar API |
| **Classic Setup** | `macaron.Classic()` | `flamego.Classic()` | Both include logger, recovery, static |
| **Handler Signature** | `func(*macaron.Context)` | `func(flamego.Context)` | Flamego uses interface |
| **Dependency Injection** | Function parameters | Function parameters | Same pattern |
| **Routing** | Basic | Advanced (regex, optional segments) | Flamego more powerful |
| **Route Groups** | `m.Group()` | `f.Group()` | Same concept, similar API |
| **Middleware** | `m.Use()` | `f.Use()` | Same pattern |

### Context API Comparison

| Operation | Macaron | Flamego |
|-----------|---------|---------|
| **Get Request** | `c.Req.Request` | `c.Request().Request` |
| **Get Response** | `c.Resp` | `c.ResponseWriter()` |
| **URL Params** | `c.Params(":name")` | `c.Param("name")` (no colon) |
| **Query Params** | `c.Query("key")` | `c.Query("key")` |
| **Redirect** | `c.Redirect(url)` | `c.Redirect(url)` |
| **Set Header** | `c.Resp.Header().Set()` | `c.ResponseWriter().Header().Set()` |
| **JSON Response** | `c.JSON(200, data)` | Use render middleware |
| **HTML Response** | `c.HTML(200, tpl)` | Use template middleware |

### Example Code Comparison

**Macaron:**
```go
m := macaron.New()
m.Use(macaron.Logger())
m.Use(macaron.Recovery())

m.Get("/:username/:repo", func(c *macaron.Context) {
    username := c.Params(":username")
    repo := c.Params(":repo")
    c.JSON(200, map[string]string{
        "username": username,
        "repo": repo,
    })
})
```

**Flamego:**
```go
f := flamego.New()
f.Use(flamego.Logger())
f.Use(flamego.Recovery())

f.Get("/<username>/<repo>", func(c flamego.Context) {
    username := c.Param("username")
    repo := c.Param("repo")
    // Use render middleware for JSON
    c.ResponseWriter().Header().Set("Content-Type", "application/json")
    json.NewEncoder(c.ResponseWriter()).Encode(map[string]string{
        "username": username,
        "repo": repo,
    })
})
```

## Middleware Availability

### Middleware Mapping

| Function | Macaron Package | Flamego Package | Status |
|----------|----------------|-----------------|--------|
| **Core** | `gopkg.in/macaron.v1` | `github.com/flamego/flamego` | ✅ Available |
| **Binding** | `github.com/go-macaron/binding` | `github.com/flamego/binding` | ✅ Available |
| **Cache** | `github.com/go-macaron/cache` | `github.com/flamego/cache` | ✅ Available |
| **Captcha** | `github.com/go-macaron/captcha` | `github.com/flamego/captcha` | ✅ Available |
| **CSRF** | `github.com/go-macaron/csrf` | `github.com/flamego/csrf` | ✅ Available |
| **Gzip** | `github.com/go-macaron/gzip` | `github.com/flamego/gzip` | ✅ Available |
| **i18n** | `github.com/go-macaron/i18n` | `github.com/flamego/i18n` | ✅ Available |
| **Session** | `github.com/go-macaron/session` | `github.com/flamego/session` | ✅ Available |
| **Template** | Built-in `macaron.Renderer()` | `github.com/flamego/template` | ✅ Available |
| **Toolbox** | `github.com/go-macaron/toolbox` | N/A | ⚠️ Need custom implementation |

### Middleware API Changes

#### Session

**Macaron:**
```go
m.Use(session.Sessioner(session.Options{
    Provider:       "memory",
    ProviderConfig: "",
}))

// In handler
func(sess session.Store) {
    sess.Set("key", "value")
    value := sess.Get("key")
}
```

**Flamego:**
```go
f.Use(session.Sessioner(session.Options{
    Config: session.MemoryConfig{},
}))

// In handler
func(s session.Session) {
    s.Set("key", "value")
    value := s.Get("key")
}
```

#### CSRF

**Macaron:**
```go
m.Use(csrf.Csrfer(csrf.Options{
    Secret: "secret-key",
}))

// In handler
func(x csrf.CSRF) {
    token := x.GetToken()
}
```

**Flamego:**
```go
f.Use(csrf.Csrfer(csrf.Options{
    Secret: "secret-key",
}))

// In handler - similar API
func(x csrf.CSRF) {
    token := x.Token()
}
```

#### Binding

**Macaron:**
```go
type Form struct {
    Username string `form:"username" binding:"Required"`
}

m.Post("/signup", binding.Bind(Form{}), func(form Form) {
    // Use form
})
```

**Flamego:**
```go
type Form struct {
    Username string `form:"username" validate:"required"`
}

f.Post("/signup", binding.Form(Form{}), func(form Form) {
    // Use form
})
```

#### Template/Renderer

**Macaron:**
```go
m.Use(macaron.Renderer(macaron.RenderOptions{
    Directory: "templates",
}))

func(c *macaron.Context) {
    c.HTML(200, "index")
}
```

**Flamego:**
```go
import "github.com/flamego/template"

f.Use(template.Templater(template.Options{
    Directory: "templates",
}))

func(t template.Template, data template.Data) {
    data["Title"] = "Home"
    t.HTML(200, "index")
}
```

#### Cache

**Macaron:**
```go
m.Use(cache.Cacher(cache.Options{
    Adapter: "memory",
}))

func(cache cache.Cache) {
    cache.Put("key", "value", 60)
    value := cache.Get("key")
}
```

**Flamego:**
```go
import "github.com/flamego/cache"

f.Use(cache.Cacher(cache.Options{
    Config: cache.MemoryConfig{},
}))

func(c cache.Cache) {
    c.Set("key", "value", 60)
    value := c.Get("key")
}
```

## Migration Strategy

### Phase 1: Preparation (1-2 days)

1. **Create feature branch**: `feature/flamego-migration`
2. **Update go.mod**: Add Flamego dependencies
3. **Study Flamego docs**: Ensure team understanding
4. **Identify custom middleware**: Document any custom Macaron extensions
5. **Setup test environment**: Ensure comprehensive test coverage

### Phase 2: Core Migration (3-5 days)

1. **Update main application setup** (`internal/cmd/web.go`)
   - Replace `macaron.New()` with `flamego.New()`
   - Convert middleware stack to Flamego
   - Update static file serving

2. **Update Context wrapper** (`internal/context/context.go`)
   - Change from `*macaron.Context` to `flamego.Context`
   - Update all Context methods to use Flamego APIs
   - Ensure backward compatibility where possible

3. **Migrate middleware configuration**
   - Session → Flamego session
   - CSRF → Flamego csrf
   - Cache → Flamego cache
   - i18n → Flamego i18n
   - Template rendering → Flamego template
   - Gzip → Flamego gzip
   - Captcha → Flamego captcha

### Phase 3: Route Handlers (5-7 days)

1. **Update route definitions**
   - Change route parameter syntax (`:param` → `<param>`)
   - Update regex patterns if used
   - Test all route patterns

2. **Update handler functions** (organized by module)
   - User routes (`internal/route/user/*.go`)
   - Repo routes (`internal/route/repo/*.go`)
   - Admin routes (`internal/route/admin/*.go`)
   - Org routes (`internal/route/org/*.go`)
   - API routes (`internal/route/api/v1/*.go`)
   - LFS routes (`internal/route/lfs/*.go`)

3. **Update context usage in handlers**
   - Replace `c.Params(":name")` with `c.Param("name")`
   - Update response methods
   - Update redirect calls

### Phase 4: Forms and Binding (2-3 days)

1. **Update form structs** (`internal/form/*.go`)
   - Change binding tags to Flamego format
   - Update validation rules
   - Test form binding with all HTTP methods

2. **Update custom validators**
   - Adapt to Flamego's validation system
   - Ensure all custom rules work

### Phase 5: Testing (3-5 days)

1. **Unit tests**
   - Update test helpers
   - Fix broken tests
   - Add new tests for changed functionality

2. **Integration tests**
   - Test all major user flows
   - Test API endpoints
   - Test authentication/authorization

3. **Manual testing**
   - Test UI flows
   - Test file uploads
   - Test webhooks
   - Test LFS

### Phase 6: Performance and Polish (2-3 days)

1. **Performance testing**
   - Benchmark critical paths
   - Compare with Macaron version
   - Optimize if needed

2. **Code cleanup**
   - Remove old Macaron imports
   - Update comments and documentation
   - Remove unused code

3. **Documentation updates**
   - Update README if needed
   - Update developer documentation
   - Document new patterns

### Total Estimated Timeline: 16-25 days

## Code Changes Required

### File Categories

1. **Core Web Setup** (2 files)
   - `internal/cmd/web.go` - Main application setup
   - `internal/app/api.go` - API setup
   
2. **Context System** (10 files in `internal/context/`)
   - `context.go` - Main context wrapper
   - `auth.go` - Authentication context
   - `api.go` - API context
   - `user.go` - User context
   - `repo.go` - Repository context
   - `org.go` - Organization context
   - And others...

3. **Form Definitions** (6 files in `internal/form/`)
   - All form binding structs need tag updates

4. **Route Handlers** (100+ files)
   - All files in `internal/route/` and subdirectories
   - Update handler signatures
   - Update context usage

5. **Tests** (50+ files)
   - Update test helpers
   - Fix integration tests
   - Update mocks

### Critical Files to Update

```
/internal/cmd/web.go                 # Main app setup - HIGH PRIORITY
/internal/context/context.go         # Context wrapper - HIGH PRIORITY
/internal/form/form.go               # Form binding - HIGH PRIORITY
/internal/route/install.go           # Install flow - CRITICAL
/internal/route/home.go              # Home page - CRITICAL
/internal/route/user/*.go            # User management
/internal/route/repo/*.go            # Repository operations
/internal/route/admin/*.go           # Admin panel
/internal/route/api/v1/*.go          # API endpoints
/internal/route/lfs/*.go             # LFS operations
/templates/embed.go                  # Template system
/go.mod                              # Dependencies
```

## Potential Issues

### 1. Toolbox Middleware

**Issue**: Macaron's toolbox middleware (health checks, profiling) has no direct Flamego equivalent.

**Solution**: Implement custom health check endpoint:
```go
f.Get("/-/health", func(c flamego.Context) {
    if err := database.Ping(); err != nil {
        c.ResponseWriter().WriteHeader(500)
        return
    }
    c.ResponseWriter().WriteHeader(200)
    c.ResponseWriter().Write([]byte("OK"))
})
```

### 2. Context Embedding

**Issue**: Current Context embeds `*macaron.Context`, which is tightly coupled.

**Solution**: Refactor to use composition instead:
```go
type Context struct {
    ctx flamego.Context
    // Other fields...
}

func (c *Context) Context() flamego.Context {
    return c.ctx
}
```

### 3. Response Methods

**Issue**: Gogs has many custom response methods on Context (HTML, JSON, etc.).

**Solution**: Update methods to use Flamego's middleware:
```go
// Before (Macaron)
func (c *Context) JSON(status int, data any) {
    c.Context.JSON(status, data)
}

// After (Flamego) - inject template.Template
func (c *Context) JSON(status int, data any) {
    c.ResponseWriter().Header().Set("Content-Type", "application/json")
    c.ResponseWriter().WriteHeader(status)
    json.NewEncoder(c.ResponseWriter()).Encode(data)
}
```

### 4. Route Parameter Syntax

**Issue**: Macaron uses `:param`, Flamego uses `<param>`.

**Solution**: Find and replace all route definitions:
```bash
# Find all route definitions
grep -r 'm\.Get\|m\.Post\|m\.Put\|m\.Delete\|m\.Patch' internal/cmd/web.go

# Update syntax
:param → <param>
```

### 5. Regex Routes

**Issue**: Macaron uses `^pattern$` for regex, Flamego has different syntax.

**Solution**: Update regex patterns to Flamego format:
```go
// Macaron
m.Get("/^:type(issues|pulls)$", handler)

// Flamego
f.Get("/<type:issues|pulls>", handler)
```

### 6. Dependency Injection Order

**Issue**: Handler function parameter order matters in both frameworks.

**Solution**: Ensure correct parameter order in handlers:
```go
// Flamego injects in order: Context, custom services, form bindings
func handler(
    c flamego.Context,
    sess session.Session,
    form UserForm,
) { }
```

### 7. Flash Messages

**Issue**: Flash messages API may differ.

**Solution**: Test and update flash message handling:
```go
// Verify API compatibility
sess.SetFlash("message")
flash := sess.GetFlash()
```

### 8. Custom Middleware

**Issue**: Any custom Macaron middleware needs porting.

**Solution**: Audit and rewrite custom middleware:
```go
// Macaron middleware
func MyMiddleware() macaron.Handler {
    return func(c *macaron.Context) { }
}

// Flamego middleware
func MyMiddleware() flamego.Handler {
    return func(c flamego.Context) { }
}
```

## Testing Strategy

### 1. Test Environment Setup

```bash
# Keep both versions temporarily
go mod tidy

# Run tests with Flamego
go test ./...

# Compare behavior
```

### 2. Critical Test Cases

- [ ] User registration and login
- [ ] Repository creation and deletion
- [ ] Push and pull operations (HTTP)
- [ ] Issue creation and comments
- [ ] Pull request flow
- [ ] Webhooks
- [ ] API endpoints (all v1 routes)
- [ ] LFS operations
- [ ] Admin panel functionality
- [ ] File uploads
- [ ] Session management
- [ ] CSRF protection
- [ ] i18n/localization

### 3. Integration Tests

Create integration test suite that covers:
- HTTP request/response cycle
- Middleware execution order
- Session persistence
- CSRF token validation
- Form binding and validation
- Template rendering
- Static file serving

### 4. Performance Testing

```bash
# Benchmark before migration
ab -n 1000 -c 10 http://localhost:3000/

# Benchmark after migration
ab -n 1000 -c 10 http://localhost:3000/

# Compare results
```

### 5. Security Testing

- [ ] CSRF protection works
- [ ] Session security maintained
- [ ] Authentication bypass tests
- [ ] XSS prevention
- [ ] SQL injection prevention (should be unchanged)

## Rollback Plan

### Quick Rollback

If critical issues are discovered:

1. **Git Revert**
   ```bash
   git revert <commit-range>
   git push
   ```

2. **go.mod Rollback**
   ```bash
   git checkout main -- go.mod go.sum
   go mod tidy
   ```

3. **Deploy Previous Version**
   - Use tagged release
   - Roll back to last stable commit

### Gradual Migration (Alternative Approach)

If full migration is too risky:

1. **Feature Flag**: Use build tags or environment variables
2. **Parallel Handlers**: Support both frameworks temporarily
3. **Incremental Migration**: Migrate module by module
4. **A/B Testing**: Route subset of traffic to new version

## Conclusion

### Summary

Migrating from Macaron to Flamego is a **significant but manageable** undertaking. Flamego provides excellent feature parity with Macaron, including all the middleware that Gogs currently uses (except toolbox, which is easy to replace).

### Key Advantages of Migration

✅ **Complete Feature Parity**: All required middleware is available
✅ **Same Philosophy**: Dependency injection pattern maintained  
✅ **Better Performance**: Improved routing engine  
✅ **Active Development**: Regular updates and improvements  
✅ **Official Successor**: Created by Macaron's author  
✅ **Better Routing**: More powerful routing capabilities  

### Remaining Concerns

⚠️ **Large Scope**: ~150+ files need modification  
⚠️ **Testing Burden**: Comprehensive testing required  
⚠️ **Learning Curve**: Team needs to learn new APIs  
⚠️ **Toolbox Replacement**: Need custom health check implementation  

### Recommendation

**Proceed with migration** using the phased approach outlined above. The migration is worthwhile because:

1. Flamego is the official successor to Macaron
2. All necessary middleware is available
3. The migration path is clear and well-documented
4. Long-term benefits outweigh short-term costs
5. Macaron is in maintenance mode only

### Next Steps

1. **Get team buy-in** on migration decision
2. **Allocate resources** (3-4 weeks of developer time)
3. **Create feature branch** and begin Phase 1
4. **Set up comprehensive test coverage** before starting
5. **Document progress** and issues encountered
6. **Plan staged rollout** to production

## References

- [Flamego Official Documentation](https://flamego.dev/)
- [Flamego GitHub](https://github.com/flamego/flamego)
- [Flamego vs Macaron Comparison](https://flamego.dev/faqs.html#how-is-flamego-different-from-macaron)
- [Macaron GitHub](https://github.com/go-macaron/macaron)

## Appendix: Quick Reference

### Import Changes

```go
// Old imports
import (
    "gopkg.in/macaron.v1"
    "github.com/go-macaron/binding"
    "github.com/go-macaron/cache"
    "github.com/go-macaron/captcha"
    "github.com/go-macaron/csrf"
    "github.com/go-macaron/gzip"
    "github.com/go-macaron/i18n"
    "github.com/go-macaron/session"
)

// New imports
import (
    "github.com/flamego/flamego"
    "github.com/flamego/binding"
    "github.com/flamego/cache"
    "github.com/flamego/captcha"
    "github.com/flamego/csrf"
    "github.com/flamego/gzip"
    "github.com/flamego/i18n"
    "github.com/flamego/session"
    "github.com/flamego/template"
)
```

### Common Pattern Changes

```go
// Route parameters
m.Get("/:username/:repo")          → f.Get("/<username>/<repo>")

// Handler signature
func(c *macaron.Context)           → func(c flamego.Context)

// Get parameter
c.Params(":username")              → c.Param("username")

// Response writer
c.Resp                             → c.ResponseWriter()

// Request
c.Req.Request                      → c.Request().Request

// Session interface
func(sess session.Store)           → func(sess session.Session)

// Template rendering
c.HTML(200, "tpl")                 → t.HTML(200, "tpl")
```
