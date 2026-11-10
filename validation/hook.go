package validation

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"path/filepath"
	"reflect"
	"slices"
	"strings"

	"github.com/go-kanna/internal/diag"
	"github.com/go-kanna/internal/packages"
	"github.com/go-kanna/internal/scan"
	"github.com/go-kanna/internal/suggest"
)

// base is the name validator.Base gives the path of an error on a value as a
// whole.
const base = "base"

// Hook returns the validate method s declares for its generated checks to call
// last, or nil, and says why one by that name cannot be called.
//
// Only a method declared on s itself counts. One promoted from an embedded
// struct belongs to that struct's own Validate, which the generated code calls
// already.
func Hook(fset *token.FileSet, s scan.Struct) (*types.Func, []diag.Diag) {
	for m := range s.Named.Methods() {
		if m.Name() != "validate" {
			continue
		}
		sig := m.Signature()
		if sig.Params().Len() == 1 && sig.Results().Len() == 0 && isErrorsPointer(sig.Params().At(0).Type()) {
			return m, nil
		}
		return nil, []diag.Diag{diag.Errorf(fset.Position(m.Pos()),
			"%s.validate must be func(*validator.Errors) for the generated Validate to call it", s.Name).
			WithHints("rename it if it is not meant to be called from Validate")}
	}
	return nil, nil
}

// isErrorsPointer reports whether t is *validator.Errors of the runtime.
func isErrorsPointer(t types.Type) bool {
	ptr, ok := t.(*types.Pointer)
	if !ok {
		return false
	}
	named, ok := types.Unalias(ptr.Elem()).(*types.Named)
	return ok && named.Obj().Name() == "Errors" && named.Obj().Pkg() != nil && named.Obj().Pkg().Path() == runtimePath
}

// HookPaths reports the paths hook, the hook of s that Hook returns, adds or
// nests errors under that no error of s can be under: a field s does not have,
// or more after a value with nothing under it. pkgs are the packages loaded
// with what Config.TypesInfo records, among them that of s.
//
// It looks at the calls of Add and Nest on the Errors the hook is given, whose
// path starts with validator.Field of constants, validator.Base, or
// validator.Path{}, and goes on with the methods Field of constants, Index,
// and Key. Below a value whose Validate
// kanna-validator does not write, nothing is checked, as what it reports is up
// to that Validate.
//
// It also reports a string constant given as the path, which does not compile,
// with the path to write instead, and a key given to Key that is not
// comparable, on which Key panics.
func HookPaths(fset *token.FileSet, pkgs []*packages.Package, s scan.Struct, hook *types.Func) []diag.Diag {
	i := slices.IndexFunc(pkgs, func(p *packages.Package) bool { return p.PkgPath == s.PkgPath })
	if i < 0 || pkgs[i].TypesInfo == nil {
		return nil
	}
	info := pkgs[i].TypesInfo
	decl, errs := hookDecl(pkgs[i].Syntax, info, hook)
	if errs == nil {
		return nil
	}
	c := hookChecker{fset: fset, info: info, target: s, errs: errs, qualify: types.RelativeTo(s.Named.Obj().Pkg())}
	return c.check(decl.Body)
}

// hookDecl returns the declaration of hook among files, and the variable of
// its parameter, or a nil one when it has no body or no name for it.
func hookDecl(files []*ast.File, info *types.Info, hook *types.Func) (*ast.FuncDecl, types.Object) {
	for _, f := range files {
		for _, d := range f.Decls {
			decl, ok := d.(*ast.FuncDecl)
			if !ok || decl.Body == nil || info.Defs[decl.Name] != hook {
				continue
			}
			params := decl.Type.Params.List
			if len(params) != 1 || len(params[0].Names) != 1 {
				return nil, nil
			}
			return decl, info.Defs[params[0].Names[0]]
		}
	}
	return nil, nil
}

type hookChecker struct {
	fset    *token.FileSet
	info    *types.Info
	target  scan.Struct
	errs    types.Object // the hook's parameter
	qualify types.Qualifier
}

func (c hookChecker) check(body *ast.BlockStmt) []diag.Diag {
	var diags []diag.Diag
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || len(call.Args) == 0 {
			return true
		}
		method := c.errsMethod(call)
		if method == "" {
			return true
		}
		arg := call.Args[0]
		if d, ok := c.stringPath(method, arg); ok {
			diags = append(diags, d)
			return true
		}
		if ds := c.uncomparableKeys(arg); len(ds) > 0 {
			diags = append(diags, ds...)
			return true
		}
		segs, ok := c.pathOf(arg)
		if !ok {
			return true
		}
		if len(segs) == 0 {
			if method == "Add" {
				diags = append(diags, diag.Errorf(c.fset.Position(arg.Pos()),
					"%s is not a path of %s: Add needs a field", types.ExprString(arg), c.target.Name).WithHints(c.wholeHint()))
			}
			return true
		}
		w := walk{c: c, segs: segs}
		if !w.value(c.target.Named, 0) {
			d := diag.Errorf(c.fset.Position(arg.Pos()), "%s is not a path of %s: %s",
				c.spell(arg, segs), c.target.Name, w.why)
			if w.hint != "" {
				d = d.WithHints(w.hint)
			}
			diags = append(diags, d)
		}
		return true
	})
	return diags
}

// stringPath reports arg, the path given to method, when it is a string
// constant, which does not compile, with the path to write instead.
func (c hookChecker) stringPath(method string, arg ast.Expr) (diag.Diag, bool) {
	tv := c.info.Types[arg]
	if tv.Value == nil || tv.Value.Kind() != constant.String {
		return diag.Diag{}, false
	}
	name := constant.StringVal(tv.Value)
	var hint string
	switch {
	case name == "" && method == "Add":
		hint = c.wholeHint()
	case name == "":
		hint = "write validator.Path{}"
	case name == base:
		hint = "write validator.Base"
	case strings.ContainsAny(name, "[]"):
		hint = `write it with validator.Field and the methods of Path, as in validator.Field("items").Index(0).Field("sku")`
	default:
		quoted := strings.Split(name, ".")
		for i, q := range quoted {
			quoted[i] = fmt.Sprintf("%q", q)
		}
		hint = "write validator.Field(" + strings.Join(quoted, ", ") + ")"
	}
	return diag.Errorf(c.fset.Position(arg.Pos()), "%s is not a path of %s: %s takes a validator.Path, not a string",
		types.ExprString(arg), c.target.Name, method).WithHints(hint), true
}

// uncomparableKeys reports each key given to Path.Key in e that is not
// comparable, on which Key panics.
func (c hookChecker) uncomparableKeys(e ast.Expr) []diag.Diag {
	var diags []diag.Diag
	ast.Inspect(e, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || len(call.Args) != 1 {
			return true
		}
		sel, ok := ast.Unparen(call.Fun).(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Key" || !c.isPathMethod(sel.Sel) {
			return true
		}
		key := call.Args[0]
		t := c.info.Types[key].Type
		if t == nil || packages.Invalid(t) || types.Comparable(t) {
			return true
		}
		if _, ok := types.Unalias(t).(*types.TypeParam); ok {
			return true
		}
		diags = append(diags, diag.Errorf(c.fset.Position(key.Pos()),
			"Path.Key panics on %s, as %s is not comparable", types.ExprString(key), types.TypeString(t, c.qualify)))
		return true
	})
	return diags
}

// errsMethod returns "Add" or "Nest" for a call of that method on the Errors
// the hook is given, and "" for any other call.
func (c hookChecker) errsMethod(call *ast.CallExpr) string {
	sel, ok := ast.Unparen(call.Fun).(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	recv, ok := ast.Unparen(sel.X).(*ast.Ident)
	if !ok || c.info.Uses[recv] != c.errs {
		return ""
	}
	fn, ok := c.info.Uses[sel.Sel].(*types.Func)
	if !ok {
		return ""
	}
	switch fn.FullName() {
	case "(*" + runtimePath + ".Errors).Add":
		return "Add"
	case "(*" + runtimePath + ".Errors).Nest":
		return "Nest"
	}
	return ""
}

// seg is one step of a path in the source.
type seg struct {
	kind segKind
	name string // of a field
	arg  string // of an index or a key, as the source spells it
	// method tells a field given to the method Field from one given to the
	// function.
	method bool
}

type segKind int

const (
	fieldSeg segKind = iota
	indexSeg
	keySeg
)

// pathOf returns the steps of the path e makes, or false for a path that is
// not made of constants and the functions and methods of the runtime.
func (c hookChecker) pathOf(e ast.Expr) ([]seg, bool) {
	e = ast.Unparen(e)
	if c.isRuntime(e, "Base") {
		return []seg{{kind: fieldSeg, name: base}}, true
	}
	if lit, ok := e.(*ast.CompositeLit); ok && len(lit.Elts) == 0 && c.isRuntimeType(c.info.Types[e].Type, "Path") {
		return nil, true
	}
	call, ok := e.(*ast.CallExpr)
	if !ok || call.Ellipsis.IsValid() {
		return nil, false
	}
	if c.runtimeFunc(call) == "Field" {
		return c.fields(nil, call.Args, false)
	}
	sel, ok := ast.Unparen(call.Fun).(*ast.SelectorExpr)
	if !ok || !c.isPathMethod(sel.Sel) {
		return nil, false
	}
	segs, ok := c.pathOf(sel.X)
	if !ok {
		return nil, false
	}
	switch sel.Sel.Name {
	case "Field":
		return c.fields(segs, call.Args, true)
	case "Index":
		return append(segs, seg{kind: indexSeg, arg: types.ExprString(call.Args[0])}), true
	case "Key":
		return append(segs, seg{kind: keySeg, arg: types.ExprString(call.Args[0])}), true
	}
	return nil, false
}

// fields returns segs and then the fields named by args, or false when one is
// not a constant.
func (c hookChecker) fields(segs []seg, args []ast.Expr, method bool) ([]seg, bool) {
	for _, arg := range args {
		tv, ok := c.info.Types[arg]
		if !ok || tv.Value == nil || tv.Value.Kind() != constant.String {
			return nil, false
		}
		segs = append(segs, seg{kind: fieldSeg, name: constant.StringVal(tv.Value), method: method})
	}
	return segs, true
}

// isPathMethod reports whether id names a method of the Path of the runtime.
func (c hookChecker) isPathMethod(id *ast.Ident) bool {
	fn, ok := c.info.Uses[id].(*types.Func)
	if !ok {
		return false
	}
	recv := fn.Signature().Recv()
	return recv != nil && c.isRuntimeType(recv.Type(), "Path")
}

// isRuntime reports whether e names the object called name of the runtime.
func (c hookChecker) isRuntime(e ast.Expr, name string) bool {
	var id *ast.Ident
	switch e := e.(type) {
	case *ast.SelectorExpr:
		id = e.Sel
	case *ast.Ident:
		id = e
	default:
		return false
	}
	obj := c.info.Uses[id]
	return obj != nil && obj.Name() == name && obj.Pkg() != nil && obj.Pkg().Path() == runtimePath
}

// isRuntimeType reports whether t is the type called name of the runtime.
func (c hookChecker) isRuntimeType(t types.Type, name string) bool {
	named, ok := types.Unalias(t).(*types.Named)
	return ok && named.Obj().Name() == name && named.Obj().Pkg() != nil && named.Obj().Pkg().Path() == runtimePath
}

// runtimeFunc returns the name of the function of the runtime call calls, or
// "".
func (c hookChecker) runtimeFunc(call *ast.CallExpr) string {
	var id *ast.Ident
	switch f := ast.Unparen(call.Fun).(type) {
	case *ast.SelectorExpr:
		id = f.Sel
	case *ast.Ident:
		id = f
	default:
		return ""
	}
	fn, ok := c.info.Uses[id].(*types.Func)
	if !ok || fn.Pkg() == nil || fn.Pkg().Path() != runtimePath || fn.Signature().Recv() != nil {
		return ""
	}
	return fn.Name()
}

// spell names the argument e, a path through segs, in a message: by the path
// it makes when it is made of fields alone, as errors name it.
func (c hookChecker) spell(e ast.Expr, segs []seg) string {
	if !onlyFields(segs) {
		return types.ExprString(e)
	}
	names := make([]string, len(segs))
	for i, s := range segs {
		names[i] = s.name
	}
	return fmt.Sprintf("%q", strings.Join(names, "."))
}

func onlyFields(segs []seg) bool {
	return !slices.ContainsFunc(segs, func(s seg) bool { return s.kind != fieldSeg })
}

func (c hookChecker) wholeHint() string {
	return "an error on " + c.target.Name + " as a whole goes under validator.Base"
}

// kind is what a path can go on with below a value.
type kind int

const (
	leaf   kind = iota // nothing
	opaque             // whatever its own Validate reports
	record             // a field, after a dot
	list               // an element, as [0]
	dict               // a key, after a dot
)

// kindOf returns what a path can go on with below a value of type t, through
// pointers, and the type of its elements for a list or a dict.
func (c hookChecker) kindOf(t types.Type) (kind, types.Type) {
	for {
		p, ok := t.Underlying().(*types.Pointer)
		if !ok {
			break
		}
		t = p.Elem()
	}
	if _, ok := types.Unalias(t).(*types.TypeParam); ok || types.IsInterface(t) || c.validatesItself(t) {
		return opaque, nil
	}
	switch u := t.Underlying().(type) {
	case *types.Struct:
		return record, nil
	case *types.Slice:
		return list, u.Elem()
	case *types.Array:
		return list, u.Elem()
	case *types.Map:
		return dict, u.Elem()
	}
	return leaf, nil
}

// validatesItself reports whether t has a Validate that kanna-validator does
// not write, which reports its errors under paths of its own.
func (c hookChecker) validatesItself(t types.Type) bool {
	obj, _, _ := types.LookupFieldOrMethod(t, true, nil, "Validate")
	fn, ok := obj.(*types.Func)
	return ok && filepath.Base(c.fset.Position(fn.Pos()).Filename) != OutputFile
}

// member is a name the errors of a struct can be under.
type member struct {
	name string
	typ  types.Type
}

// members returns the names the errors of the struct t can be under, as the
// generated Validate names its fields, and whether a field it flattens reports
// under names of its own, which none of them tells.
func (c hookChecker) members(t types.Type, seen []types.Type) ([]member, bool) {
	if slices.ContainsFunc(seen, func(s types.Type) bool { return types.Identical(s, t) }) {
		return nil, false
	}
	seen = append(slices.Clip(seen), t)
	for {
		p, ok := t.Underlying().(*types.Pointer)
		if !ok {
			break
		}
		t = p.Elem()
	}
	st, ok := t.Underlying().(*types.Struct)
	if !ok {
		return nil, false
	}

	var (
		found []member
		open  bool
	)
	for i := range st.NumFields() {
		f := st.Field(i)
		if f.Name() == "_" {
			continue
		}
		tag := reflect.StructTag(st.Tag(i))
		name, named := JSONName(scan.Field{Name: f.Name(), Tag: tag})
		found = append(found, member{name: name, typ: f.Type()})
		if !f.Embedded() || named || tag.Get("json") == "-" {
			continue
		}
		switch k, _ := c.kindOf(f.Type()); k {
		case opaque:
			open = true
		case record:
			flat, o := c.members(f.Type(), seen)
			found = append(found, flat...)
			open = open || o
		case leaf, list, dict:
			// What it reports goes under its own name.
		}
	}
	return found, open
}

// walk tells whether a path can be one the errors of the target are under,
// and if not, why.
type walk struct {
	c    hookChecker
	segs []seg

	why  string // what is wrong
	hint string
}

// fail keeps why the path names nothing, and returns false.
func (w *walk) fail(hint, format string, args ...any) bool {
	w.why, w.hint = fmt.Sprintf(format, args...), hint
	return false
}

// reached spells the first n steps of the path: fields by their names, and an
// index or a key as Go indexes by it, such as items[i].sku.
func (w *walk) reached(n int) string {
	var b strings.Builder
	for i, s := range w.segs[:n] {
		switch {
		case s.kind != fieldSeg:
			b.WriteString("[" + s.arg + "]")
		case i > 0:
			b.WriteString("." + s.name)
		default:
			b.WriteString(s.name)
		}
	}
	return b.String()
}

// value reports whether the steps of the path from at can follow a value of
// type t.
func (w *walk) value(t types.Type, at int) bool {
	if at == len(w.segs) {
		return true
	}
	k, elem := w.c.kindOf(t)
	if k == opaque {
		return true
	}
	switch s := w.segs[at]; {
	case s.kind == fieldSeg && k == record:
		return w.field(t, at)
	case s.kind == indexSeg && k == list, s.kind == keySeg && k == dict:
		return w.value(elem, at+1)
	case s.kind == indexSeg:
		return w.fail("", "Path.Index needs a slice or an array, not %s", types.TypeString(t, w.c.qualify))
	case s.kind == keySeg:
		return w.fail("", "Path.Key needs a map, not %s", types.TypeString(t, w.c.qualify))
	}
	reached := w.reached(at)
	switch k {
	case dict:
		return w.fail("", "%s is a map, whose values go under Path.Key", reached)
	case list:
		return w.fail("", "%s is %s, whose elements go under Path.Index", reached, sliceOrArray(t))
	case leaf, opaque, record:
	}
	return w.fail("", "%s is %s, with nothing under it", reached, types.TypeString(t, w.c.qualify))
}

// field reports whether the field the path names at, and the steps after it,
// can be one of the struct t.
func (w *walk) field(t types.Type, at int) bool {
	name := w.segs[at].name
	members, open := w.c.members(t, nil)
	for _, m := range members {
		if m.name == name {
			return w.value(m.typ, at+1)
		}
	}
	if name == base {
		if at+1 == len(w.segs) {
			return true
		}
		if !open {
			return w.fail("", "%s is the value as a whole, with nothing under it", w.reached(at+1))
		}
	}
	if open {
		return true
	}

	names := make([]string, len(members))
	for i, m := range members {
		names[i] = m.name
	}
	hint := ""
	if s, ok := suggest.Closest(name, names); ok {
		if onlyFields(w.segs[:at]) {
			s = strings.TrimPrefix(w.reached(at)+"."+s, ".")
		}
		hint = fmt.Sprintf("did you mean %q?", s)
	} else if strings.Contains(name, ".") {
		quoted := strings.Split(name, ".")
		for i, q := range quoted {
			quoted[i] = fmt.Sprintf("%q", q)
		}
		call := "validator.Field"
		if w.segs[at].method {
			call = "Path.Field"
		}
		hint = "a name is one field: give each to " + call + ", as in " + call + "(" + strings.Join(quoted, ", ") + ")"
	} else if at == 0 {
		hint = w.c.wholeHint()
	}
	owner := w.c.target.Name
	if at > 0 {
		owner = w.reached(at)
	}
	return w.fail(hint, "%s has no field %q", owner, name)
}

func sliceOrArray(t types.Type) string {
	for {
		p, ok := t.Underlying().(*types.Pointer)
		if !ok {
			break
		}
		t = p.Elem()
	}
	if _, ok := t.Underlying().(*types.Array); ok {
		return "an array"
	}
	return "a slice"
}
