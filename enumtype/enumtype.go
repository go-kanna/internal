// Package enumtype knows the enums kanna-value generates: a named string type
// whose values are the constants declared beside it, made from a string with a
// constructor that accepts only those.
//
// kanna-value names the functions it generates for an enum by the rules here,
// and the generators reading its types know one by the same rules, from its
// types alone: a generator that has only the export data of the package
// declaring an enum cannot read the //kanna:enum on its declaration.
package enumtype

import (
	"go/token"
	"go/types"
	"slices"
	"strings"
)

// Type is an enum.
type Type struct {
	Named *types.Named

	// Constants are the constants of the type declared in its package, in the
	// order they are declared, as far as what is loaded of the package tells:
	// the export data of a package seen only through another carries its
	// exported constants, but not necessarily the unexported ones.
	Constants []*types.Const

	// Constructor makes a value of the type from a string, NewX(string) (X,
	// error), or nil when what is loaded of the package declaring the type
	// lacks it.
	Constructor *types.Func
}

// Of returns t as the enum it is, if it is one: a named type, not generic,
// whose underlying type is string and whose package declares constants of it.
func Of(t types.Type) (Type, bool) {
	named, ok := types.Unalias(t).(*types.Named)
	if !ok || named.TypeParams().Len() > 0 || !types.Identical(named.Underlying(), types.Typ[types.String]) {
		return Type{}, false
	}
	consts := Constants(named)
	if len(consts) == 0 {
		return Type{}, false
	}
	return Type{Named: named, Constants: consts, Constructor: constructor(named)}, true
}

// Constants returns the constants of type named declared in its package, in
// the order they are declared.
func Constants(named *types.Named) []*types.Const {
	pkg := named.Obj().Pkg()
	var consts []*types.Const
	for _, name := range pkg.Scope().Names() {
		if c, ok := pkg.Scope().Lookup(name).(*types.Const); ok && types.Identical(c.Type(), named) {
			consts = append(consts, c)
		}
	}
	slices.SortFunc(consts, func(a, b *types.Const) int { return int(a.Pos() - b.Pos()) })
	return consts
}

// Funcs names the functions generated for an enum named name, exported as the
// type is: NewX and XValues, or newX and xValues for an unexported x.
func Funcs(name string) (newFunc, valuesFunc string) {
	if token.IsExported(name) {
		return "New" + name, name + "Values"
	}
	return "new" + strings.ToUpper(name[:1]) + name[1:], name + "Values"
}

// constructor returns the constructor of named from what is loaded of the
// package declaring it, or nil.
func constructor(named *types.Named) *types.Func {
	name, _ := Funcs(named.Obj().Name())
	fn, ok := named.Obj().Pkg().Scope().Lookup(name).(*types.Func)
	if !ok {
		return nil
	}
	sig := fn.Signature()
	if sig.Params().Len() != 1 || sig.Results().Len() != 2 ||
		!types.Identical(sig.Params().At(0).Type(), types.Typ[types.String]) ||
		!types.Identical(sig.Results().At(0).Type(), named) ||
		!types.Identical(sig.Results().At(1).Type(), types.Universe.Lookup("error").Type()) {
		return nil
	}
	return fn
}
