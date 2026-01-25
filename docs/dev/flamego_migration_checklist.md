# Flamego Migration Checklist

This checklist provides a step-by-step guide for executing the migration from Macaron to Flamego.

## Pre-Migration

### 1. Team Preparation
- [ ] Review migration guide with entire team
- [ ] Ensure all developers understand Flamego basics
- [ ] Allocate 3-4 weeks for migration effort
- [ ] Schedule regular sync meetings during migration
- [ ] Identify rollback champion

### 2. Documentation Review
- [ ] Read [Flamego documentation](https://flamego.dev/)
- [ ] Review [migration examples](./flamego_migration_examples.md)
- [ ] Review [middleware documentation](https://flamego.dev/middleware/)
- [ ] Understand Flamego's dependency injection

### 3. Environment Setup
- [ ] Create feature branch: `feature/flamego-migration`
- [ ] Set up local development environment
- [ ] Verify current tests pass: `go test ./...`
- [ ] Document current test coverage: `go test -cover ./...`
- [ ] Benchmark current performance (optional)

### 4. Backup and Safety
- [ ] Tag current stable version: `git tag v0.14.0-pre-flamego`
- [ ] Create backup branch: `git branch backup/before-flamego`
- [ ] Document current behavior (screenshots, videos)
- [ ] Ensure CI/CD can roll back quickly

## Phase 1: Dependencies (Day 1)

### 1.1 Update go.mod

- [ ] Add Flamego core
  ```bash
  go get github.com/flamego/flamego@latest
  ```

- [ ] Add Flamego middleware
  ```bash
  go get github.com/flamego/binding@latest
  go get github.com/flamego/cache@latest
  go get github.com/flamego/captcha@latest
  go get github.com/flamego/csrf@latest
  go get github.com/flamego/gzip@latest
  go get github.com/flamego/i18n@latest
  go get github.com/flamego/session@latest
  go get github.com/flamego/template@latest
  ```

- [ ] Run `go mod tidy`
- [ ] Verify no conflicts
- [ ] Commit: `git commit -m "Add Flamego dependencies"`

### 1.2 Import Updates

- [ ] Create find-and-replace script
- [ ] Test script on sample file
- [ ] Document import mapping

## Phase 2: Core Framework (Days 2-3)

### 2.1 Main Application Setup

File: `internal/cmd/web.go`

- [ ] Update imports
  ```go
  // Remove
  "gopkg.in/macaron.v1"
  "github.com/go-macaron/*"
  
  // Add
  "github.com/flamego/flamego"
  "github.com/flamego/session"
  "github.com/flamego/csrf"
  // etc...
  ```

- [ ] Rename function: `newMacaron()` → `newFlamego()`
- [ ] Update initialization: `macaron.New()` → `flamego.New()`
- [ ] Update logger: `macaron.Logger()` → `flamego.Logger()`
- [ ] Update recovery: `macaron.Recovery()` → `flamego.Recovery()`
- [ ] Update gzip: Keep similar pattern
- [ ] Update static file serving
  ```go
  // Change from macaron.Static() to flamego.Static()
  f.Use(flamego.Static(...))
  ```

- [ ] Update renderer setup
  ```go
  // Change from macaron.Renderer() to template.Templater()
  f.Use(template.Templater(template.Options{...}))
  ```

- [ ] Test compilation: `go build`
- [ ] Fix any compilation errors
- [ ] Commit: `git commit -m "Migrate main app setup to Flamego"`

### 2.2 Middleware Configuration

Still in `internal/cmd/web.go`:

- [ ] Update i18n middleware
  - Change `Langs` → `Languages`
  - Change `DefaultLang` → `DefaultLanguage`
  - Change `SubURL` → `URLPrefix`

- [ ] Update cache middleware
  - Change from string adapter to config struct
  - Update method calls: `Put()` → `Set()`

- [ ] Update captcha middleware
  - Verify options compatibility
  - Test captcha generation

- [ ] Update toolbox functionality
  - Remove toolbox middleware
  - Create custom health check endpoint
  ```go
  f.Get("/-/health", func(c flamego.Context) {
      if err := database.Ping(); err != nil {
          c.ResponseWriter().WriteHeader(500)
          return
      }
      c.ResponseWriter().WriteHeader(200)
  })
  ```

- [ ] Update session middleware
  - Change `Provider` → Use config structs
  - Change interface: `session.Store` → `session.Session`
  - Test session persistence

- [ ] Update CSRF middleware
  - Verify options compatibility
  - Test token generation
  - Update method calls: `GetToken()` → `Token()`

- [ ] Test server starts: `go run gogs.go web`
- [ ] Verify middleware loads in correct order
- [ ] Commit: `git commit -m "Migrate middleware to Flamego"`

### 2.3 Route Definitions

Still in `internal/cmd/web.go`:

- [ ] Update basic routes: `:param` → `<param>`
- [ ] Update regex routes: `^:name(a|b)$` → `<name:a|b>`
- [ ] Test route compilation
- [ ] Verify route pattern matching

Routes to update:
- [ ] Home route: `/`
- [ ] Explore routes: `/explore/*`
- [ ] Install routes: `/install`
- [ ] User routes: `/user/*`
- [ ] Admin routes: `/admin/*`
- [ ] Org routes: `/org/*`
- [ ] Repo routes: `/:username/:reponame/*`
- [ ] API routes: `/api/*`

- [ ] Commit: `git commit -m "Update route syntax to Flamego"`

## Phase 3: Context System (Days 4-5)

### 3.1 Context Wrapper

File: `internal/context/context.go`

- [ ] Update imports
  ```go
  "github.com/flamego/flamego"
  "github.com/flamego/cache"
  "github.com/flamego/csrf"
  "github.com/flamego/session"
  ```

- [ ] Update Context struct
  ```go
  type Context struct {
      flamego.Context  // Embedded interface
      cache   cache.Cache
      csrf    csrf.CSRF
      flash   *session.Flash
      session session.Session
      // ... other fields
  }
  ```

- [ ] Add accessor methods
  ```go
  func (c *Context) Cache() cache.Cache { return c.cache }
  func (c *Context) CSRF() csrf.CSRF { return c.csrf }
  func (c *Context) Session() session.Session { return c.session }
  ```

- [ ] Update Contexter middleware signature
  ```go
  func Contexter(store Store) flamego.Handler {
      return func(
          ctx flamego.Context,
          cache cache.Cache,
          sess session.Session,
          // ... other injectables
      ) {
          // ...
      }
  }
  ```

- [ ] Update response methods
  - `c.HTML()` - needs template parameter or stored reference
  - `c.JSON()` - use ResponseWriter directly
  - `c.Redirect()` - should work as-is
  - `c.PlainText()` - use ResponseWriter
  - `c.ServeContent()` - use ResponseWriter

- [ ] Update parameter access
  - All `c.Params(":name")` → `c.Param("name")`

- [ ] Test compilation: `go build`
- [ ] Commit: `git commit -m "Migrate Context wrapper to Flamego"`

### 3.2 Other Context Files

Files in `internal/context/`:

- [ ] `auth.go` - Update handler signatures
- [ ] `api.go` - Update APIContext
- [ ] `user.go` - Update user context helpers
- [ ] `repo.go` - Update repository context
  - Fix all `c.Params()` calls
  - Update middleware signatures
- [ ] `org.go` - Update organization context
- [ ] `go_get.go` - Update go-get handler

For each file:
- [ ] Update imports
- [ ] Change `macaron.Handler` → `flamego.Handler`
- [ ] Change `*macaron.Context` → `flamego.Context` or `*Context`
- [ ] Update `c.Params(":name")` → `c.Param("name")`
- [ ] Fix compilation errors

- [ ] Commit after each file or group
- [ ] Final commit: `git commit -m "Complete context system migration"`

## Phase 4: Form Binding (Days 6-7)

### 4.1 Form Package

File: `internal/form/form.go`

- [ ] Update imports
  ```go
  "github.com/flamego/binding"
  ```

- [ ] Update custom validators
  ```go
  // Register custom validators with go-playground/validator
  binding.RegisterValidation("alphaDashDot", validatorFunc)
  ```

- [ ] Update `SetNameMapper` if used
- [ ] Test validator registration

### 4.2 Form Structs

Files: `internal/form/*.go`

For each file:
- [ ] `auth.go` - Update auth forms
- [ ] `admin.go` - Update admin forms
- [ ] `user.go` - Update user forms
- [ ] `repo.go` - Update repo forms
- [ ] `org.go` - Update org forms

For each form:
- [ ] Change `binding:"Required"` → `validate:"required"`
- [ ] Change `binding:"MaxSize(100)"` → `validate:"max=100"`
- [ ] Change `binding:"MinSize(5)"` → `validate:"min=5"`
- [ ] Update custom validators
- [ ] Test form validation

Pattern replacements:
- `binding:"Required"` → `validate:"required"`
- `binding:"AlphaDashDot"` → `validate:"alphaDashDot"`
- `binding:"MaxSize(N)"` → `validate:"max=N"`
- `binding:"MinSize(N)"` → `validate:"min=N"`
- `binding:"Email"` → `validate:"email"`
- `binding:"Url"` → `validate:"url"`

- [ ] Commit: `git commit -m "Migrate form binding to Flamego"`

## Phase 5: Route Handlers (Days 8-14)

### 5.1 User Routes

Files: `internal/route/user/*.go`

- [ ] `user.go` - Basic user handlers
  - Update handler signatures
  - Add template parameters
  - Fix parameter access
  
- [ ] `auth.go` - Login/logout handlers
  - Update session access: `sess.Get()` etc.
  - Fix CSRF token access
  
- [ ] `setting.go` - User settings
  - Update form binding usage
  - Fix template rendering

- [ ] Test user flows:
  - [ ] User registration
  - [ ] User login
  - [ ] User logout
  - [ ] Profile view
  - [ ] Settings update

- [ ] Commit: `git commit -m "Migrate user routes to Flamego"`

### 5.2 Repository Routes

Files: `internal/route/repo/*.go`

Priority files:
- [ ] `repo.go` - Main repo handler
- [ ] `home.go` - Repository home
- [ ] `issue.go` - Issue management
- [ ] `pull.go` - Pull requests
- [ ] `release.go` - Releases
- [ ] `webhook.go` - Webhooks
- [ ] `setting.go` - Repo settings
- [ ] `http.go` - HTTP Git operations

For each file:
- [ ] Update imports
- [ ] Update handler signatures
- [ ] Add template parameters where needed
- [ ] Fix `c.Params()` calls
- [ ] Update form binding calls

- [ ] Test repository flows:
  - [ ] Create repository
  - [ ] View repository
  - [ ] Create issue
  - [ ] Create pull request
  - [ ] Push via HTTP

- [ ] Commit: `git commit -m "Migrate repository routes to Flamego"`

### 5.3 Admin Routes

Files: `internal/route/admin/*.go`

- [ ] `admin.go` - Admin dashboard
- [ ] `user.go` - User management
- [ ] `org.go` - Organization management
- [ ] `repo.go` - Repository management
- [ ] `auth.go` - Auth source management
- [ ] `notice.go` - System notices

- [ ] Test admin flows:
  - [ ] Admin dashboard
  - [ ] Create user
  - [ ] Delete user
  - [ ] Manage auth sources

- [ ] Commit: `git commit -m "Migrate admin routes to Flamego"`

### 5.4 Organization Routes

Files: `internal/route/org/*.go`

- [ ] `org.go` - Organization handlers
- [ ] `team.go` - Team management
- [ ] `setting.go` - Org settings

- [ ] Test organization flows:
  - [ ] Create organization
  - [ ] Manage teams
  - [ ] Org settings

- [ ] Commit: `git commit -m "Migrate organization routes to Flamego"`

### 5.5 API Routes

Files: `internal/route/api/v1/*.go`

- [ ] `api.go` - API router setup
- [ ] `user/*.go` - User API endpoints
- [ ] `repo/*.go` - Repository API endpoints
- [ ] `org/*.go` - Organization API endpoints
- [ ] `admin/*.go` - Admin API endpoints

For API handlers:
- [ ] Update JSON response methods
- [ ] Ensure authentication works
- [ ] Test error responses

- [ ] Test API endpoints:
  - [ ] GET /api/v1/user
  - [ ] GET /api/v1/users/:username
  - [ ] GET /api/v1/repos/:owner/:repo
  - [ ] Create/Update operations

- [ ] Commit: `git commit -m "Migrate API routes to Flamego"`

### 5.6 LFS Routes

Files: `internal/route/lfs/*.go`

- [ ] `route.go` - LFS router
- [ ] `basic.go` - Basic auth
- [ ] `batch.go` - Batch API
- [ ] Update tests in `*_test.go`

- [ ] Test LFS operations
- [ ] Commit: `git commit -m "Migrate LFS routes to Flamego"`

### 5.7 Other Routes

Files: `internal/route/*.go`

- [ ] `home.go` - Home page
- [ ] `install.go` - Installation
- [ ] `dev/*.go` - Development tools

- [ ] Commit: `git commit -m "Migrate remaining routes to Flamego"`

## Phase 6: Testing (Days 15-18)

### 6.1 Unit Tests

- [ ] Update test helpers
  - Create mock flamego.Context
  - Update test fixtures

- [ ] Run unit tests: `go test ./internal/context/...`
- [ ] Run unit tests: `go test ./internal/form/...`
- [ ] Run unit tests: `go test ./internal/route/...`

- [ ] Fix failing tests one by one
- [ ] Document test changes
- [ ] Commit: `git commit -m "Fix unit tests for Flamego"`

### 6.2 Integration Tests

- [ ] Update integration test setup
- [ ] Test complete user flows
  - [ ] Registration → Login → Create Repo → Push → Pull
  
- [ ] Test admin flows
  - [ ] Admin login → User management
  
- [ ] Test API flows
  - [ ] Token auth → API calls

- [ ] Commit: `git commit -m "Fix integration tests for Flamego"`

### 6.3 Manual Testing

Create test plan document covering:

Web UI:
- [ ] Homepage loads
- [ ] User registration works
- [ ] User login works
- [ ] User logout works
- [ ] Profile viewing works
- [ ] Repository creation works
- [ ] Repository viewing works
- [ ] Issue creation works
- [ ] Issue commenting works
- [ ] Pull request creation works
- [ ] Pull request merging works
- [ ] Webhooks work
- [ ] LFS operations work
- [ ] File uploads work
- [ ] Avatar uploads work
- [ ] Admin panel works
- [ ] Organization creation works
- [ ] Team management works

Git Operations:
- [ ] HTTP clone works
- [ ] HTTP push works
- [ ] HTTP pull works
- [ ] SSH clone works
- [ ] SSH push works
- [ ] SSH pull works

API:
- [ ] Authentication works
- [ ] All v1 endpoints work
- [ ] Error responses correct

Security:
- [ ] CSRF protection works
- [ ] Session security works
- [ ] Auth required endpoints protected

Localization:
- [ ] Language switching works
- [ ] Translations load correctly

- [ ] Document any issues found
- [ ] Create issues for bugs
- [ ] Commit fixes as they're made

### 6.4 Performance Testing

- [ ] Benchmark homepage
- [ ] Benchmark repository view
- [ ] Benchmark API endpoints
- [ ] Compare with pre-migration benchmarks
- [ ] Document performance differences
- [ ] Optimize if needed

## Phase 7: Cleanup (Days 19-20)

### 7.1 Remove Old Code

- [ ] Remove all Macaron imports
  ```bash
  grep -r "gopkg.in/macaron.v1" .
  grep -r "github.com/go-macaron/" .
  ```

- [ ] Remove from go.mod
  ```bash
  go mod edit -droprequire gopkg.in/macaron.v1
  go mod edit -droprequire github.com/go-macaron/binding
  # etc...
  ```

- [ ] Run `go mod tidy`
- [ ] Verify unused dependencies removed
- [ ] Commit: `git commit -m "Remove Macaron dependencies"`

### 7.2 Code Quality

- [ ] Run linter: `golangci-lint run`
- [ ] Fix linter issues
- [ ] Run `go fmt ./...`
- [ ] Run `go vet ./...`
- [ ] Check for TODO/FIXME comments
- [ ] Commit: `git commit -m "Code quality improvements"`

### 7.3 Documentation

- [ ] Update README.md if needed
- [ ] Update CONTRIBUTING.md if needed
- [ ] Update development documentation
- [ ] Document migration in CHANGELOG.md
- [ ] Create migration announcement
- [ ] Commit: `git commit -m "Update documentation for Flamego"`

### 7.4 Final Review

- [ ] Review all changes
- [ ] Ensure no debug code left
- [ ] Verify test coverage maintained
- [ ] Check for security issues
- [ ] Run final test suite: `go test ./...`
- [ ] Run final manual tests

## Phase 8: Deployment (Days 21-22)

### 8.1 Pre-Deployment

- [ ] Create release candidate tag
- [ ] Deploy to staging environment
- [ ] Run smoke tests on staging
- [ ] Performance test on staging
- [ ] Security scan
- [ ] Get team approval

### 8.2 Deployment

- [ ] Schedule deployment window
- [ ] Notify users of maintenance
- [ ] Take backup of production
- [ ] Deploy new version
- [ ] Monitor logs
- [ ] Run smoke tests on production
- [ ] Monitor performance metrics

### 8.3 Post-Deployment

- [ ] Monitor for issues (24-48 hours)
- [ ] Check error rates
- [ ] Verify all features working
- [ ] Collect user feedback
- [ ] Address any urgent issues

## Rollback Procedure

If critical issues occur:

### Quick Rollback
1. [ ] Stop application
2. [ ] Restore from backup
3. [ ] Start application
4. [ ] Verify functionality
5. [ ] Notify users

### Git Rollback
1. [ ] Identify last good commit
2. [ ] `git revert <commit-range>`
3. [ ] `git push`
4. [ ] Deploy reverted version

### Issues to Watch For
- [ ] Session persistence issues
- [ ] CSRF token validation failures
- [ ] Form validation errors
- [ ] Template rendering errors
- [ ] Performance degradation
- [ ] Memory leaks
- [ ] Authentication bypass

## Success Criteria

Migration is successful when:

- [ ] All tests pass
- [ ] All manual test cases pass
- [ ] Performance is equal or better
- [ ] No security regressions
- [ ] No functionality lost
- [ ] Code quality maintained
- [ ] Documentation updated
- [ ] Team trained on new code

## Notes Section

Use this section to track:
- Issues encountered
- Solutions found
- Time spent on each phase
- Lessons learned
- Tips for future migrations

---

**Migration Started:** _____________  
**Migration Completed:** _____________  
**Total Time:** _____________  
**Team Members:** _____________  
**Issues Created:** _____________  
**Issues Resolved:** _____________
