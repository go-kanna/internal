// Package valuetype knows the value types kanna-value generates: a struct
// holding its value in one unexported field, read with an accessor named after
// the value's type and made with a constructor beside it.
//
// kanna-value names the accessor and the constructors by the rules here, and
// the generators reading its types know one by the same rules, from its types
// alone: a generator that has only the export data of the package declaring a
// value type cannot read the //kanna:value on its declaration.
package valuetype

import (
	"go/token"
	"go/types"
	"unicode"
	"unicode/utf8"
)

// Type is a value type.
type Type struct {
	// Named is the struct, and Held the type of the value its one field holds.
	Named *types.Named
	Held  types.Type

	// Accessor names the method returning the value.
	Accessor string

	// Constructor makes a value of the type, NewX(T) (X, error), or nil when
	// what is loaded of the package declaring the type lacks it: the export
	// data of a package seen only through another carries its types, but not
	// necessarily the functions beside them.
	Constructor *types.Func
}

// Of returns t as the value type it is, if it is one: a struct, not generic,
// with one field, unexported and not embedded, and the accessor of that field's
// value.
func Of(t types.Type) (Type, bool) {
	named, ok := types.Unalias(t).(*types.Named)
	if !ok || named.TypeParams().Len() > 0 {
		return Type{}, false
	}
	st, ok := named.Underlying().(*types.Struct)
	if !ok || st.NumFields() != 1 || st.Field(0).Exported() || st.Field(0).Embedded() {
		return Type{}, false
	}

	held := st.Field(0).Type()
	accessor, ok := Accessor(held)
	if !ok {
		return Type{}, false
	}
	m, _, _ := types.LookupFieldOrMethod(named, false, nil, accessor)
	fn, ok := m.(*types.Func)
	if !ok || fn.Signature().Params().Len() != 0 || fn.Signature().Results().Len() != 1 ||
		!types.Identical(fn.Signature().Results().At(0).Type(), held) {
		return Type{}, false
	}
	return Type{Named: named, Held: held, Accessor: accessor, Constructor: constructor(named, held)}, true
}

// Accessor names the accessor of a value of type held, after the type: String
// for a string, Int64 for an int64, Time for a time.Time. A type without a
// name, such as []string, has none.
func Accessor(held types.Type) (string, bool) {
	switch h := types.Unalias(held).(type) {
	case *types.Basic:
		return upperFirst(h.Name()), true
	case *types.Named:
		return upperFirst(h.Obj().Name()), true
	}
	return "", false
}

// Constructors names the constructors of a value type named name, exported as
// the type is: NewX and MustX, or newX and mustX for an unexported x.
func Constructors(name string) (newFunc, mustFunc string) {
	if token.IsExported(name) {
		return "New" + name, "Must" + name
	}
	return "new" + upperFirst(name), "must" + upperFirst(name)
}

// constructor returns the constructor of named, holding a held, from what is
// loaded of the package declaring it, or nil.
func constructor(named *types.Named, held types.Type) *types.Func {
	name, _ := Constructors(named.Obj().Name())
	fn, ok := named.Obj().Pkg().Scope().Lookup(name).(*types.Func)
	if !ok {
		return nil
	}
	sig := fn.Signature()
	if sig.Params().Len() != 1 || sig.Results().Len() != 2 ||
		!types.Identical(sig.Params().At(0).Type(), held) ||
		!types.Identical(sig.Results().At(0).Type(), named) ||
		!types.Identical(sig.Results().At(1).Type(), types.Universe.Lookup("error").Type()) {
		return nil
	}
	return fn
}

// upperFirst capitalizes the first letter of s, which may take more than one
// byte, as the é of éclair does.
func upperFirst(s string) string {
	r, size := utf8.DecodeRuneInString(s)
	return string(unicode.ToUpper(r)) + s[size:]
}
