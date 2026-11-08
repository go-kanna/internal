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
// module does not require: a hook takes its *validator.Errors, and a path names
// its Index and Key.
const fakeRuntime = `package validator

const Base = "base"

type FieldError struct {
	Field string
	Err   error
}

type Errors []FieldError

func (e *Errors) Add(field string, err error)  {}
func (e *Errors) Nest(field string, err error) {}
func (e Errors) Err() error                    { return nil }
func (e Errors) Error() string                 { return "" }

func Index(field string, i int) string            { return field }
func Key[K comparable](field string, key K) string { return field }

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
func hooks(t *testing.T, dir string) ([]string, string) {
	t.Helper()

	set, err := packages.Load([]string{"."}, packages.Config{Dir: dir, TypesInfo: true})
	if err != nil || len(set.Diags) > 0 {
		t.Fatalf("load: %v %v", err, set.Diags)
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
		errs.Add("email", validator.Fail("reserved"))
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
		errs.Add("by", validator.Fail("reserved"))
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
	errs.Add("virtual", validator.Blank())
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
	errs.Add(named, nil)
	errs.Add(validator.Base, nil)
	errs.Add("address.zip", nil)
	errs.Add("address.base", nil)
	errs.Add("items[0].sku", nil)
	errs.Add("grid[0][1]", nil)
	errs.Add("labels.a.b.sku", nil)
	errs.Add("a.b", nil)
	errs.Add("Secret", nil)
	errs.Add("note", nil)
	errs.Add("payload.anything", nil)
	errs.Add("checked.anything", nil)
	errs.Add("wrapped.virtual", nil)
	errs.Add(validator.Index("items", 1), nil)
	errs.Add(validator.Index(validator.Index("grid", 0), 1), nil)
	errs.Add(validator.Key("labels", "x"), nil)
	errs.Nest("", nil)
	errs.Nest("address", nil)
	func() {
		errs.Add("address.zip", nil)
		errs := &validator.Errors{}
		errs.Add("nothing", nil)
	}()
	var other validator.Errors
	other.Add("nothing", nil)
	other.Add(validator.Index("nothing", 0), nil)
	_ = o.note
}

// Flat reports, through the Validate of Manual, names no field of it tells.
//
//kanna:validate
type Flat struct {
	Manual
}

func (f Flat) validate(errs *validator.Errors) {
	errs.Add("virtual", nil)
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
	errs.Add("aeg", nil)
	errs.Add(typo, nil)
	errs.Add("zzzzzz", nil)
	errs.Add("address.zpi", nil)
	errs.Add("age.years", nil)
	errs.Add("items.sku", nil)
	errs.Add("fixed.sku", nil)
	errs.Add("items[0].skuu", nil)
	errs.Add("labels[0]", nil)
	errs.Add("address[0]", nil)
	errs.Add("base.zip", nil)
	errs.Add("", nil)
	errs.Add(validator.Index("name", 0), nil)
	errs.Add(validator.Key("items", "x"), nil)
	errs.Nest(validator.Index("", 0), nil)
	errs.Add("virtual", nil)
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
./order.go:24:11: error: "items.sku" is not a path of Order: items is a slice, whose elements go as items[0]
./order.go:25:11: error: "fixed.sku" is not a path of Order: fixed is an array, whose elements go as fixed[0]
./order.go:26:11: error: "items[0].skuu" is not a path of Order: items[0] has no field "skuu"
  did you mean "items[0].sku"?
./order.go:27:11: error: "labels[0]" is not a path of Order: labels is a map, whose keys follow a dot
./order.go:28:11: error: "address[0]" is not a path of Order: address is a struct, whose fields follow a dot
./order.go:29:11: error: "base.zip" is not a path of Order: base is the value as a whole, with nothing under it
./order.go:30:11: error: "" is not a path of Order: Add needs a field
  an error on Order as a whole goes under validator.Base
./order.go:31:11: error: validator.Index("name", 0) is not a path of Order: ` +
		`validator.Index needs a slice or an array, not string
./order.go:32:11: error: validator.Key("items", "x") is not a path of Order: validator.Key needs a map, not []Item
./order.go:33:12: error: validator.Index("", 0) is not a path of Order: ` +
		`validator.Index needs a slice or an array, not Order
./order.go:34:11: error: "virtual" is not a path of Order: Order has no field "virtual"
  an error on Order as a whole goes under validator.Base
`
	if diags != want {
		t.Errorf("diagnostics:\n%s\nwant:\n%s", diags, want)
	}
}
