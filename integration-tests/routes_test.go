package integrationtests

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/cockroachdb/errors"
)

// route is a route the server registers.
type route struct {
	// Method is "*" for a route that takes any method.
	Method string
	// Pattern is the path as registered, joined with the paths of the groups
	// it is in.
	Pattern string
	// Bridge is true for a route that hands requests to the API v1 handler,
	// which is covered by the API v1 routes behind it.
	Bridge bool
	// Page is true for a route that renders a page of the web app, which the
	// tests leave out. They test what the web app gets from the server.
	Page bool
	// PathRegexp is set for a route whose path is not a constant, which
	// matches the paths of requests to it.
	PathRegexp string
	// Pos is where the route is registered.
	Pos string
}

// dynamicRoutePaths match the paths of the routes registered with a path that
// is not a constant, by the file they are registered in.
var dynamicRoutePaths = map[string]string{
	// One route per language, e.g., "/assets/locale/en-US.0123456789ab.json".
	"webapp_locale.go": `^/assets/locale/[^/]+\.[0-9a-f]{12}\.json$`,
}

// routePackages are the packages, relative to this one, that register routes.
var routePackages = map[string]string{
	"gogs.io/gogs/cmd/gogs/internal/web": "../cmd/gogs/internal/web",
	"gogs.io/gogs/internal/route/api/v1": "../internal/route/api/v1",
	"gogs.io/gogs/internal/route/lfs":    "../internal/route/lfs",
	// Only read for the handlers of pages.
	"gogs.io/gogs/internal/route/repo": "../internal/route/repo",
	"gogs.io/gogs/internal/route/user": "../internal/route/user",
}

// routeRoots are the functions of the web package that set up routers, with
// the name of the router variable in each.
var routeRoots = []struct {
	fn, router string
}{
	{fn: "newRoutingHandler", router: "f"},
	{fn: "newAPIV1Handler", router: "f"},
}

// listRoutes reads the source of the server and returns the routes it
// registers. Routes are found by following calls from routeRoots on the router
// variable, including into functions the router is passed to, and groups whose
// function is a literal or a local variable holding one. A path that is not a
// constant is an error, so that no route is skipped silently.
func listRoutes() ([]route, error) {
	fset := token.NewFileSet()
	l := &routeLister{
		fset:   fset,
		funcs:  make(map[string]map[string]*ast.FuncDecl),
		consts: make(map[string]map[string]string),
	}
	for pkgPath, dir := range routePackages {
		files, err := filepath.Glob(filepath.Join(dir, "*.go"))
		if err != nil {
			return nil, errors.Wrap(err, "glob")
		}
		l.funcs[pkgPath] = make(map[string]*ast.FuncDecl)
		l.consts[pkgPath] = make(map[string]string)
		for _, path := range files {
			if strings.HasSuffix(path, "_test.go") {
				continue
			}
			f, err := parser.ParseFile(fset, path, nil, 0)
			if err != nil {
				return nil, errors.Wrapf(err, "parse %q", path)
			}
			l.collect(pkgPath, f)
		}
	}

	const webPkg = "gogs.io/gogs/cmd/gogs/internal/web"
	for _, root := range routeRoots {
		fn := l.funcs[webPkg][root.fn]
		if fn == nil {
			return nil, errors.Newf("function %q not found", root.fn)
		}
		l.walkFunc(webPkg, fn.Body, root.router, "")
	}
	if len(l.errs) > 0 {
		return nil, errors.Newf("list routes:\n%s", strings.Join(l.errs, "\n"))
	}
	return l.routes, nil
}

type routeLister struct {
	fset *token.FileSet
	// funcs are the functions of each package by name.
	funcs map[string]map[string]*ast.FuncDecl
	// consts are the string constants of each package by name.
	consts map[string]map[string]string
	// imports map the names of imported packages to their paths, by file.
	imports map[*ast.File]map[string]string
	// fileOf is the file of each function.
	fileOf map[*ast.FuncDecl]*ast.File

	routes []route
	errs   []string
}

func (l *routeLister) collect(pkgPath string, f *ast.File) {
	if l.imports == nil {
		l.imports = make(map[*ast.File]map[string]string)
		l.fileOf = make(map[*ast.FuncDecl]*ast.File)
	}
	imports := make(map[string]string)
	for _, imp := range f.Imports {
		path, _ := strconv.Unquote(imp.Path.Value)
		name := filepath.Base(path)
		if imp.Name != nil {
			name = imp.Name.Name
		}
		imports[name] = path
	}
	l.imports[f] = imports

	for _, decl := range f.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if d.Recv == nil {
				l.funcs[pkgPath][d.Name.Name] = d
				l.fileOf[d] = f
			}
		case *ast.GenDecl:
			if d.Tok != token.CONST {
				continue
			}
			for _, spec := range d.Specs {
				vs := spec.(*ast.ValueSpec)
				for i, name := range vs.Names {
					if i < len(vs.Values) {
						if lit, ok := vs.Values[i].(*ast.BasicLit); ok && lit.Kind == token.STRING {
							l.consts[pkgPath][name.Name], _ = strconv.Unquote(lit.Value)
						}
					}
				}
			}
		}
	}
}

// walkFunc records the routes registered on the router in the body.
func (l *routeLister) walkFunc(pkgPath string, body *ast.BlockStmt, router, prefix string) {
	// Groups may take a function from a local variable.
	locals := make(map[string]*ast.FuncLit)
	ast.Inspect(body, func(n ast.Node) bool {
		if assign, ok := n.(*ast.AssignStmt); ok {
			for i, lhs := range assign.Lhs {
				if id, ok := lhs.(*ast.Ident); ok && i < len(assign.Rhs) {
					if lit, ok := assign.Rhs[i].(*ast.FuncLit); ok {
						locals[id.Name] = lit
					}
				}
			}
		}
		return true
	})

	ast.Inspect(body, func(n ast.Node) bool {
		if assign, ok := n.(*ast.AssignStmt); ok {
			// Walked with the prefix of the groups that use them.
			for _, rhs := range assign.Rhs {
				if _, ok := rhs.(*ast.FuncLit); ok {
					return false
				}
			}
		}
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		return l.visitCall(pkgPath, call, router, prefix, locals)
	})
}

// visitCall records the route of the call, if it is one, and returns whether
// to look into its arguments.
func (l *routeLister) visitCall(pkgPath string, call *ast.CallExpr, router, prefix string, locals map[string]*ast.FuncLit) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		// A function of the same package the router is passed to.
		if id, ok := call.Fun.(*ast.Ident); ok {
			l.follow(pkgPath, l.funcs[pkgPath][id.Name], call, router, prefix)
		}
		return true
	}

	// A function of another package the router is passed to.
	if pkg, ok := sel.X.(*ast.Ident); ok && pkg.Name != router {
		for name, path := range l.importsOf(pkgPath, call) {
			if name == pkg.Name && l.funcs[path] != nil {
				l.follow(path, l.funcs[path][sel.Sel.Name], call, router, prefix)
			}
		}
		return true
	}

	// A chain of Combo, e.g., `f.Combo("/x").Get(h).Post(h)`, is visited from
	// its outermost call.
	var methods []string
	for {
		inner, ok := sel.X.(*ast.CallExpr)
		if !ok {
			break
		}
		innerSel, ok := inner.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		methods = append(methods, strings.ToUpper(sel.Sel.Name))
		call, sel = inner, innerSel
	}
	if id, ok := sel.X.(*ast.Ident); !ok || id.Name != router {
		return true
	}

	pos := l.fset.Position(call.Pos()).String()
	dynamicPath := dynamicRoutePaths[filepath.Base(l.fset.Position(call.Pos()).Filename)]
	dynamic := false
	path := func() (string, bool) {
		if len(call.Args) == 0 {
			l.errs = append(l.errs, pos+": missing path")
			return "", false
		}
		p, ok := l.constString(pkgPath, call.Args[0])
		if !ok {
			if dynamicPath == "" {
				l.errs = append(l.errs, pos+": path is not a constant")
				return "", false
			}
			dynamic = true
		}
		return p, true
	}
	add := func(method, p string, handlers []ast.Expr) {
		r := route{
			Method:  method,
			Pattern: prefix + p,
			Bridge:  isBridge(handlers),
			// The web app itself, which serves every page.
			Page: prefix+p == "/{**}",
			Pos:  pos,
		}
		if dynamic {
			r.PathRegexp = dynamicPath
		}
		l.routes = append(l.routes, r)
	}

	switch name := sel.Sel.Name; name {
	case "Group":
		p, ok := path()
		if !ok || len(call.Args) < 2 {
			return false
		}
		var fn *ast.BlockStmt
		switch arg := call.Args[1].(type) {
		case *ast.FuncLit:
			fn = arg.Body
		case *ast.Ident:
			if lit := locals[arg.Name]; lit != nil {
				fn = lit.Body
			}
		}
		if fn == nil {
			l.errs = append(l.errs, pos+": group function not found")
			return false
		}
		l.walkFunc(pkgPath, fn, router, prefix+p)
		return false
	case "Combo":
		p, ok := path()
		if !ok {
			return false
		}
		if len(methods) == 0 {
			l.errs = append(l.errs, pos+": combo without methods")
		}
		for _, m := range methods {
			add(m, p, nil)
		}
		return false
	case "Get", "Post", "Put", "Patch", "Delete", "Head", "Options", "Any":
		p, ok := path()
		if !ok {
			return false
		}
		method := strings.ToUpper(name)
		if name == "Any" {
			method = "*"
		}
		add(method, p, call.Args[1:])
		return false
	case "Route", "Routes":
		p, ok := path()
		if !ok || len(call.Args) < 2 {
			return false
		}
		ms, ok := l.constString(pkgPath, call.Args[1])
		if !ok {
			l.errs = append(l.errs, pos+": methods are not a constant")
			return false
		}
		for _, m := range strings.Split(ms, ",") {
			add(strings.TrimSpace(m), p, call.Args[2:])
		}
		return false
	}
	return true
}

// follow walks the function the router is passed to in the call.
func (l *routeLister) follow(pkgPath string, fn *ast.FuncDecl, call *ast.CallExpr, router, prefix string) {
	if fn == nil {
		return
	}
	for i, arg := range call.Args {
		if id, ok := arg.(*ast.Ident); !ok || id.Name != router {
			continue
		}
		param := paramName(fn, i)
		if param != "" {
			l.walkFunc(pkgPath, fn.Body, param, prefix)
		}
	}
}

func paramName(fn *ast.FuncDecl, i int) string {
	n := 0
	for _, field := range fn.Type.Params.List {
		for _, name := range field.Names {
			if n == i {
				return name.Name
			}
			n++
		}
	}
	return ""
}

// importsOf returns the imports of the file the call is in.
func (l *routeLister) importsOf(pkgPath string, call *ast.CallExpr) map[string]string {
	for fn, f := range l.fileOf {
		if fn.Body != nil && fn.Body.Pos() <= call.Pos() && call.End() <= fn.Body.End() && l.funcs[pkgPath][fn.Name.Name] == fn {
			return l.imports[f]
		}
	}
	return nil
}

// constString evaluates a string constant made of literals, constants of the
// package, the HTTP method constants and concatenations of them.
func (l *routeLister) constString(pkgPath string, e ast.Expr) (string, bool) {
	switch e := e.(type) {
	case *ast.BasicLit:
		if e.Kind != token.STRING {
			return "", false
		}
		s, err := strconv.Unquote(e.Value)
		return s, err == nil
	case *ast.Ident:
		s, ok := l.consts[pkgPath][e.Name]
		return s, ok
	case *ast.SelectorExpr:
		if pkg, ok := e.X.(*ast.Ident); ok && pkg.Name == "http" && strings.HasPrefix(e.Sel.Name, "Method") {
			return strings.ToUpper(strings.TrimPrefix(e.Sel.Name, "Method")), true
		}
	case *ast.BinaryExpr:
		if e.Op != token.ADD {
			return "", false
		}
		x, ok := l.constString(pkgPath, e.X)
		if !ok {
			return "", false
		}
		y, ok := l.constString(pkgPath, e.Y)
		return x + y, ok
	case *ast.ParenExpr:
		return l.constString(pkgPath, e.X)
	}
	return "", false
}

// isBridge returns true when a handler hands the request to the API v1
// handler.
func isBridge(handlers []ast.Expr) bool {
	for _, h := range handlers {
		if call, ok := h.(*ast.CallExpr); ok {
			if id, ok := call.Fun.(*ast.Ident); ok && id.Name == "apiV1Bridger" {
				return true
			}
		}
	}
	return false
}
