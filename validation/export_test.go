package validation

import (
	"errors"
	"fmt"
	"go/token"
	"go/types"
	"reflect"
	"strconv"
	"strings"

	"github.com/go-kanna/internal/directive"
	"github.com/go-kanna/internal/gofile"
	"github.com/go-kanna/internal/packages"
	"github.com/go-kanna/internal/qualifier"
	"github.com/go-kanna/internal/scan"
)

// marked returns the structs in pkgs marked //kanna:validate, and a Planner as
// kanna-validator makes for a run that writes Validate for each of them and
// rewrites the output of every package.
func marked(fset *token.FileSet, pkgs []*packages.Package) ([]scan.Struct, Planner) {
	var (
		found  []scan.Struct
		writes []*types.TypeName
	)
	for _, s := range scan.Structs(pkgs) {
		if d, _ := directive.Find(fset, s.Doc, DirectiveKey); d.Found {
			found = append(found, s)
			writes = append(writes, s.Named.Obj())
		}
	}
	rewrites := make(map[string]bool, len(pkgs))
	for _, pkg := range pkgs {
		rewrites[pkg.PkgPath] = true
	}
	return found, NewPlanner(fset, writes, rewrites)
}

// Emit renders the file the structs in pkgs marked //kanna:validate, which make
// one package, are generated into, as kanna-validator writes it: the checks of
// each in a Validate method. declared are the top-level names the package
// declares besides.
func Emit(fset *token.FileSet, pkgs []*packages.Package, declared []string) ([]byte, error) {
	structs, p := marked(fset, pkgs)
	var planned []Planned
	for _, s := range structs {
		fields, diags := p.Struct(s)
		if len(diags) > 0 {
			return nil, fmt.Errorf("%v", diags)
		}
		planned = append(planned, Planned{Name: s.Name, Fields: fields})
	}

	f := gofile.New(Generator, pkgs[0].PkgPath, pkgs[0].Name)
	f.Reserve(declared...)
	w := NewWriter(f, planned, declared)
	for _, s := range planned {
		scope := f.Scope()
		x, errs := scope.Name("x"), scope.Name("errs")
		fmt.Fprintf(f, "// Validate checks %s against the validate tags of %s. It returns the\n", x, s.Name)
		fmt.Fprintf(f, "// %s.Errors of every rule %s breaks, or nil.\n", w.Runtime(), x)
		fmt.Fprintf(f, "func (%s %s) Validate() error {\n", x, s.Name)
		fmt.Fprintf(f, "var %s %s.Errors\n", errs, w.Runtime())

		body := w.Body(scope, x, errs)
		for _, field := range s.Fields {
			// encoding/json flattens an embedded struct without a JSON name.
			path, named := JSONName(field.Field)
			nestPath := strconv.Quote(path)
			if field.Embedded && !named && field.Tag.Get("json") != "-" {
				nestPath = `""`
			}
			fmt.Fprint(f, "\n")
			body.Value(field.Value, x+"."+field.Name, strconv.Quote(path), nestPath)
		}
		fmt.Fprintf(f, "\nreturn %s.Err()\n}\n\n", errs)
	}
	return f.Bytes() //nolint:wrapcheck // the test shows it as it is
}

// Plans plans the structs in pkgs marked //kanna:validate, as kanna-validator
// does in a run that writes Validate for each of them and rewrites the output
// of every package. It spells the plan of each struct, by name, as a line per
// field, such as `Name(name): required=="" runes<=50` with the JSON name in
// parentheses, and each diagnostic as diag.Diag.String does. A struct with a
// diagnostic has no plan, as kanna-validator writes nothing for it.
func Plans(fset *token.FileSet, pkgs []*packages.Package) (map[string][]string, []string) {
	structs, p := marked(fset, pkgs)
	spelled := make(map[string][]string, len(structs))
	var msgs []string
	for _, s := range structs {
		fields, diags := p.Struct(s)
		for _, d := range diags {
			msgs = append(msgs, d.String())
		}
		if len(diags) > 0 {
			continue
		}
		lines := []string{}
		for _, f := range fields {
			path, _ := JSONName(f.Field)
			lines = append(lines, f.Name+"("+path+"): "+describeValue(f.Value))
		}
		spelled[s.Name] = lines
	}
	return spelled, msgs
}

var zeroSpellings = map[zeroTest]string{
	zeroString: `""`, zeroNumber: "0", zeroBool: "false", zeroNil: "nil", zeroMethod: "IsZero", zeroComposite: "{}",
}

// describeValue spells v out on one line, such as `required=="" len>=3 nest`,
// with what a pointer points to in *{...} and each element in []{...}.
func describeValue(v *Value) string {
	var parts []string
	if v.required {
		parts = append(parts, "required=="+zeroSpellings[v.zero])
	}
	if v.omitEmpty {
		parts = append(parts, "omitempty=="+zeroSpellings[v.zero])
	}
	for _, c := range v.checks {
		parts = append(parts, describe(c))
	}
	if v.nest {
		parts = append(parts, "nest")
	}
	if v.pointee != nil {
		parts = append(parts, "*{"+describeValue(v.pointee)+"}")
	}
	if v.elem != nil {
		parts = append(parts, "[]{"+describeValue(v.elem)+"}")
	}
	return strings.Join(parts, " ")
}

// Sibling is another field of the struct, for eqfield to name.
type Sibling = sibling

func NewSibling(name, jsonName string, t types.Type) Sibling {
	return sibling{name: name, jsonName: jsonName, typ: t}
}

// ResolveRule resolves written, one rule as a tag spells it, such as "max=5",
// against t, in a struct that pkg declares with fields besides, and spells out
// the check, as describe does. A qualified rule= may name pkg or what it
// imports.
func ResolveRule(written string, t types.Type, pkg *packages.Package, fields []Sibling) (string, error) {
	name, param, _ := strings.Cut(written, "=")
	at := site{pkg: pkg.Types, imports: qualifier.Of(pkg.Syntax[0], pkg.Types), fields: fields}
	c, err := resolve(rule{name: name, param: param}, t, at)
	if err != nil {
		return "", err
	}
	return describe(c), nil
}

// Hint returns the hint of a rule's error, or "".
func Hint(err error) string {
	if e, ok := errors.AsType[*ruleError](err); ok {
		return e.hint
	}
	return ""
}

// describe spells c out, such as "runes<=50", ">=18", "rule.Email",
// "rule=test.Past", or "rule=test.Deadline via NewDeadline".
func describe(c check) string {
	switch c := c.(type) {
	case lengthCheck:
		if c.runes {
			return "runes" + c.op.String() + strconv.Itoa(c.bound)
		}
		return "len" + c.op.String() + strconv.Itoa(c.bound)
	case compareCheck:
		return c.op.String() + c.bound + durationUnit(c.duration)
	case oneOfCheck:
		return "oneof(" + strings.Join(c.values, ",") + ")" + durationUnit(c.duration)
	case confirmCheck:
		if c.pointer {
			return "confirms *" + c.field + " as " + c.attribute
		}
		return "confirms " + c.field + " as " + c.attribute
	case patternCheck:
		return "pattern(" + c.expr + ")"
	case formatCheck:
		return "rule." + c.rule
	case ruleCheck:
		return "rule=" + types.TypeString(c.typ, (*types.Package).Name) + " via " + c.ctor.Name()
	}
	return fmt.Sprintf("%#v", c)
}

func durationUnit(duration bool) string {
	if duration {
		return "ns"
	}
	return ""
}

// ParseTag is parseTag with each level spelled as a tag of its own, required
// and omitempty first, such as "required,max=50".
func ParseTag(tag reflect.StructTag) ([]string, error) {
	levels, err := parseTag(tag)
	if levels == nil {
		return nil, err
	}

	spelled := make([]string, len(levels))
	for i, l := range levels {
		var parts []string
		if l.required {
			parts = append(parts, "required")
		}
		if l.omitEmpty {
			parts = append(parts, "omitempty")
		}
		for _, r := range l.rules {
			if r.param == "" {
				parts = append(parts, r.name)
			} else {
				parts = append(parts, r.name+"="+r.param)
			}
		}
		spelled[i] = strings.Join(parts, ",")
	}
	return spelled, nil
}

// WrittenBy is writtenBy, which reports whether tool wrote the file at path,
// which it names name.
var WrittenBy = writtenBy
