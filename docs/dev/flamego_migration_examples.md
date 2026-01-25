# Flamego Migration: Code Examples

This document provides practical, side-by-side code examples showing how to migrate from Macaron to Flamego in the Gogs codebase.

## Table of Contents

1. [Basic Application Setup](#basic-application-setup)
2. [Middleware Configuration](#middleware-configuration)
3. [Route Definitions](#route-definitions)
4. [Handler Functions](#handler-functions)
5. [Context Usage](#context-usage)
6. [Form Binding](#form-binding)
7. [Template Rendering](#template-rendering)
8. [Custom Middleware](#custom-middleware)
9. [Complete Example](#complete-example)

## Basic Application Setup

### Macaron (Current)

```go
// internal/cmd/web.go
package cmd

import (
    "gopkg.in/macaron.v1"
    "github.com/go-macaron/session"
    "github.com/go-macaron/csrf"
)

func newMacaron() *macaron.Macaron {
    m := macaron.New()
    
    // Basic middleware
    if !conf.Server.DisableRouterLog {
        m.Use(macaron.Logger())
    }
    m.Use(macaron.Recovery())
    
    // Optional gzip
    if conf.Server.EnableGzip {
        m.Use(gzip.Gziper())
    }
    
    return m
}

func runWeb(c *cli.Context) error {
    m := newMacaron()
    
    // Configure routes
    m.Get("/", route.Home)
    
    // Start server
    return http.ListenAndServe(":3000", m)
}
```

### Flamego (Target)

```go
// internal/cmd/web.go
package cmd

import (
    "github.com/flamego/flamego"
    "github.com/flamego/session"
    "github.com/flamego/csrf"
)

func newFlamego() *flamego.Flame {
    f := flamego.New()
    
    // Basic middleware
    if !conf.Server.DisableRouterLog {
        f.Use(flamego.Logger())
    }
    f.Use(flamego.Recovery())
    
    // Optional gzip
    if conf.Server.EnableGzip {
        f.Use(gzip.Gziper())
    }
    
    return f
}

func runWeb(c *cli.Context) error {
    f := newFlamego()
    
    // Configure routes
    f.Get("/", route.Home)
    
    // Start server
    return f.Run(":3000")
}
```

## Middleware Configuration

### Session Middleware

#### Macaron (Current)

```go
import "github.com/go-macaron/session"

m.Use(session.Sessioner(session.Options{
    Provider:       conf.Session.Provider,
    ProviderConfig: conf.Session.ProviderConfig,
    CookieName:     conf.Session.CookieName,
    CookiePath:     conf.Server.Subpath,
    Gclifetime:     conf.Session.GCInterval,
    Maxlifetime:    conf.Session.MaxLifeTime,
    Secure:         conf.Session.CookieSecure,
}))

// Handler usage
func handler(sess session.Store) {
    sess.Set("user_id", 123)
    userID := sess.Get("user_id")
}
```

#### Flamego (Target)

```go
import "github.com/flamego/session"

f.Use(session.Sessioner(session.Options{
    // Config depends on provider type
    Config: session.RedisConfig{
        Options: &redis.Options{
            Addr: conf.Session.ProviderConfig,
        },
    },
    Cookie: session.CookieOptions{
        Name:     conf.Session.CookieName,
        Path:     conf.Server.Subpath,
        MaxAge:   conf.Session.MaxLifeTime,
        Secure:   conf.Session.CookieSecure,
    },
    // For memory provider:
    // Config: session.MemoryConfig{
    //     GCInterval: conf.Session.GCInterval,
    // },
}))

// Handler usage - interface name changed
func handler(sess session.Session) {
    sess.Set("user_id", 123)
    userID := sess.Get("user_id")
}
```

### CSRF Middleware

#### Macaron (Current)

```go
import "github.com/go-macaron/csrf"

m.Use(csrf.Csrfer(csrf.Options{
    Secret:         conf.Security.SecretKey,
    Header:         "X-CSRF-Token",
    Cookie:         conf.Session.CSRFCookieName,
    CookieDomain:   conf.Server.URL.Hostname(),
    CookiePath:     conf.Server.Subpath,
    CookieHttpOnly: true,
    SetCookie:      true,
    Secure:         conf.Server.URL.Scheme == "https",
}))

// Handler usage
func handler(x csrf.CSRF) {
    token := x.GetToken()
}
```

#### Flamego (Target)

```go
import "github.com/flamego/csrf"

f.Use(csrf.Csrfer(csrf.Options{
    Secret:     conf.Security.SecretKey,
    Header:     "X-CSRF-Token",
    Cookie:     conf.Session.CSRFCookieName,
    CookiePath: conf.Server.Subpath,
    Secure:     conf.Server.URL.Scheme == "https",
}))

// Handler usage - method name changed
func handler(x csrf.CSRF) {
    token := x.Token()  // Changed from GetToken()
}
```

### Template Middleware

#### Macaron (Current)

```go
m.Use(macaron.Renderer(macaron.RenderOptions{
    Directory:         filepath.Join(conf.WorkDir(), "templates"),
    AppendDirectories: []string{customDir},
    Funcs:             template.FuncMap(),
    IndentJSON:        macaron.Env != macaron.PROD,
}))

// Handler usage
func handler(c *macaron.Context) {
    c.Data["Title"] = "Home"
    c.HTML(200, "home")
}
```

#### Flamego (Target)

```go
import "github.com/flamego/template"

f.Use(template.Templater(template.Options{
    Directory:          filepath.Join(conf.WorkDir(), "templates"),
    AppendDirectories:  []string{customDir},
    FuncMaps:           []template.FuncMap{template.FuncMap()},
}))

// Handler usage - separate template and data injection
func handler(t template.Template, data template.Data) {
    data["Title"] = "Home"
    t.HTML(200, "home")
}
```

### Cache Middleware

#### Macaron (Current)

```go
import "github.com/go-macaron/cache"

m.Use(cache.Cacher(cache.Options{
    Adapter:       conf.Cache.Adapter,
    AdapterConfig: conf.Cache.Host,
    Interval:      conf.Cache.Interval,
}))

// Handler usage
func handler(cache cache.Cache) {
    cache.Put("key", "value", 60)
    value := cache.Get("key")
    cache.Delete("key")
}
```

#### Flamego (Target)

```go
import "github.com/flamego/cache"

var cacheConfig cache.Config
switch conf.Cache.Adapter {
case "memory":
    cacheConfig = cache.MemoryConfig{
        GCInterval: conf.Cache.Interval,
    }
case "redis":
    cacheConfig = cache.RedisConfig{
        Options: &redis.Options{
            Addr: conf.Cache.Host,
        },
    }
}

f.Use(cache.Cacher(cache.Options{
    Config: cacheConfig,
}))

// Handler usage - method names changed
func handler(c cache.Cache) {
    c.Set("key", "value", 60)  // Changed from Put
    value := c.Get("key")
    c.Delete("key")
}
```

### i18n Middleware

#### Macaron (Current)

```go
import "github.com/go-macaron/i18n"

m.Use(i18n.I18n(i18n.Options{
    SubURL:          conf.Server.Subpath,
    Files:           localeFiles,
    CustomDirectory: filepath.Join(conf.CustomDir(), "conf", "locale"),
    Langs:           conf.I18n.Langs,
    Names:           conf.I18n.Names,
    DefaultLang:     "en-US",
    Redirect:        true,
}))

// Handler usage
func handler(l i18n.Locale) {
    text := l.Tr("user.login")
}
```

#### Flamego (Target)

```go
import "github.com/flamego/i18n"

f.Use(i18n.I18n(i18n.Options{
    URLPrefix:       conf.Server.Subpath,
    Files:           localeFiles,
    CustomDirectory: filepath.Join(conf.CustomDir(), "conf", "locale"),
    Languages:       conf.I18n.Langs,  // Changed from Langs
    Names:           conf.I18n.Names,
    DefaultLanguage: "en-US",          // Changed from DefaultLang
    Redirect:        true,
}))

// Handler usage - same interface
func handler(l i18n.Locale) {
    text := l.Tr("user.login")
}
```

## Route Definitions

### Basic Routes

#### Macaron (Current)

```go
m.Get("/", ignSignIn, route.Home)
m.Post("/login", bindIgnErr(form.SignIn{}), user.LoginPost)
m.Get("/:username", user.Profile)
m.Get("/:username/:reponame", context.RepoAssignment(), repo.Home)
```

#### Flamego (Target)

```go
f.Get("/", ignSignIn, route.Home)
f.Post("/login", binding.Form(form.SignIn{}), user.LoginPost)
f.Get("/<username>", user.Profile)
f.Get("/<username>/<reponame>", context.RepoAssignment(), repo.Home)
```

**Key Changes:**
- `:param` becomes `<param>`
- `bindIgnErr(form)` becomes `binding.Form(form)`

### Route Groups

#### Macaron (Current)

```go
m.Group("/user", func() {
    m.Group("/login", func() {
        m.Combo("").Get(user.Login).
            Post(bindIgnErr(form.SignIn{}), user.LoginPost)
        m.Combo("/two_factor").Get(user.LoginTwoFactor).
            Post(user.LoginTwoFactorPost)
    })
    
    m.Get("/sign_up", user.SignUp)
    m.Post("/sign_up", bindIgnErr(form.Register{}), user.SignUpPost)
}, reqSignOut)
```

#### Flamego (Target)

```go
f.Group("/user", func() {
    f.Group("/login", func() {
        f.Combo("").Get(user.Login).
            Post(binding.Form(form.SignIn{}), user.LoginPost)
        f.Combo("/two_factor").Get(user.LoginTwoFactor).
            Post(user.LoginTwoFactorPost)
    })
    
    f.Get("/sign_up", user.SignUp)
    f.Post("/sign_up", binding.Form(form.Register{}), user.SignUpPost)
}, reqSignOut)
```

**Key Changes:**
- `m.Group` becomes `f.Group`
- `bindIgnErr` becomes `binding.Form`

### Regex Routes

#### Macaron (Current)

```go
m.Get("/^:type(issues|pulls)$", reqSignIn, user.Issues)
```

#### Flamego (Target)

```go
f.Get("/<type:issues|pulls>", reqSignIn, user.Issues)
```

### Route with Optional Segments

#### Macaron (Current)

```go
// Not well supported - need multiple routes
m.Get("/wiki", repo.Wiki)
m.Get("/wiki/:page", repo.Wiki)
```

#### Flamego (Target)

```go
// Better support for optional segments
f.Get("/wiki/?<page>", repo.Wiki)
```

## Handler Functions

### Basic Handler

#### Macaron (Current)

```go
func Home(c *context.Context) {
    c.Data["Title"] = "Home"
    c.HTML(http.StatusOK, "home")
}
```

#### Flamego (Target)

```go
func Home(c *context.Context, t template.Template, data template.Data) {
    data["Title"] = "Home"
    t.HTML(http.StatusOK, "home")
}

// Note: context.Context needs to be updated to wrap flamego.Context
```

### Handler with Parameters

#### Macaron (Current)

```go
func UserProfile(c *context.Context) {
    username := c.Params(":username")
    
    user, err := database.GetUserByName(username)
    if err != nil {
        c.NotFoundOrError(err, "get user")
        return
    }
    
    c.Data["User"] = user
    c.HTML(http.StatusOK, "user/profile")
}
```

#### Flamego (Target)

```go
func UserProfile(c *context.Context, t template.Template, data template.Data) {
    username := c.Param("username")  // No colon prefix
    
    user, err := database.GetUserByName(username)
    if err != nil {
        c.NotFoundOrError(err, "get user")
        return
    }
    
    data["User"] = user
    t.HTML(http.StatusOK, "user/profile")
}
```

### Handler with Form Binding

#### Macaron (Current)

```go
type LoginForm struct {
    Username string `form:"username" binding:"Required"`
    Password string `form:"password" binding:"Required"`
}

func LoginPost(c *context.Context, form LoginForm) {
    if !database.ValidateUser(form.Username, form.Password) {
        c.RenderWithErr("Invalid credentials", "user/login", &form)
        return
    }
    
    c.Session.Set("user_id", user.ID)
    c.Redirect("/")
}
```

#### Flamego (Target)

```go
type LoginForm struct {
    Username string `form:"username" validate:"required"`
    Password string `form:"password" validate:"required"`
}

func LoginPost(c *context.Context, form LoginForm, t template.Template, data template.Data) {
    if !database.ValidateUser(form.Username, form.Password) {
        c.RenderWithErr("Invalid credentials", "user/login", &form, t, data)
        return
    }
    
    c.Session().Set("user_id", user.ID)
    c.Redirect("/")
}
```

### Handler with Session

#### Macaron (Current)

```go
func RequireLogin(c *context.Context, sess session.Store) {
    userID := sess.Get("user_id")
    if userID == nil {
        c.Redirect("/login")
        return
    }
    
    user, err := database.GetUserByID(userID.(int64))
    if err != nil {
        c.Error(err, "get user")
        return
    }
    
    c.User = user
}
```

#### Flamego (Target)

```go
func RequireLogin(c *context.Context, sess session.Session) {
    userID := sess.Get("user_id")
    if userID == nil {
        c.Redirect("/login")
        return
    }
    
    user, err := database.GetUserByID(userID.(int64))
    if err != nil {
        c.Error(err, "get user")
        return
    }
    
    c.User = user
}
```

### JSON API Handler

#### Macaron (Current)

```go
func APIUserInfo(c *context.APIContext) {
    user := c.User
    
    c.JSON(http.StatusOK, &api.User{
        ID:       user.ID,
        Username: user.Name,
        Email:    user.Email,
    })
}
```

#### Flamego (Target)

```go
import "encoding/json"

func APIUserInfo(c *context.APIContext) {
    user := c.User
    
    resp := &api.User{
        ID:       user.ID,
        Username: user.Name,
        Email:    user.Email,
    }
    
    c.ResponseWriter().Header().Set("Content-Type", "application/json")
    c.ResponseWriter().WriteHeader(http.StatusOK)
    json.NewEncoder(c.ResponseWriter()).Encode(resp)
}

// Or create a helper method on context.APIContext
func (c *APIContext) JSON(status int, v any) {
    c.ResponseWriter().Header().Set("Content-Type", "application/json")
    c.ResponseWriter().WriteHeader(status)
    json.NewEncoder(c.ResponseWriter()).Encode(v)
}
```

## Context Usage

### Context Wrapper Update

#### Macaron (Current)

```go
// internal/context/context.go
package context

import (
    "github.com/go-macaron/cache"
    "github.com/go-macaron/csrf"
    "github.com/go-macaron/session"
    "gopkg.in/macaron.v1"
)

type Context struct {
    *macaron.Context
    Cache   cache.Cache
    csrf    csrf.CSRF
    Flash   *session.Flash
    Session session.Store
    
    Link        string
    User        *database.User
    IsLogged    bool
    Repo        *Repository
    Org         *Organization
}

// Contexter middleware
func Contexter(store Store) macaron.Handler {
    return func(
        ctx *macaron.Context,
        l i18n.Locale,
        cache cache.Cache,
        sess session.Store,
        f *session.Flash,
        x csrf.CSRF,
    ) {
        c := &Context{
            Context: ctx,
            Cache:   cache,
            csrf:    x,
            Flash:   f,
            Session: sess,
        }
        
        // Authentication logic...
        c.User, c.IsBasicAuth, c.IsTokenAuth = authenticatedUser(store, c.Context, c.Session)
        
        ctx.Map(c)
    }
}
```

#### Flamego (Target)

```go
// internal/context/context.go
package context

import (
    "github.com/flamego/flamego"
    "github.com/flamego/cache"
    "github.com/flamego/csrf"
    "github.com/flamego/session"
)

type Context struct {
    flamego.Context  // Embedded instead of pointer
    cache   cache.Cache
    csrf    csrf.CSRF
    flash   *session.Flash
    session session.Session
    
    Link        string
    User        *database.User
    IsLogged    bool
    Repo        *Repository
    Org         *Organization
}

// Accessor methods
func (c *Context) Cache() cache.Cache { return c.cache }
func (c *Context) CSRF() csrf.CSRF { return c.csrf }
func (c *Context) Flash() *session.Flash { return c.flash }
func (c *Context) Session() session.Session { return c.session }

// Contexter middleware
func Contexter(store Store) flamego.Handler {
    return func(
        ctx flamego.Context,
        l i18n.Locale,
        cache cache.Cache,
        sess session.Session,
        f *session.Flash,
        x csrf.CSRF,
    ) {
        c := &Context{
            Context: ctx,
            cache:   cache,
            csrf:    x,
            flash:   f,
            session: sess,
        }
        
        // Authentication logic - note Session is now a method
        c.User, c.IsBasicAuth, c.IsTokenAuth = authenticatedUser(store, c, c.session)
        
        ctx.MapTo(c, (*Context)(nil))
    }
}
```

### Response Methods

#### Macaron (Current)

```go
func (c *Context) HTML(status int, name string) {
    log.Trace("Template: %s", name)
    c.Context.HTML(status, name)
}

func (c *Context) JSON(status int, data any) {
    c.Context.JSON(status, data)
}
```

#### Flamego (Target)

```go
// These methods need to be updated to work with injected services

// Option 1: Require template.Template injection
func (c *Context) HTML(status int, name string, t template.Template, data template.Data) {
    log.Trace("Template: %s", name)
    
    // Copy c.Data to template.Data if needed
    for k, v := range c.Data {
        data[k] = v
    }
    
    t.HTML(status, name)
}

// Option 2: Store template reference in context during initialization
func (c *Context) HTML(status int, name string) {
    if c.template == nil {
        panic("template not initialized")
    }
    
    log.Trace("Template: %s", name)
    c.template.HTML(status, name)
}

func (c *Context) JSON(status int, data any) {
    c.ResponseWriter().Header().Set("Content-Type", "application/json")
    c.ResponseWriter().WriteHeader(status)
    json.NewEncoder(c.ResponseWriter()).Encode(data)
}
```

## Form Binding

### Form Struct Tags

#### Macaron (Current)

```go
type CreateRepoForm struct {
    UserID      int64  `form:"user_id" binding:"Required"`
    RepoName    string `form:"repo_name" binding:"Required;AlphaDashDot;MaxSize(100)"`
    Private     bool   `form:"private"`
    Description string `form:"description" binding:"MaxSize(255)"`
    AutoInit    bool   `form:"auto_init"`
    Gitignores  string `form:"gitignores"`
    License     string `form:"license"`
    Readme      string `form:"readme"`
}
```

#### Flamego (Target)

```go
type CreateRepoForm struct {
    UserID      int64  `form:"user_id" validate:"required"`
    RepoName    string `form:"repo_name" validate:"required,alphaDashDot,max=100"`
    Private     bool   `form:"private"`
    Description string `form:"description" validate:"max=255"`
    AutoInit    bool   `form:"auto_init"`
    Gitignores  string `form:"gitignores"`
    License     string `form:"license"`
    Readme      string `form:"readme"`
}

// Note: Custom validators like AlphaDashDot need to be registered with Flamego's validator
```

### Custom Validators

#### Macaron (Current)

```go
import "github.com/go-macaron/binding"

const (
    AlphaDashDotSlash binding.Rule = "AlphaDashDotSlash"
)

func init() {
    binding.SetNameMapper(com.ToSnakeCase)
    binding.AddRule(&binding.Rule{
        IsMatch: func(rule string) bool {
            return rule == "AlphaDashDotSlash"
        },
        IsValid: func(errs binding.Errors, name string, v interface{}) (bool, binding.Errors) {
            str := v.(string)
            if !alphaDashDotSlashPattern.MatchString(str) {
                errs = append(errs, binding.Error{
                    FieldNames: []string{name},
                    Message:    name + " must be valid alpha, dash, dot or slash",
                })
                return false, errs
            }
            return true, errs
        },
    })
}
```

#### Flamego (Target)

```go
import (
    "github.com/flamego/binding"
    "github.com/go-playground/validator/v10"
)

func init() {
    // Register custom validator
    binding.RegisterValidation("alphaDashDotSlash", func(fl validator.FieldLevel) bool {
        str := fl.Field().String()
        return alphaDashDotSlashPattern.MatchString(str)
    })
}

// Usage in struct
type Form struct {
    Path string `form:"path" validate:"required,alphaDashDotSlash"`
}
```

### Multipart Form

#### Macaron (Current)

```go
import "github.com/go-macaron/binding"

type AvatarForm struct {
    Avatar *multipart.FileHeader `form:"avatar"`
}

m.Post("/avatar", binding.MultipartForm(AvatarForm{}), handler)
```

#### Flamego (Target)

```go
import "github.com/flamego/binding"

type AvatarForm struct {
    Avatar *multipart.FileHeader `form:"avatar"`
}

f.Post("/avatar", binding.MultipartForm(AvatarForm{}), handler)
```

## Template Rendering

### Render with Data

#### Macaron (Current)

```go
func ShowRepo(c *context.Context) {
    c.Data["Title"] = c.Repo.Repository.Name
    c.Data["Owner"] = c.Repo.Owner
    c.Data["Repository"] = c.Repo.Repository
    c.Data["IsRepositoryAdmin"] = c.Repo.IsAdmin()
    
    c.HTML(http.StatusOK, "repo/home")
}
```

#### Flamego (Target)

```go
func ShowRepo(c *context.Context, t template.Template, data template.Data) {
    data["Title"] = c.Repo.Repository.Name
    data["Owner"] = c.Repo.Owner
    data["Repository"] = c.Repo.Repository
    data["IsRepositoryAdmin"] = c.Repo.IsAdmin()
    
    t.HTML(http.StatusOK, "repo/home")
}

// Or if context has template reference:
func ShowRepo(c *context.Context) {
    c.Data["Title"] = c.Repo.Repository.Name
    c.Data["Owner"] = c.Repo.Owner
    c.Data["Repository"] = c.Repo.Repository
    c.Data["IsRepositoryAdmin"] = c.Repo.IsAdmin()
    
    c.HTML(http.StatusOK, "repo/home")
}
```

### Render with Error

#### Macaron (Current)

```go
func (c *Context) RenderWithErr(msg, tpl string, f any) {
    if f != nil {
        form.Assign(f, c.Data)
    }
    c.Flash.ErrorMsg = msg
    c.Data["Flash"] = c.Flash
    c.HTML(http.StatusOK, tpl)
}
```

#### Flamego (Target)

```go
func (c *Context) RenderWithErr(msg, tpl string, f any, t template.Template, data template.Data) {
    if f != nil {
        form.Assign(f, c.Data)
        // Also need to assign to data
        for k, v := range c.Data {
            data[k] = v
        }
    }
    c.Flash().ErrorMsg = msg
    data["Flash"] = c.Flash()
    t.HTML(http.StatusOK, tpl)
}
```

## Custom Middleware

### Authentication Middleware

#### Macaron (Current)

```go
func Toggle(options *ToggleOptions) macaron.Handler {
    return func(c *Context) {
        // Check authentication
        if options.SignInRequired {
            if !c.IsLogged {
                c.SetCookie("redirect_to", c.Req.RequestURI, 0, conf.Server.Subpath)
                c.Redirect(conf.Server.Subpath + "/user/login")
                return
            }
        }
        
        // Check admin
        if options.AdminRequired {
            if !c.User.IsAdmin {
                c.Error(nil, http.StatusForbidden)
                return
            }
        }
    }
}
```

#### Flamego (Target)

```go
func Toggle(options *ToggleOptions) flamego.Handler {
    return func(c *Context) {
        // Check authentication
        if options.SignInRequired {
            if !c.IsLogged {
                c.SetCookie(http.Cookie{
                    Name:  "redirect_to",
                    Value: c.Request().RequestURI,
                    Path:  conf.Server.Subpath,
                })
                c.Redirect(conf.Server.Subpath + "/user/login")
                return
            }
        }
        
        // Check admin
        if options.AdminRequired {
            if !c.User.IsAdmin {
                c.Error(nil, http.StatusForbidden)
                return
            }
        }
    }
}
```

### Repository Context Middleware

#### Macaron (Current)

```go
func RepoAssignment() macaron.Handler {
    return func(c *Context) {
        userName := c.Params(":username")
        repoName := c.Params(":reponame")
        
        owner, err := database.GetUserByName(userName)
        if err != nil {
            c.NotFoundOrError(err, "get user")
            return
        }
        c.Repo.Owner = owner
        
        repo, err := database.GetRepositoryByName(owner.ID, repoName)
        if err != nil {
            c.NotFoundOrError(err, "get repository")
            return
        }
        c.Repo.Repository = repo
    }
}
```

#### Flamego (Target)

```go
func RepoAssignment() flamego.Handler {
    return func(c *Context) {
        userName := c.Param("username")  // No colon prefix
        repoName := c.Param("reponame")
        
        owner, err := database.GetUserByName(userName)
        if err != nil {
            c.NotFoundOrError(err, "get user")
            return
        }
        c.Repo.Owner = owner
        
        repo, err := database.GetRepositoryByName(owner.ID, repoName)
        if err != nil {
            c.NotFoundOrError(err, "get repository")
            return
        }
        c.Repo.Repository = repo
    }
}
```

## Complete Example

### Full Route Handler Chain

#### Macaron (Current)

```go
// Setup
m := macaron.New()
m.Use(macaron.Logger())
m.Use(macaron.Recovery())
m.Use(session.Sessioner())
m.Use(csrf.Csrfer())
m.Use(context.Contexter(store))

// Middleware
reqSignIn := context.Toggle(&context.ToggleOptions{SignInRequired: true})

// Routes
m.Group("/:username/:reponame", func() {
    m.Get("/issues", repo.Issues)
    m.Combo("/issues/new").
        Get(repo.NewIssue).
        Post(bindIgnErr(form.NewIssue{}), repo.NewIssuePost)
}, reqSignIn, context.RepoAssignment())

// Handler
func NewIssuePost(c *context.Context, form form.NewIssue) {
    if c.HasError() {
        c.RenderWithErr(c.GetErrMsg(), "repo/issue/new", &form)
        return
    }
    
    issue, err := database.NewIssue(&database.Issue{
        RepoID:  c.Repo.Repository.ID,
        Index:   c.Repo.Repository.NextIssueIndex(),
        Title:   form.Title,
        Content: form.Content,
    })
    if err != nil {
        c.Error(err, "create issue")
        return
    }
    
    c.Redirect(fmt.Sprintf("/%s/%s/issues/%d", 
        c.Repo.Owner.Name, c.Repo.Repository.Name, issue.Index))
}
```

#### Flamego (Target)

```go
// Setup
f := flamego.New()
f.Use(flamego.Logger())
f.Use(flamego.Recovery())
f.Use(session.Sessioner())
f.Use(csrf.Csrfer())
f.Use(template.Templater())
f.Use(context.Contexter(store))

// Middleware
reqSignIn := context.Toggle(&context.ToggleOptions{SignInRequired: true})

// Routes - note parameter syntax change
f.Group("/<username>/<reponame>", func() {
    f.Get("/issues", repo.Issues)
    f.Combo("/issues/new").
        Get(repo.NewIssue).
        Post(binding.Form(form.NewIssue{}), repo.NewIssuePost)
}, reqSignIn, context.RepoAssignment())

// Handler - note template injection
func NewIssuePost(
    c *context.Context, 
    form form.NewIssue,
    t template.Template,
    data template.Data,
) {
    if c.HasError() {
        c.RenderWithErr(c.GetErrMsg(), "repo/issue/new", &form, t, data)
        return
    }
    
    issue, err := database.NewIssue(&database.Issue{
        RepoID:  c.Repo.Repository.ID,
        Index:   c.Repo.Repository.NextIssueIndex(),
        Title:   form.Title,
        Content: form.Content,
    })
    if err != nil {
        c.Error(err, "create issue")
        return
    }
    
    c.Redirect(fmt.Sprintf("/%s/%s/issues/%d", 
        c.Repo.Owner.Name, c.Repo.Repository.Name, issue.Index))
}
```

## Key Takeaways

1. **Parameter Names**: Remove `:` prefix when getting params (`c.Param("name")` vs `c.Params(":name")`)
2. **Route Syntax**: Use `<param>` instead of `:param`
3. **Interface Names**: `session.Store` → `session.Session`
4. **Method Names**: `GetToken()` → `Token()`, `Put()` → `Set()`
5. **Template Injection**: Need to inject `template.Template` and `template.Data` parameters
6. **Response Access**: `c.Resp` → `c.ResponseWriter()`
7. **Request Access**: `c.Req` → `c.Request()`
8. **Context Embedding**: Use `flamego.Context` interface instead of `*macaron.Context` pointer

## Summary

The migration from Macaron to Flamego is largely mechanical with clear patterns:

- Most middleware has direct equivalents
- Handler signatures gain template parameters
- Route parameter syntax changes
- Context access changes from fields to methods
- Overall structure and patterns remain similar

The main work is updating ~150+ files to follow these new patterns consistently.
