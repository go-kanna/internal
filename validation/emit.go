package validation

import (
	"fmt"
	"go/constant"
	"go/token"
	"go/types"
	"strconv"
	"strings"
	"time"

	"github.com/go-kanna/internal/enumtype"
	"github.com/go-kanna/internal/gofile"
)

// runtimePath is the package generated checks record what breaks with.
const runtimePath = "github.com/go-kanna/validator"

// formatRulesPath is the package holding the rules email, url, and uuid run.
const formatRulesPath = "github.com/go-kanna/value/rule"

// The comparison that breaks each requirement, and the error each reports.
var (
	breaking = map[token.Token]token.Token{
		token.EQL: token.NEQ, token.NEQ: token.EQL,
		token.LSS: token.GEQ, token.LEQ: token.GTR,
		token.GTR: token.LEQ, token.GEQ: token.LSS,
	}
	compareErrors = map[token.Token]string{
		token.EQL: "EqualTo", token.NEQ: "OtherThan",
		token.LSS: "LessThan", token.LEQ: "LessThanOrEqualTo",
		token.GTR: "GreaterThan", token.GEQ: "GreaterThanOrEqualTo",
	}
	lengthErrors = map[token.Token]string{token.GEQ: "TooShort", token.LEQ: "TooLong", token.EQL: "WrongLength"}
)

// Planned is a struct and the plans of its fields with something to check.
type Planned struct {
	Name   string
	Fields []Field
}

// Writer writes the checks of fields into one file.
type Writer struct {
	f *gofile.File

	// rt is the name the file imports the runtime under.
	rt string

	// patterns are the package variables holding each regular expression.
	patterns map[string]string
}

// NewWriter starts the checks of the fields of structs in f. taken are the
// top-level names that the package declares besides and that f declares itself,
// all of which the caller has reserved: gofile wants every top-level name the
// file declares reserved before it hands out any other name. NewWriter imports
// the runtime, and declares a package variable for each regular expression the
// fields match.
func NewWriter(f *gofile.File, structs []Planned, taken []string) *Writer {
	patterns, order := namePatterns(structs, taken)
	for _, expr := range order {
		f.Reserve(patterns[expr])
	}

	w := &Writer{f: f, rt: f.Import(runtimePath, "validator"), patterns: patterns}
	if len(order) > 0 {
		re := f.Import("regexp", "regexp")
		fmt.Fprint(f, "var (\n")
		for _, expr := range order {
			fmt.Fprintf(f, "%s = %s.MustCompile(%s)\n", patterns[expr], re, rawString(expr))
		}
		fmt.Fprint(f, ")\n\n")
	}
	return w
}

// Runtime returns the name the file imports go-kanna/validator under.
func (w *Writer) Runtime() string {
	return w.rt
}

// Body starts the checks of one function or method, whose local variables s
// hands out: x holds the struct the fields are of, and errs is the
// validator.Errors what they break goes into.
func (w *Writer) Body(s gofile.Scope, x, errs string) Body {
	return Body{m: &method{Writer: w, scope: s, x: x, errs: errs, loopVars: make(map[string][]string)}}
}

// Body writes the checks of one function or method.
type Body struct {
	m *method
}

// Value writes the checks of v, the value of the Go expression expr. What it
// breaks goes under path, and what its own Validate reports under nestPath,
// both Go expressions of a path: "" keeps them at the level of the struct, as
// encoding/json flattens an embedded struct without a JSON name.
func (b Body) Value(v *Value, expr, path, nestPath string) {
	b.m.value(v, place{expr: expr, recv: expr, path: path, nestPath: nestPath}, 0)
}

// namePatterns names a package variable for each regular expression the fields
// of structs match, after the first field to match it, such as
// patternRequestZip: one compiled expression serves every field writing the
// same. It returns the names by expression, and the expressions in the order
// they first appear.
func namePatterns(structs []Planned, declared []string) (map[string]string, []string) {
	taken := make(map[string]bool, len(declared))
	for _, name := range declared {
		taken[name] = true
	}

	names := make(map[string]string)
	var order []string
	var walk func(v *Value, base string)
	walk = func(v *Value, base string) {
		if v == nil {
			return
		}
		for _, c := range v.checks {
			if c, ok := c.(patternCheck); ok && names[c.expr] == "" {
				name := base
				for n := 2; taken[name]; n++ {
					name = base + strconv.Itoa(n)
				}
				taken[name] = true
				names[c.expr] = name
				order = append(order, c.expr)
			}
		}
		walk(v.pointee, base)
		walk(v.elem, base)
	}
	for _, s := range structs {
		for _, f := range s.Fields {
			walk(f.Value, "pattern"+s.Name+f.Name)
		}
	}
	return names, order
}

// method writes the checks of one function or method.
type method struct {
	*Writer

	scope gofile.Scope

	// x is the struct, and errs the Errors collected.
	x, errs string

	// loopVars are the loop variables by name and depth, which loops of one
	// depth share.
	loopVars map[string][]string

	// err is the variable errVar names, once a check has asked for it.
	err string
}

// place is where a value is rendered.
type place struct {
	// expr is the Go expression of the value, such as x.Name or (*x.Tags)[i].
	expr string

	// recv is the expression to call the value's methods on. For what a
	// pointer points to, it is the pointer, which Go dereferences itself.
	recv string

	// path is the Go expression of the path errors report the value under.
	path string

	// nestPath is the path the value's own Validate nests its errors under,
	// which is "" for an embedded struct that encoding/json flattens.
	nestPath string
}

// value writes the checks of one value. depth counts the loops around it.
//
// A value that breaks required is reported blank and nothing else: whatever
// else an empty value breaks only says again that it is missing.
func (m *method) value(v *Value, at place, depth int) {
	if v.pointee != nil {
		m.pointer(v, at, depth)
		return
	}

	rest := len(v.checks) > 0 || v.nest || v.elem != nil
	switch {
	case v.required && !rest:
		m.printf("if %s {\n%s\n}\n", m.zero(v, at), m.add(at.path, "Blank()"))
		return
	case v.required:
		m.printf("if %s {\n%s\n} else {\n", m.zero(v, at), m.add(at.path, "Blank()"))
	case v.omitEmpty:
		m.printf("if %s {\n", m.nonZero(v, at))
	}

	// An enum that oneof rejects is not one of its constants either, which
	// its Validate would report again.
	var narrowing *oneOfCheck
	_, enum := enumtype.Of(v.typ)
	for _, c := range v.checks {
		if c, ok := c.(oneOfCheck); ok && enum && v.nest && narrowing == nil {
			narrowing = &c
			continue
		}
		m.check(c, v.typ, at)
	}
	if narrowing != nil {
		m.printf("if %s {\n%s\n} else {\n", m.oneOfCond(*narrowing, at), m.add(at.path, "Inclusion()"))
	}

	if v.nest {
		call := fmt.Sprintf("%s.Nest(%s, %s.Validate())\n", m.errs, at.nestPath, at.recv)
		var tests []string
		if types.IsInterface(v.typ) && !v.required && !v.omitEmpty {
			// Unless required or omitempty has tested it for nil already.
			tests = append(tests, at.expr+" != nil")
		}
		for _, selector := range v.nilable {
			tests = append(tests, at.recv+"."+selector+" != nil")
		}
		if len(tests) > 0 {
			call = fmt.Sprintf("if %s {\n%s}\n", strings.Join(tests, " && "), call)
		}
		m.printf("%s", call)
	}

	if narrowing != nil {
		m.printf("}\n")
	}

	if v.elem != nil {
		m.elem(v, at, depth)
	}

	if v.required || v.omitEmpty {
		m.printf("}\n")
	}
}

// pointer writes the checks of a pointer: required tests it for nil, and the
// rest checks what it points to, when there is something.
func (m *method) pointer(v *Value, at place, depth int) {
	if v.required {
		m.printf("if %s == nil {\n%s\n} else {\n", at.expr, m.add(at.path, "Blank()"))
	} else {
		m.printf("if %s != nil {\n", at.expr)
	}
	pointee := at
	pointee.expr = "*" + paren(at.expr)
	pointee.recv = paren(at.expr)
	m.value(v.pointee, pointee, depth)
	m.printf("}\n")
}

// elem loops over the elements of a slice or array, or the values of a map,
// and writes v.elem for each.
//
// A map is walked in the order of its keys, or of their paths for keys
// without an order, such as structs, so that its errors come out in the same
// order on every run.
func (m *method) elem(v *Value, at place, depth int) {
	elem := at
	if mt, ok := v.typ.Underlying().(*types.Map); ok {
		k, val := m.loopVar("k", depth), m.loopVar("v", depth)
		keys := m.rt + ".SortedKeys(" + at.expr + ")"
		if isOrdered(mt.Key()) {
			keys = m.f.Import("slices", "slices") + ".Sorted(" + m.f.Import("maps", "maps") + ".Keys(" + at.expr + "))"
		}
		m.printf("for _, %s := range %s {\n%s := %s[%s]\n", k, keys, val, paren(at.expr), k)
		elem.expr, elem.recv = val, val
		elem.path = fmt.Sprintf("%s.Key(%s, %s)", m.rt, at.path, k)
	} else {
		i := m.loopVar("i", depth)
		m.printf("for %s := range %s {\n", i, at.expr)
		elem.expr = paren(at.expr) + "[" + i + "]"
		elem.recv = elem.expr
		elem.path = fmt.Sprintf("%s.Index(%s, %s)", m.rt, at.path, i)
	}
	elem.nestPath = elem.path
	m.value(v.elem, elem, depth+1)
	m.printf("}\n")
}

func (m *method) check(c check, t types.Type, at place) {
	switch c := c.(type) {
	case lengthCheck:
		n := "len(" + at.expr + ")"
		if c.runes {
			n = m.f.Import("unicode/utf8", "utf8") + ".RuneCountInString(" + asString(at.expr, t) + ")"
		}
		m.printf("if %s %s %d {\n%s\n}\n", n, breaking[c.op], c.bound,
			m.add(at.path, fmt.Sprintf("%s(%d)", lengthErrors[c.op], c.bound)))
	case compareCheck:
		bound := m.bound(c.bound, c.duration)
		cond := at.expr + " " + breaking[c.op].String() + " " + bound
		switch {
		case isFloat(t) && c.op == token.NEQ:
			// NaN differs from every number, so it meets ne unless tested
			// for.
			cond += " || " + m.f.Import("math", "math") + ".IsNaN(" + asFloat64(at.expr, t) + ")"
		case isFloat(t):
			// NaN compares false with everything, so it meets neither a
			// requirement nor its opposite: the requirement itself is negated.
			cond = "!(" + at.expr + " " + c.op.String() + " " + bound + ")"
		}
		typeArg := ""
		if !c.duration {
			typeArg = m.typeArg(c.bound, t)
		}
		m.printf("if %s {\n%s\n}\n", cond, m.add(at.path, compareErrors[c.op]+typeArg+"("+bound+")"))
	case oneOfCheck:
		m.printf("if %s {\n%s\n}\n", m.oneOfCond(c, at), m.add(at.path, "Inclusion()"))
	case confirmCheck:
		other := m.x + "." + c.field
		test := at.expr + " != " + other
		if c.pointer {
			test = other + " == nil || " + at.expr + " != *" + other
		}
		m.printf("if %s {\n%s\n}\n", test, m.add(at.path, "Confirmation("+strconv.Quote(c.attribute)+")"))
	case patternCheck:
		m.printf("if !%s.MatchString(%s) {\n%s\n}\n", m.patterns[c.expr], asString(at.expr, t), m.add(at.path, "Invalid()"))
	case formatCheck:
		m.printf("%s.Add(%s, %s.%s(%s))\n",
			m.errs, at.path, m.f.Import(formatRulesPath, "rule"), c.rule, asString(at.expr, t))
	case ruleCheck:
		ctor := c.ctor.Name()
		if q := m.f.Qualifier(c.ctor.Pkg()); q != "" {
			ctor = q + "." + ctor
		}
		err := m.errVar()
		m.printf("if _, %s := %s(%s); %s != nil {\n", err, ctor, at.expr, err)
		m.printf("%s.Add(%s, %s)\n", m.errs, at.path, err)
		m.printf("}\n")
	}
}

// oneOfCond is the condition that holds for a value c does not list.
func (m *method) oneOfCond(c oneOfCheck, at place) string {
	conds := make([]string, len(c.values))
	for i, v := range c.values {
		conds[i] = at.expr + " != " + m.bound(v, c.duration)
	}
	return strings.Join(conds, " && ")
}

// add is the statement recording err, a call into the runtime, at path.
func (m *method) add(path, err string) string {
	return fmt.Sprintf("%s.Add(%s, %s.%s)", m.errs, path, m.rt, err)
}

// bound is lit, a Go literal a rule compares with, as the generated code
// writes it: the nanoseconds of a time.Duration in the largest unit they are a
// whole number of, such as 90 * time.Second, which has type time.Duration.
func (m *method) bound(lit string, duration bool) string {
	if !duration {
		return lit
	}
	n, err := strconv.ParseInt(lit, 10, 64)
	if err != nil {
		panic(fmt.Sprintf("emit: duration %q is not in nanoseconds", lit))
	}
	timePkg := m.f.Import("time", "time")
	d := time.Duration(n)
	if d == 0 {
		return timePkg + ".Duration(0)"
	}
	for _, u := range []struct {
		d    time.Duration
		name string
	}{
		{time.Hour, "Hour"}, {time.Minute, "Minute"}, {time.Second, "Second"},
		{time.Millisecond, "Millisecond"}, {time.Microsecond, "Microsecond"}, {time.Nanosecond, "Nanosecond"},
	} {
		if d%u.d != 0 {
			continue
		}
		unit := timePkg + "." + u.name
		if n := d / u.d; n == 1 {
			return unit
		} else if n == -1 {
			return "-" + unit
		}
		return fmt.Sprintf("%d * %s", d/u.d, unit)
	}
	panic("unreachable")
}

// typeArg is the type argument a numeric error needs so that the count it
// reports has t, the type of the value checked: none when the literal bound
// already has that type by default, as 18 has int.
func (m *method) typeArg(bound string, t types.Type) string {
	def := types.Typ[types.Float64]
	if constant.MakeFromLiteral(strings.TrimPrefix(bound, "-"), token.INT, 0).Kind() != constant.Unknown {
		def = types.Typ[types.Int]
	}
	if types.Identical(types.Unalias(t), def) {
		return ""
	}
	return "[" + types.TypeString(t, m.f.Qualifier) + "]"
}

// zero is the condition that holds for the zero value.
func (m *method) zero(v *Value, at place) string {
	switch v.zero {
	case zeroString:
		return at.expr + ` == ""`
	case zeroNumber:
		return at.expr + " == 0"
	case zeroBool:
		return "!" + at.expr
	case zeroNil:
		return at.expr + " == nil"
	case zeroMethod:
		return at.recv + ".IsZero()"
	case zeroComposite:
		return at.expr + " == (" + types.TypeString(v.typ, m.f.Qualifier) + "{})"
	}
	panic(fmt.Sprintf("emit: unknown zero test %d", v.zero))
}

// nonZero is the condition that holds for any value but the zero one.
func (m *method) nonZero(v *Value, at place) string {
	switch v.zero {
	case zeroString:
		return at.expr + ` != ""`
	case zeroNumber:
		return at.expr + " != 0"
	case zeroBool:
		return at.expr
	case zeroNil:
		return at.expr + " != nil"
	case zeroMethod:
		return "!" + at.recv + ".IsZero()"
	case zeroComposite:
		return at.expr + " != (" + types.TypeString(v.typ, m.f.Qualifier) + "{})"
	}
	panic(fmt.Sprintf("emit: unknown zero test %d", v.zero))
}

// errVar names the variable holding what a constructor returned, which every
// check of a method shares, as each declares it in an if of its own.
func (m *method) errVar() string {
	if m.err == "" {
		m.err = m.scope.Name("err")
	}
	return m.err
}

// loopVar names the loop variable base at depth. Every loop of one depth in a
// method shares the name, so that only nested loops number theirs: i, i2, i3.
func (m *method) loopVar(base string, depth int) string {
	names := m.loopVars[base]
	for len(names) <= depth {
		names = append(names, m.scope.Name(base))
	}
	m.loopVars[base] = names
	return names[depth]
}

func (m *method) printf(format string, args ...any) {
	fmt.Fprintf(m.f, format, args...)
}

// asString converts expr, of type t, to a string where a function needs one;
// a value of a type defined on string has to be converted.
func asString(expr string, t types.Type) string {
	if types.Identical(types.Unalias(t), types.Typ[types.String]) {
		return expr
	}
	return "string(" + expr + ")"
}

// asFloat64 converts expr, a floating-point number of type t, to the float64
// math takes.
func asFloat64(expr string, t types.Type) string {
	if types.Identical(types.Unalias(t), types.Typ[types.Float64]) {
		return expr
	}
	return "float64(" + expr + ")"
}

// isOrdered reports whether values of t can be sorted with <.
func isOrdered(t types.Type) bool {
	b, ok := t.Underlying().(*types.Basic)
	return ok && b.Info()&types.IsOrdered != 0
}

// paren wraps a dereference, so that indexing or calling a method applies to
// what it yields.
func paren(expr string) string {
	if strings.HasPrefix(expr, "*") {
		return "(" + expr + ")"
	}
	return expr
}

// rawString quotes s as a raw string where it can, which keeps a regular
// expression as it was written in the tag.
func rawString(s string) string {
	if strconv.CanBackquote(s) {
		return "`" + s + "`"
	}
	return strconv.Quote(s)
}

// isFloat reports whether t is a floating-point number.
func isFloat(t types.Type) bool {
	b, ok := t.Underlying().(*types.Basic)
	return ok && b.Info()&types.IsFloat != 0
}
