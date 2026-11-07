package validation_test

import (
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/go-kanna/internal/packages"
	"github.com/go-kanna/internal/pkgtest"
	"github.com/go-kanna/internal/validation"
)

// plans loads files as one package and plans its targets, as validation.Plans spells
// them.
func plans(t *testing.T, files map[string]string) (map[string][]string, []string) {
	t.Helper()

	pkg := pkgtest.Load(t, files)
	return validation.Plans(pkg.Fset, []*packages.Package{pkg})
}

const planSrc = `package test

import "time"

//kanna:validate
type Request struct {
	Name     string           ` + "`json:\"name\" validate:\"required,min=1,max=50\"`" + `
	Age      int              ` + "`json:\"age\" validate:\"gte=18\"`" + `
	Role     string           ` + "`json:\"role,omitempty\" validate:\"oneof=admin member\"`" + `
	Email    string           ` + "`json:\"email\" validate:\"omitempty,email\"`" + `
	Tags     []string         ` + "`json:\"tags\" validate:\"max=5,dive,max=20\"`" + `
	Nickname *string          ` + "`json:\"nickname\" validate:\"max=20\"`" + `
	Avatar   *string          ` + "`json:\"avatar\" validate:\"required,url\"`" + `
	When     time.Time        ` + "`json:\"when\" validate:\"required\"`" + `
	Point    Point            ` + "`json:\"point\" validate:\"required\"`" + `
	Active   bool             ` + "`json:\"active\" validate:\"required\"`" + `
	Count    int              ` + "`json:\"count\" validate:\"required\"`" + `
	Password string           ` + "`json:\"password\"`" + `
	Confirm  string           ` + "`json:\"confirm\" validate:\"eqfield=Password\"`" + `
	Secret   string           ` + "`json:\"-\" validate:\"required\"`" + `
	Plain    string           ` + "`json:\",omitempty\" validate:\"required\"`" + `
	Address  Address          ` + "`json:\"address\"`" + `
	OptAddr  Address          ` + "`json:\"opt_addr\" validate:\"omitempty\"`" + `
	Billing  *Address         ` + "`json:\"billing\" validate:\"required\"`" + `
	Items    []Item           ` + "`json:\"items\" validate:\"min=1\"`" + `
	Labels   map[string]*Item ` + "`json:\"labels\"`" + `
	Grid     [2]Item          ` + "`json:\"grid\"`" + `
	Matrix   [][]int          ` + "`json:\"matrix\" validate:\"dive,dive,gte=0\"`" + `
	Picks    *[]string        ` + "`json:\"picks\" validate:\"dive,required\"`" + `
	Checked  Checked          ` + "`json:\"checked\"`" + `
	Tree     Tree             ` + "`json:\"tree\"`" + `
	Done     *time.Time       ` + "`json:\"done\" validate:\"rule=Past\"`" + `
	Shifts   []time.Time      ` + "`json:\"shifts\" validate:\"dive,rule=Past\"`" + `
	Ignored  string           ` + "`json:\"ignored\"`" + `
	Audit
	Stamp ` + "`json:\"-\"`" + `
}

type Point struct{ X, Y int }

//kanna:validate
type Address struct {
	Zip string ` + "`json:\"zip\" validate:\"required,pattern=^\\\\d{3}-\\\\d{4}$\"`" + `
}

// Item validates itself by hand.
type Item struct{ SKU string }

func (Item) Validate() error { return nil }

// Checked has a Validate that the generated code cannot call.
type Checked struct{ OK bool }

func (Checked) Validate(strict bool) error { return nil }

// Tree contains itself, and nothing in it validates itself.
type Tree map[string]Tree

// Past is a value type, whose constructor holds a time to its rule.
type Past struct{ t time.Time }

func (p Past) Time() time.Time { return p.t }

func NewPast(t time.Time) (Past, error) { return Past{t: t}, nil }

//kanna:validate
type Audit struct {
	By string ` + "`json:\"by\" validate:\"required\"`" + `
}

// Stamp is embedded under json:"-", which encoding/json leaves out rather
// than flattens.
//
//kanna:validate
type Stamp struct {
	At string ` + "`json:\"at\" validate:\"required\"`" + `
}
`

func TestPlans(t *testing.T) {
	t.Parallel()

	got, diags := plans(t, map[string]string{"test.go": planSrc})
	if len(diags) > 0 {
		t.Fatalf("diagnostics: %q", diags)
	}

	want := map[string][]string{
		"Request": {
			`Name(name): required=="" runes>=1 runes<=50`,
			`Age(age): >=18`,
			`Role(role): oneof("admin","member")`,
			`Email(email): omitempty=="" rule.Email`,
			`Tags(tags): len<=5 []{runes<=20}`,
			`Nickname(nickname): *{runes<=20}`,
			`Avatar(avatar): required==nil *{rule.URL}`,
			`When(when): required==IsZero`,
			`Point(point): required=={}`,
			`Active(active): required==false`,
			`Count(count): required==0`,
			`Confirm(confirm): confirms Password as password`,
			`Secret(Secret): required==""`,
			`Plain(Plain): required==""`,
			`Address(address): nest`,
			`OptAddr(opt_addr): omitempty=={} nest`,
			`Billing(billing): required==nil *{nest}`,
			`Items(items): len>=1 []{nest}`,
			`Labels(labels): []{*{nest}}`,
			`Grid(grid): []{nest}`,
			`Matrix(matrix): []{[]{>=0}}`,
			`Picks(picks): *{[]{required==""}}`,
			`Done(done): *{rule=test.Past via NewPast}`,
			`Shifts(shifts): []{rule=test.Past via NewPast}`,
			`Audit(Audit): nest`,
			`Stamp(Stamp): nest`,
		},
		"Address": {`Zip(zip): required=="" pattern(^\d{3}-\d{4}$)`},
		"Audit":   {`By(by): required==""`},
		"Stamp":   {`At(at): required==""`},
	}
	for _, name := range slices.Sorted(maps.Keys(want)) {
		if !slices.Equal(got[name], want[name]) {
			t.Errorf("plan of %s:\n got  %q\n want %q", name, got[name], want[name])
		}
	}
}

// A blank field cannot be read, so one of a type that validates itself is left
// alone rather than checked through x._.
func TestPlans_BlankField(t *testing.T) {
	t.Parallel()

	got, diags := plans(t, map[string]string{"test.go": `package test

//kanna:validate
type Inner struct {
	Name string ` + "`" + `validate:"required"` + "`" + `
}

//kanna:validate
type Outer struct {
	_     Inner
	Inner Inner
}
`})
	if len(diags) > 0 {
		t.Fatalf("diagnostics: %q", diags)
	}
	if want := []string{"Inner(Inner): nest"}; !slices.Equal(got["Outer"], want) {
		t.Errorf("plan of Outer = %q, want %q", got["Outer"], want)
	}
}

func TestPlans_Errors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		field string
		want  string
	}{
		{
			name:  "rule error names the field",
			field: "Age int `validate:\"emial\"`",
			want:  "test.go:5:2: error: unknown rule \"emial\" (field Age)\n  did you mean \"email\"?",
		},
		{
			name:  "unreadable tag",
			field: "Zip string `validate:\"pattern=^\\d$\"`",
			want:  `the validate tag is not a valid quoted string; write a backslash as \\ (field Zip)`,
		},
		{
			name:  "a rule after pattern",
			field: "Slug string `validate:\"pattern=^[a-z]+$,max=20\"`",
			want: "test.go:5:2: warning: pattern= takes the rest of the tag as its regular expression, " +
				"so \",max=20\" is part of it (field Slug)\n  write pattern= last, after the other rules",
		},
		{
			name:  "a bound given twice",
			field: "Name string `validate:\"max=5,max=10\"`",
			want:  "max is given twice; which of max=5 and max=10 is meant? (field Name)",
		},
		{
			name:  "bounds that no number passes",
			field: "Age int `validate:\"gte=10,lte=5\"`",
			want:  "no value passes gte=10 and lte=5 together (field Age)",
		},
		{
			name:  "an open bound meeting the other",
			field: "Ratio float64 `validate:\"gt=1,lte=1\"`",
			want:  "no value passes gt=1 and lte=1 together (field Ratio)",
		},
		{
			name:  "eq outside the bounds",
			field: "Age int `validate:\"gte=18,eq=7\"`",
			want:  "no value passes gte=18 and eq=7 together (field Age)",
		},
		{
			name:  "lengths that no string passes",
			field: "Name string `validate:\"min=5,max=3\"`",
			want:  "no value passes min=5 and max=3 together (field Name)",
		},
		{
			name:  "a length outside the bounds",
			field: "Tags []string `validate:\"len=2,max=1\"`",
			want:  "no value passes len=2 and max=1 together (field Tags)",
		},
		{
			name:  "a tag on a blank field",
			field: "_ string `validate:\"required\"`",
			want: "test.go:5:2: error: a field named _ cannot be read, so its validate tag would check nothing (field _)\n" +
				"  give the field a name",
		},
		{
			name:  "bad syntax",
			field: "Name string `validate:\"required,\"`",
			want:  "empty rule; check for a stray comma (field Name)",
		},
		{
			name:  "dive on a string",
			field: "Name string `validate:\"dive,required\"`",
			want:  "dive needs a slice, array, or map, not string (field Name)",
		},
		{
			name:  "dive through a pointer to a string",
			field: "Name *string `validate:\"dive,required\"`",
			want:  "dive needs a slice, array, or map, not string (field Name)",
		},
		{
			name:  "required on an incomparable struct",
			field: "Bag struct{ S []string } `validate:\"required\"`",
			want: "required cannot tell whether struct{S []string} is empty (field Bag)\n" +
				"  it is not comparable and has no IsZero() bool method; give it one, or use a pointer",
		},
		{
			name:  "omitempty on an incomparable element",
			field: "Bags []struct{ S []string } `validate:\"dive,omitempty,max=1\"`",
			want:  "omitempty cannot tell whether struct{S []string} is empty (field Bags)",
		},
		{
			// A nil pointer or interface skips every rule but required anyway.
			name:  "omitempty on a pointer",
			field: "Site *string `validate:\"omitempty,max=20\"`",
			want: "omitempty does nothing on *string: a nil pointer skips every rule but required already (field Site)\n" +
				"  remove omitempty",
		},
		{
			name:  "omitempty on an interface",
			field: "Err error `validate:\"omitempty\"`",
			want: "omitempty does nothing on error: a nil interface skips every rule but required already (field Err)\n" +
				"  remove omitempty",
		},
		{
			name:  "eqfield after dive",
			field: "Tags []string `validate:\"dive,eqfield=Other\"`\n\tOther string",
			want:  "eqfield applies to the field itself, not to the elements after dive (field Tags)",
		},
		{
			name:  "rule after dive",
			field: "Tags []string `validate:\"dive,rule=Tag\"`",
			want:  "rule=Tag: no such type (field Tags)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			src := "package test\n\n//kanna:validate\ntype Target struct {\n\t" + tt.field + "\n}\n"
			got, diags := plans(t, map[string]string{"test.go": src})
			if _, ok := got["Target"]; ok {
				t.Errorf("Target was planned despite the error")
			}
			if !slices.ContainsFunc(diags, func(d string) bool { return strings.Contains(d, tt.want) }) {
				t.Errorf("diagnostics = %q, want one containing %q", diags, tt.want)
			}
		})
	}
}

// A Validate promoted through what may be nil is called only once the code has
// tested that, which it cannot do through what another package does not
// export.
func TestPlans_HiddenNil(t *testing.T) {
	t.Parallel()

	set, err := packages.Load([]string{"./testdata/hidden"}, packages.Config{})
	if err != nil || len(set.Diags) > 0 {
		t.Fatalf("load: %v %v", err, set.Diags)
	}
	got, diags := validation.Plans(set.Fset, set.Packages)
	if _, ok := got["Request"]; ok {
		t.Errorf("Request was planned despite the errors")
	}

	const (
		msg  = " has Validate through an embedded field that may be nil, which package hidden cannot test, as "
		hint = " is not exported (field "
	)
	for _, want := range []string{
		"user.ByPointer" + msg + "status" + hint + "Pointer)\n  give user.ByPointer a Validate of its own, or export status",
		"user.ByInterface" + msg + "checker" + hint + "Interface)",
		"user.Below" + msg + "hidden" + hint + "Below)",
		"user.ByPointer" + msg + "status" + hint + "Elements)",
	} {
		if !slices.ContainsFunc(diags, func(d string) bool { return strings.Contains(d, want) }) {
			t.Errorf("diagnostics = %q, want one containing %q", diags, want)
		}
	}
}

func TestWrittenBy(t *testing.T) {
	t.Parallel()

	const body = "\n\npackage test\n"
	tests := []struct {
		name string
		file string
		src  string
		want bool
	}{
		{
			name: "written",
			file: "validate_gen.go",
			src:  "// Code generated by kanna-validate. DO NOT EDIT." + body,
			want: true,
		},
		{
			name: "written by a release",
			file: "validate_gen.go",
			src:  "// Code generated by kanna-validate v0.0.24. DO NOT EDIT." + body,
			want: true,
		},
		{name: "by another tool", file: "validate_gen.go", src: "// Code generated by kanna-value. DO NOT EDIT." + body},
		{name: "by hand", file: "validate_gen.go", src: "package test\n"},
		{name: "another name", file: "rules.go", src: "// Code generated by kanna-validate. DO NOT EDIT." + body},
		{name: "not Go", file: "validate_gen.go", src: "// Code generated by kanna-validate. DO NOT EDIT.\n\nnot go\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			path := filepath.Join(t.TempDir(), tt.file)
			if err := os.WriteFile(path, []byte(tt.src), 0o600); err != nil {
				t.Fatal(err)
			}
			if got := validation.WrittenBy(path, validation.OutputFile, validation.Generator); got != tt.want {
				t.Errorf("WrittenBy(%s) = %v, want %v", tt.file, got, tt.want)
			}
		})
	}
}
