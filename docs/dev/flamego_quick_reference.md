# Macaron vs Flamego: Quick Reference

This document provides quick lookup tables for common migration patterns.

## At a Glance

| Aspect | Macaron | Flamego | Status |
|--------|---------|---------|--------|
| **Creator** | Unknwon | Unknwon | ✅ Same author |
| **Status** | Maintenance only | Active development | ⚠️ Macaron deprecated |
| **Go Version** | 1.11+ | 1.19+ | 📈 Modern |
| **Philosophy** | Dependency injection | Dependency injection | ✅ Same |
| **Performance** | Good | Better | 📈 Improved |
| **Routing** | Basic | Advanced | 📈 Enhanced |

## Import Mapping

| Macaron Package | Flamego Package | Notes |
|----------------|-----------------|-------|
| `gopkg.in/macaron.v1` | `github.com/flamego/flamego` | Core framework |
| `github.com/go-macaron/binding` | `github.com/flamego/binding` | Form binding |
| `github.com/go-macaron/cache` | `github.com/flamego/cache` | Caching |
| `github.com/go-macaron/captcha` | `github.com/flamego/captcha` | Captcha |
| `github.com/go-macaron/csrf` | `github.com/flamego/csrf` | CSRF protection |
| `github.com/go-macaron/gzip` | `github.com/flamego/gzip` | Gzip compression |
| `github.com/go-macaron/i18n` | `github.com/flamego/i18n` | Internationalization |
| `github.com/go-macaron/session` | `github.com/flamego/session` | Session management |
| Built-in | `github.com/flamego/template` | Template rendering |
| `github.com/go-macaron/toolbox` | ❌ Custom implementation | Health checks |

## Type Mapping

| Macaron Type | Flamego Type | Change |
|--------------|--------------|--------|
| `*macaron.Macaron` | `*flamego.Flame` | Main app type |
| `macaron.Context` | `flamego.Context` | Interface vs pointer |
| `macaron.Handler` | `flamego.Handler` | Same concept |
| `session.Store` | `session.Session` | Interface name |
| `csrf.CSRF` | `csrf.CSRF` | Same |
| `cache.Cache` | `cache.Cache` | Same |

## Method Mapping

### Core Methods

| Operation | Macaron | Flamego |
|-----------|---------|---------|
| Create app | `macaron.New()` | `flamego.New()` |
| Classic setup | `macaron.Classic()` | `flamego.Classic()` |
| Add middleware | `m.Use(handler)` | `f.Use(handler)` |
| GET route | `m.Get(path, h)` | `f.Get(path, h)` |
| POST route | `m.Post(path, h)` | `f.Post(path, h)` |
| Route group | `m.Group(path, fn)` | `f.Group(path, fn)` |
| Combo route | `m.Combo(path)` | `f.Combo(path)` |
| Start server | `http.ListenAndServe(addr, m)` | `f.Run(addr)` |

### Context Methods

| Operation | Macaron | Flamego |
|-----------|---------|---------|
| Get param | `c.Params(":name")` | `c.Param("name")` |
| Get query | `c.Query("key")` | `c.Query("key")` |
| Get request | `c.Req` | `c.Request()` |
| Get response | `c.Resp` | `c.ResponseWriter()` |
| Redirect | `c.Redirect(url)` | `c.Redirect(url)` |
| Set cookie | `c.SetCookie(...)` | `c.SetCookie(...)` |
| Get cookie | `c.GetCookie(name)` | `c.Cookie(name)` |

### Session Methods

| Operation | Macaron | Flamego |
|-----------|---------|---------|
| Set value | `sess.Set(k, v)` | `sess.Set(k, v)` |
| Get value | `sess.Get(k)` | `sess.Get(k)` |
| Delete | `sess.Delete(k)` | `sess.Delete(k)` |
| ID | `sess.ID()` | `sess.ID()` |
| Flush | `sess.Flush()` | `sess.Flush()` |

### CSRF Methods

| Operation | Macaron | Flamego |
|-----------|---------|---------|
| Get token | `x.GetToken()` | `x.Token()` |
| Validate | Automatic | Automatic |

### Cache Methods

| Operation | Macaron | Flamego |
|-----------|---------|---------|
| Set value | `c.Put(k, v, timeout)` | `c.Set(k, v, timeout)` |
| Get value | `c.Get(k)` | `c.Get(k)` |
| Delete | `c.Delete(k)` | `c.Delete(k)` |
| Flush | `c.Flush()` | `c.Flush()` |

## Route Syntax

| Feature | Macaron | Flamego | Example |
|---------|---------|---------|---------|
| Basic param | `:param` | `<param>` | `/:id` → `/<id>` |
| Regex param | `^:name(a\|b)$` | `<name:a\|b>` | Pattern matching |
| Optional param | Multiple routes | `?<param>` | `/wiki/?<page>` |
| Wildcard | `:path(*)` | `<**path>` | Glob pattern |

### Before (Macaron)
```go
m.Get("/", handler)                          // Root
m.Get("/:username", handler)                 // Basic param
m.Get("/:username/:repo", handler)           // Multiple params
m.Get("/^:type(issues|pulls)$", handler)     // Regex
```

### After (Flamego)
```go
f.Get("/", handler)                          // Root
f.Get("/<username>", handler)                // Basic param
f.Get("/<username>/<repo>", handler)         // Multiple params
f.Get("/<type:issues|pulls>", handler)       // Regex
```

## Handler Signatures

### Basic Handler

**Macaron:**
```go
func Handler(c *macaron.Context) {
    c.JSON(200, map[string]string{"msg": "hello"})
}
```

**Flamego:**
```go
func Handler(c flamego.Context) {
    c.ResponseWriter().Header().Set("Content-Type", "application/json")
    c.ResponseWriter().WriteHeader(200)
    json.NewEncoder(c.ResponseWriter()).Encode(map[string]string{"msg": "hello"})
}
```

### With Custom Context

**Macaron:**
```go
func Handler(c *context.Context) {
    c.Data["Title"] = "Page"
    c.HTML(200, "template")
}
```

**Flamego:**
```go
func Handler(c *context.Context, t template.Template, data template.Data) {
    data["Title"] = "Page"
    t.HTML(200, "template")
}
```

### With Session

**Macaron:**
```go
func Handler(c *context.Context, sess session.Store) {
    sess.Set("key", "value")
}
```

**Flamego:**
```go
func Handler(c *context.Context, sess session.Session) {
    sess.Set("key", "value")
}
```

### With Form Binding

**Macaron:**
```go
func Handler(c *context.Context, form Form) {
    // Use form
}

// Route
m.Post("/", binding.Bind(Form{}), Handler)
```

**Flamego:**
```go
func Handler(c *context.Context, form Form) {
    // Use form
}

// Route
f.Post("/", binding.Form(Form{}), Handler)
```

## Form Tags

| Validation | Macaron | Flamego |
|------------|---------|---------|
| Required | `binding:"Required"` | `validate:"required"` |
| Email | `binding:"Email"` | `validate:"email"` |
| URL | `binding:"Url"` | `validate:"url"` |
| Min length | `binding:"MinSize(5)"` | `validate:"min=5"` |
| Max length | `binding:"MaxSize(100)"` | `validate:"max=100"` |
| Range | `binding:"Range(1,10)"` | `validate:"min=1,max=10"` |
| Alpha | `binding:"Alpha"` | `validate:"alpha"` |
| AlphaDash | `binding:"AlphaDash"` | `validate:"alphanum"` |

### Before (Macaron)
```go
type LoginForm struct {
    Username string `form:"username" binding:"Required;AlphaDash"`
    Password string `form:"password" binding:"Required;MinSize(6)"`
    Email    string `form:"email" binding:"Email"`
}
```

### After (Flamego)
```go
type LoginForm struct {
    Username string `form:"username" validate:"required,alphanum"`
    Password string `form:"password" validate:"required,min=6"`
    Email    string `form:"email" validate:"email"`
}
```

## Middleware Configuration

### Session

**Macaron:**
```go
m.Use(session.Sessioner(session.Options{
    Provider:       "memory",
    ProviderConfig: "",
    CookieName:     "session_id",
    CookiePath:     "/",
    Gclifetime:     3600,
    Maxlifetime:    3600,
}))
```

**Flamego:**
```go
f.Use(session.Sessioner(session.Options{
    Config: session.MemoryConfig{
        GCInterval: 3600,
    },
    Cookie: session.CookieOptions{
        Name:   "session_id",
        Path:   "/",
        MaxAge: 3600,
    },
}))
```

### CSRF

**Macaron:**
```go
m.Use(csrf.Csrfer(csrf.Options{
    Secret:     "secret-key",
    Cookie:     "_csrf",
    CookiePath: "/",
    SetCookie:  true,
    Secure:     false,
}))
```

**Flamego:**
```go
f.Use(csrf.Csrfer(csrf.Options{
    Secret:     "secret-key",
    Cookie:     "_csrf",
    CookiePath: "/",
    Secure:     false,
}))
```

### Cache

**Macaron:**
```go
m.Use(cache.Cacher(cache.Options{
    Adapter:       "memory",
    AdapterConfig: "",
    Interval:      60,
}))
```

**Flamego:**
```go
f.Use(cache.Cacher(cache.Options{
    Config: cache.MemoryConfig{
        GCInterval: 60,
    },
}))
```

### Template

**Macaron:**
```go
m.Use(macaron.Renderer(macaron.RenderOptions{
    Directory: "templates",
    Funcs:     template.FuncMap(),
}))
```

**Flamego:**
```go
f.Use(template.Templater(template.Options{
    Directory: "templates",
    FuncMaps:  []template.FuncMap{template.FuncMap()},
}))
```

### i18n

**Macaron:**
```go
m.Use(i18n.I18n(i18n.Options{
    SubURL:      "/",
    Langs:       []string{"en-US", "zh-CN"},
    Names:       []string{"English", "简体中文"},
    DefaultLang: "en-US",
}))
```

**Flamego:**
```go
f.Use(i18n.I18n(i18n.Options{
    URLPrefix:       "/",
    Languages:       []string{"en-US", "zh-CN"},
    Names:           []string{"English", "简体中文"},
    DefaultLanguage: "en-US",
}))
```

## Common Patterns

### Pattern: Get User by Username

**Macaron:**
```go
func UserProfile(c *context.Context) {
    username := c.Params(":username")
    user, err := database.GetUserByName(username)
    if err != nil {
        c.NotFoundOrError(err, "get user")
        return
    }
    c.Data["User"] = user
    c.HTML(200, "user/profile")
}
```

**Flamego:**
```go
func UserProfile(c *context.Context, t template.Template, data template.Data) {
    username := c.Param("username")
    user, err := database.GetUserByName(username)
    if err != nil {
        c.NotFoundOrError(err, "get user")
        return
    }
    data["User"] = user
    t.HTML(200, "user/profile")
}
```

### Pattern: Form Submission

**Macaron:**
```go
// Form struct
type CreateRepoForm struct {
    Name string `form:"name" binding:"Required;AlphaDashDot"`
}

// Route
m.Post("/repo/create", binding.Bind(CreateRepoForm{}), CreateRepoPost)

// Handler
func CreateRepoPost(c *context.Context, form CreateRepoForm) {
    if c.HasError() {
        c.RenderWithErr(c.GetErrMsg(), "repo/create", &form)
        return
    }
    // Create repo...
    c.Redirect("/")
}
```

**Flamego:**
```go
// Form struct
type CreateRepoForm struct {
    Name string `form:"name" validate:"required,alphaDashDot"`
}

// Route
f.Post("/repo/create", binding.Form(CreateRepoForm{}), CreateRepoPost)

// Handler
func CreateRepoPost(c *context.Context, form CreateRepoForm, t template.Template, data template.Data) {
    if c.HasError() {
        c.RenderWithErr(c.GetErrMsg(), "repo/create", &form, t, data)
        return
    }
    // Create repo...
    c.Redirect("/")
}
```

### Pattern: JSON API

**Macaron:**
```go
func APIHandler(c *context.APIContext) {
    data := map[string]any{
        "id":   123,
        "name": "example",
    }
    c.JSON(200, data)
}
```

**Flamego:**
```go
func APIHandler(c *context.APIContext) {
    data := map[string]any{
        "id":   123,
        "name": "example",
    }
    
    c.ResponseWriter().Header().Set("Content-Type", "application/json")
    c.ResponseWriter().WriteHeader(200)
    json.NewEncoder(c.ResponseWriter()).Encode(data)
}

// Or create helper method on APIContext
func (c *APIContext) JSON(status int, v any) error {
    c.ResponseWriter().Header().Set("Content-Type", "application/json")
    c.ResponseWriter().WriteHeader(status)
    return json.NewEncoder(c.ResponseWriter()).Encode(v)
}
```

### Pattern: Middleware Chain

**Macaron:**
```go
m.Group("/repo", func() {
    m.Get("/create", repo.Create)
    m.Post("/create", binding.Bind(form.CreateRepo{}), repo.CreatePost)
}, reqSignIn, context.RepoAssignment())
```

**Flamego:**
```go
f.Group("/repo", func() {
    f.Get("/create", repo.Create)
    f.Post("/create", binding.Form(form.CreateRepo{}), repo.CreatePost)
}, reqSignIn, context.RepoAssignment())
```

## Error Handling

### Not Found

**Macaron:**
```go
func Handler(c *context.Context) {
    user, err := getUser()
    if err != nil {
        if isNotFound(err) {
            c.NotFound()
            return
        }
        c.Error(err, "get user")
        return
    }
}
```

**Flamego:**
```go
func Handler(c *context.Context) {
    user, err := getUser()
    if err != nil {
        if isNotFound(err) {
            c.NotFound()
            return
        }
        c.Error(err, "get user")
        return
    }
}
```

## Testing

### Mock Context

**Macaron:**
```go
import "gopkg.in/macaron.v1"

func TestHandler(t *testing.T) {
    m := macaron.New()
    req, _ := http.NewRequest("GET", "/", nil)
    resp := httptest.NewRecorder()
    m.ServeHTTP(resp, req)
}
```

**Flamego:**
```go
import "github.com/flamego/flamego"

func TestHandler(t *testing.T) {
    f := flamego.New()
    req, _ := http.NewRequest("GET", "/", nil)
    resp := httptest.NewRecorder()
    f.ServeHTTP(resp, req)
}
```

## Migration Checklist (Quick)

- [ ] Update imports
- [ ] Change `:param` → `<param>` in routes
- [ ] Change `macaron.Handler` → `flamego.Handler`
- [ ] Change `*macaron.Context` → `flamego.Context`
- [ ] Change `c.Params(":name")` → `c.Param("name")`
- [ ] Change `c.Resp` → `c.ResponseWriter()`
- [ ] Change `c.Req` → `c.Request()`
- [ ] Change `session.Store` → `session.Session`
- [ ] Change `x.GetToken()` → `x.Token()`
- [ ] Change `cache.Put()` → `cache.Set()`
- [ ] Add template parameters to handlers
- [ ] Update form validation tags
- [ ] Test everything!

## Common Pitfalls

| Issue | Solution |
|-------|----------|
| Forgot to remove `:` from param name | Use `c.Param("name")` not `c.Param(":name")` |
| Template not rendering | Add `template.Template` and `template.Data` to handler |
| Session not working | Changed interface from `Store` to `Session` |
| CSRF validation fails | Use `Token()` not `GetToken()` |
| Cache not working | Use `Set()` not `Put()` |
| Form validation errors | Update tags: `binding` → `validate` |
| Context methods fail | Use methods not fields: `c.ResponseWriter()` not `c.Resp` |

## Performance Notes

### Flamego Advantages

1. **O(1) static route lookup** - Faster than Macaron's tree
2. **Better regex handling** - Compiled patterns cached
3. **Reduced allocations** - More efficient memory usage
4. **Faster middleware chain** - Optimized injection

### Expected Improvements

- 10-30% faster route matching for static routes
- 5-15% faster overall request handling
- Slightly lower memory usage
- Better scalability under load

## Support and Resources

| Need Help? | Resource |
|------------|----------|
| Official Docs | https://flamego.dev/ |
| API Reference | https://pkg.go.dev/github.com/flamego/flamego |
| GitHub | https://github.com/flamego/flamego |
| Middleware | https://github.com/flamego (multiple repos) |
| FAQ | https://flamego.dev/faqs.html |
| Examples | https://github.com/flamego/flamego/tree/main/_examples |

## Version Information

| Framework | Current Version | Release Date | Status |
|-----------|----------------|--------------|--------|
| Macaron | v1.5.0 | 2021 | Maintenance |
| Flamego | v1.9.0+ | 2024 | Active |

---

**Last Updated:** 2026-01-25  
**Applies to:** Gogs migration from Macaron to Flamego
