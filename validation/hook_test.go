package validation_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/go-kanna/internal/directive"
	"github.com/go-kanna/internal/packages"
	"github.com/go-kanna/internal/scan"
	"github.com/go-kanna/internal/validation"
)

// fakeRuntime stands in for the runtime of go-kanna/validator, which this
// module does not require: a hook takes its *validator.Errors, and a path goes
// on with its methods.
const fakeRuntime = `package validator

type Path struct{ names []string }

var Base = Field("base")

func Field(name string, names ...string) Path { return Path{} }

func (p Path) Field(name string, names ...string) Path { return p }
func (p Path) Index(i int) Path                        { return p }
func (p Path) Key(key any) Path                        { return p }

type FieldError struct {
	Field string
	Err   error
}

type Errors []FieldError

func (e *Errors) Add(p Path, err error)  {}
func (e *Errors) Nest(p Path, err error) {}
func (e Errors) Err() error              { return nil }
func (e Errors) Error() string           { return "" }

func Blank() error            { return nil }
func Fail(code string) error  { return nil }
`

// writeModule writes files into a fresh module, which resolves
// github.com/go-kanna/validator to fakeRuntime, and returns its directory.
func writeModule(t *testing.T, files map[string]string) string {
	t.Helper()

	dir := t.TempDir()
	files["go.mod"] = "module example.com/app\n\ngo 1.26.0\n\n" +
		"require github.com/go-kanna/validator v0.0.0\n\n" +
		"replace github.com/go-kanna/validator => ./validator\n"
	files["validator/go.mod"] = "module github.com/go-kanna/validator\n\ngo 1.26.0\n"
	files["validator/validator.go"] = fakeRuntime
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// hooks loads the module in dir and finds the hook of each struct marked
// //kanna:validate, as kanna-validator does, and checks its paths. It returns
// the structs by name, with " (hook)" after one that has a hook, and each
// diagnostic as diag.Diag.String spells it, with the files named from dir.
// Like kanna-validator, it goes on past the type errors of the module.
func hooks(t *testing.T, dir string) ([]string, string) {
	t.Helper()

	set, err := packages.Load([]string{"."}, packages.Config{Dir: dir, TypesInfo: true})
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if blocking, _ := set.Split(); len(blocking) > 0 {
		t.Fatalf("load: %v", blocking)
	}

	var (
		names []string
		msgs  strings.Builder
	)
	for _, s := range scan.Structs(set.Packages) {
		if d, _ := directive.Find(set.Fset, s.Doc, validation.DirectiveKey); !d.Found {
			continue
		}
		hook, diags := validation.Hook(set.Fset, s)
		if hook != nil {
			names = append(names, s.Name+" (hook)")
			diags = append(diags, validation.HookPaths(set.Fset, set.Packages, s, hook)...)
		} else {
			names = append(names, s.Name)
		}
		for _, d := range diags {
			msgs.WriteString(strings.ReplaceAll(d.String(), dir+string(filepath.Separator), "./") + "\n")
		}
	}
	return names, msgs.String()
}

func TestHook(t *testing.T) {
	t.Parallel()

	dir := writeModule(t, map[string]string{"hook.go": `package app

import "github.com/go-kanna/validator"

//kanna:validate
type Signup struct {
	Email string ` + "`" + `json:"email" validate:"required"` + "`" + `
}

func (s Signup) validate(errs *validator.Errors) {
	if s.Email == "admin@example.com" {
		errs.Add(validator.Field("email"), validator.Fail("reserved"))
	}
}

// Audit's hook takes a pointer receiver, which the generated method can call
// on its own copy.
//
//kanna:validate
type Audit struct {
	By string ` + "`" + `json:"by" validate:"required"` + "`" + `
}

func (a *Audit) validate(errs *validator.Errors) {
	if a.By == "root" {
		errs.Add(validator.Field("by"), validator.Fail("reserved"))
	}
}

// Order embeds Audit, whose hook its own Validate calls; the Validate of Order
// must not call it again.
//
//kanna:validate
type Order struct {
	Audit
}
`})

	names, diags := hooks(t, dir)
	if want := []string{"Signup (hook)", "Audit (hook)", "Order"}; !slices.Equal(names, want) {
		t.Errorf("structs = %q, want %q", names, want)
	}
	if diags != "" {
		t.Errorf("diagnostics:\n%s", diags)
	}
}

func TestHook_NotCallable(t *testing.T) {
	t.Parallel()

	dir := writeModule(t, map[string]string{"hook.go": `package app

//kanna:validate
type Signup struct {
	Email string
}

func (s Signup) validate() error { return nil }
`})

	names, diags := hooks(t, dir)
	if want := []string{"Signup"}; !slices.Equal(names, want) {
		t.Errorf("structs = %q, want %q", names, want)
	}
	want := "./hook.go:8:17: error: " +
		"Signup.validate must be func(*validator.Errors) for the generated Validate to call it\n" +
		"  rename it if it is not meant to be called from Validate\n"
	if diags != want {
		t.Errorf("diagnostics:\n%s\nwant:\n%s", diags, want)
	}
}

const hookTypes = `package app

import "github.com/go-kanna/validator"

type Address struct {
	Zip string ` + "`json:\"zip\"`" + `
}

type Item struct {
	SKU string ` + "`json:\"sku\"`" + `
}

// Manual validates itself by hand, under names of its own.
type Manual struct{}

func (Manual) Validate() error {
	var errs validator.Errors
	errs.Add(validator.Field("virtual"), validator.Blank())
	return errs.Err()
}

// Wrapper is validated by the Validate of Manual, which it embeds.
type Wrapper struct {
	Manual
}
`

func TestHookPaths_ThatCanBe(t *testing.T) {
	t.Parallel()

	dir := writeModule(t, map[string]string{
		"types.go": hookTypes,
		"order.go": `package app

import "github.com/go-kanna/validator"

//kanna:validate
type Order struct {
	Name    string          ` + "`json:\"name\"`" + `
	Address Address         ` + "`json:\"address\"`" + `
	Items   []Item          ` + "`json:\"items\"`" + `
	Grid    [][]string      ` + "`json:\"grid\"`" + `
	Labels  map[string]Item ` + "`json:\"labels\"`" + `
	Dotted  string          ` + "`json:\"a.b\"`" + `
	Secret  string          ` + "`json:\"-\"`" + `
	Payload any             ` + "`json:\"payload\"`" + `
	Checked Manual          ` + "`json:\"checked\"`" + `
	Wrapped Wrapper         ` + "`json:\"wrapped\"`" + `
	note    string
}

const named = "name"

func (o Order) validate(errs *validator.Errors) {
	errs.Add(validator.Field(named), nil)
	errs.Add(validator.Base, nil)
	errs.Add(validator.Field("address", "zip"), nil)
	errs.Add(validator.Field("address", "base"), nil)
	errs.Add(validator.Field("grid").Index(0), nil)
	errs.Add(validator.Field("a.b"), nil)
	errs.Add(validator.Field("Secret"), nil)
	errs.Add(validator.Field("note"), nil)
	errs.Add(validator.Field("payload", "anything"), nil)
	errs.Add(validator.Field("payload").Index(0), nil)
	errs.Add(validator.Field("checked", "anything"), nil)
	errs.Add(validator.Field("wrapped", "virtual"), nil)
	errs.Add(validator.Field("items").Index(1), nil)
	errs.Add(validator.Field("grid").Index(0).Index(1), nil)
	errs.Add(validator.Field("labels").Key("x"), nil)
	errs.Add((validator.Field("name")), nil)
	errs.Nest(validator.Path{}, nil)
	errs.Nest(validator.Field("address"), nil)
	errs.Nest(validator.Field("items").Index(0), nil)
	errs.Add(validator.Field("items").Index(1).Field("sku"), nil)
	errs.Add(validator.Field("labels").Key("x").Field("sku"), nil)
	errs.Add(validator.Field("items").Index(0).Field("base"), nil)
	errs.Add(validator.Field("address").Field("zip"), nil)
	errs.Add(validator.Path{}.Field("name"), nil)
	errs.Add(validator.Field("payload").Index(0).Field("anything"), nil)
	item := validator.Field("items").Index(0)
	errs.Add(item.Field("nothing"), nil)
	func() {
		errs.Add(validator.Field("address", "zip"), nil)
		errs := &validator.Errors{}
		errs.Add(validator.Field("nothing"), nil)
	}()
	var other validator.Errors
	other.Add(validator.Field("nothing"), nil)
	other.Add(validator.Field("nothing").Index(0), nil)
	names := []string{"nothing"}
	errs.Add(validator.Field(names[0]), nil)
	errs.Add(validator.Field("nothing", names...), nil)
	errs.Add(validator.Field("items").Index(0).Field("nothing", names...), nil)
	_ = o.note
}

// Flat reports, through the Validate of Manual, names no field of it tells.
//
//kanna:validate
type Flat struct {
	Manual
}

func (f Flat) validate(errs *validator.Errors) {
	errs.Add(validator.Field("virtual"), nil)
}
`,
	})

	if _, diags := hooks(t, dir); diags != "" {
		t.Errorf("diagnostics:\n%s", diags)
	}
}

func TestHookPaths_ThatCannotBe(t *testing.T) {
	t.Parallel()

	dir := writeModule(t, map[string]string{
		"types.go": hookTypes,
		"order.go": `package app

import "github.com/go-kanna/validator"

//kanna:validate
type Order struct {
	Name    string          ` + "`json:\"name\"`" + `
	Age     int             ` + "`json:\"age\"`" + `
	Address Address         ` + "`json:\"address\"`" + `
	Items   []Item          ` + "`json:\"items\"`" + `
	Fixed   [2]Item         ` + "`json:\"fixed\"`" + `
	Labels  map[string]Item ` + "`json:\"labels\"`" + `
	Wrapped Wrapper         ` + "`json:\"wrapped\"`" + `
}

const typo = "nme"

func (o Order) validate(errs *validator.Errors) {
	errs.Add(validator.Field("aeg"), nil)
	errs.Add(validator.Field(typo), nil)
	errs.Add(validator.Field("zzzzzz"), nil)
	errs.Add(validator.Field("address", "zpi"), nil)
	errs.Add(validator.Field("age", "years"), nil)
	errs.Add(validator.Field("items", "sku"), nil)
	errs.Add(validator.Field("fixed", "sku"), nil)
	errs.Add(validator.Field("labels", "x"), nil)
	errs.Add(validator.Field("labels").Index(0), nil)
	errs.Add(validator.Field("address").Index(0), nil)
	errs.Add(validator.Field("base", "zip"), nil)
	errs.Add(validator.Path{}, nil)
	errs.Add(validator.Field("name").Index(0), nil)
	errs.Add(validator.Field("items").Key("x"), nil)
	errs.Nest(validator.Path{}.Index(0), nil)
	errs.Add(validator.Field("virtual"), nil)
	errs.Add(validator.Field("address.zip"), nil)
	errs.Add(validator.Field("items").Index(0).Field("skuu"), nil)
	errs.Add(validator.Field("labels").Key("x").Field("a.b"), nil)
	errs.Add(validator.Field("items").Index(0).Field("sku", "x"), nil)
	errs.Add(validator.Field("items").Index(0).Index(1), nil)
	errs.Add(validator.Field("items").Index(0).Field("base", "x"), nil)
	errs.Add(validator.Field("labels").Field("x"), nil)
}
`,
	})

	_, diags := hooks(t, dir)
	want := `./order.go:19:11: error: "aeg" is not a path of Order: Order has no field "aeg"
  did you mean "age"?
./order.go:20:11: error: "nme" is not a path of Order: Order has no field "nme"
  did you mean "name"?
./order.go:21:11: error: "zzzzzz" is not a path of Order: Order has no field "zzzzzz"
  an error on Order as a whole goes under validator.Base
./order.go:22:11: error: "address.zpi" is not a path of Order: address has no field "zpi"
  did you mean "address.zip"?
./order.go:23:11: error: "age.years" is not a path of Order: age is int, with nothing under it
./order.go:24:11: error: "items.sku" is not a path of Order: items is a slice, whose elements go under Path.Index
./order.go:25:11: error: "fixed.sku" is not a path of Order: fixed is an array, whose elements go under Path.Index
./order.go:26:11: error: "labels.x" is not a path of Order: labels is a map, whose values go under Path.Key
./order.go:27:11: error: validator.Field("labels").Index(0) is not a path of Order: ` +
		`Path.Index needs a slice or an array, not map[string]Item
./order.go:28:11: error: validator.Field("address").Index(0) is not a path of Order: ` +
		`Path.Index needs a slice or an array, not Address
./order.go:29:11: error: "base.zip" is not a path of Order: base is the value as a whole, with nothing under it
./order.go:30:11: error: validator.Path{} is not a path of Order: Add needs a field
  an error on Order as a whole goes under validator.Base
./order.go:31:11: error: validator.Field("name").Index(0) is not a path of Order: ` +
		`Path.Index needs a slice or an array, not string
./order.go:32:11: error: validator.Field("items").Key("x") is not a path of Order: ` +
		`Path.Key needs a map, not []Item
./order.go:33:12: error: validator.Path{}.Index(0) is not a path of Order: ` +
		`Path.Index needs a slice or an array, not Order
./order.go:34:11: error: "virtual" is not a path of Order: Order has no field "virtual"
  an error on Order as a whole goes under validator.Base
./order.go:35:11: error: "address.zip" is not a path of Order: Order has no field "address.zip"
  a name is one field: give each to validator.Field, as in validator.Field("address", "zip")
./order.go:36:11: error: validator.Field("items").Index(0).Field("skuu") is not a path of Order: ` +
		`items[0] has no field "skuu"
  did you mean "sku"?
./order.go:37:11: error: validator.Field("labels").Key("x").Field("a.b") is not a path of Order: ` +
		`labels["x"] has no field "a.b"
  a name is one field: give each to Path.Field, as in Path.Field("a", "b")
./order.go:38:11: error: validator.Field("items").Index(0).Field("sku", "x") is not a path of Order: ` +
		`items[0].sku is string, with nothing under it
./order.go:39:11: error: validator.Field("items").Index(0).Index(1) is not a path of Order: ` +
		`Path.Index needs a slice or an array, not Item
./order.go:40:11: error: validator.Field("items").Index(0).Field("base", "x") is not a path of Order: ` +
		`items[0].base is the value as a whole, with nothing under it
./order.go:41:11: error: "labels.x" is not a path of Order: labels is a map, whose values go under Path.Key
`
	if diags != want {
		t.Errorf("diagnostics:\n%s\nwant:\n%s", diags, want)
	}
}

func TestHookPaths_String(t *testing.T) {
	t.Parallel()

	dir := writeModule(t, map[string]string{
		"types.go": hookTypes,
		"order.go": `package app

import "github.com/go-kanna/validator"

//kanna:validate
type Order struct {
	ID      string  ` + "`json:\"id\"`" + `
	Address Address ` + "`json:\"address\"`" + `
	Items   []Item  ` + "`json:\"items\"`" + `
}

const id = "id"

func (o Order) validate(errs *validator.Errors) {
	errs.Add("id", nil)
	errs.Add(id, nil)
	errs.Nest("address.zip", nil)
	errs.Add("base", nil)
	errs.Add("", nil)
	errs.Nest("", nil)
	errs.Add("items[0].sku", nil)
}
`,
	})

	_, diags := hooks(t, dir)
	want := `./order.go:15:11: error: "id" is not a path of Order: Add takes a validator.Path, not a string
  write validator.Field("id")
./order.go:16:11: error: id is not a path of Order: Add takes a validator.Path, not a string
  write validator.Field("id")
./order.go:17:12: error: "address.zip" is not a path of Order: Nest takes a validator.Path, not a string
  write validator.Field("address", "zip")
./order.go:18:11: error: "base" is not a path of Order: Add takes a validator.Path, not a string
  write validator.Base
./order.go:19:11: error: "" is not a path of Order: Add takes a validator.Path, not a string
  an error on Order as a whole goes under validator.Base
./order.go:20:12: error: "" is not a path of Order: Nest takes a validator.Path, not a string
  write validator.Path{}
./order.go:21:11: error: "items[0].sku" is not a path of Order: Add takes a validator.Path, not a string
  write it with validator.Field and the methods of Path, as in validator.Field("items").Index(0).Field("sku")
`
	if diags != want {
		t.Errorf("diagnostics:\n%s\nwant:\n%s", diags, want)
	}
}

func TestHookPaths_Key(t *testing.T) {
	t.Parallel()

	dir := writeModule(t, map[string]string{
		"types.go": hookTypes,
		"order.go": `package app

import "github.com/go-kanna/validator"

type pair struct {
	tags []string
}

type point struct{ x, y int }

//kanna:validate
type Order[K comparable, T any] struct {
	Labels map[string]Item ` + "`json:\"labels\"`" + `
	ByKey  map[K]T         ` + "`json:\"by_key\"`" + `
}

func (o Order[K, T]) validate(errs *validator.Errors) {
	var (
		tags []string
		set  map[string]bool
		fn   func()
		p    pair
		arr  [1][]string
		any_ any
		pt   point
		k    K
		v    T
	)
	errs.Add(validator.Field("labels").Key(tags), nil)
	errs.Add(validator.Field("labels").Key(set).Field("sku"), nil)
	errs.Nest(validator.Field("labels").Key(fn), nil)
	errs.Add(validator.Field("labels").Key(p), nil)
	errs.Add(validator.Field("labels").Key(arr), nil)
	errs.Add(validator.Field("labels").Key(any_), nil)
	errs.Add(validator.Field("labels").Key(pt), nil)
	errs.Add(validator.Field("labels").Key(1), nil)
	errs.Add(validator.Field("by_key").Key(k), nil)
	errs.Add(validator.Field("by_key").Key(v), nil)
}
`,
	})

	_, diags := hooks(t, dir)
	want := `./order.go:29:41: error: Path.Key panics on tags, as []string is not comparable
./order.go:30:41: error: Path.Key panics on set, as map[string]bool is not comparable
./order.go:31:42: error: Path.Key panics on fn, as func() is not comparable
./order.go:32:41: error: Path.Key panics on p, as pair is not comparable
./order.go:33:41: error: Path.Key panics on arr, as [1][]string is not comparable
`
	if diags != want {
		t.Errorf("diagnostics:\n%s\nwant:\n%s", diags, want)
	}
}
