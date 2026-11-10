package qualifier_test

import (
	"errors"
	"testing"

	"github.com/go-kanna/internal/pkgtest"
	"github.com/go-kanna/internal/qualifier"
)

func TestScope_Lookup(t *testing.T) {
	t.Parallel()

	pkg := pkgtest.Load(t, map[string]string{
		"a.go": `package test

import (
	_ "embed"
	h "net/http"
	"time"
	_ "math/rand"
	_ "math/rand/v2"
	. "strings"
	_ "net/url"
)

var (
	_ = h.StatusOK
	_ time.Duration
	_ = ToUpper
)
`,
		"b.go": "package test\n\nimport _ \"net/netip\"\n",
	})
	s := qualifier.Of(pkg.Syntax[0], pkg.Types)

	for name, want := range map[string]string{
		"url":   "net/url",  // imported as _, by its own name
		"h":     "net/http", // by the name it is imported under
		"time":  "time",     // imported without a name
		"embed": "embed",
	} {
		got, err := s.Lookup(name)
		if err != nil || got.Path() != want {
			t.Errorf("Lookup(%s) = %v, %v; want %s", name, got, err, want)
		}
	}

	for name, want := range map[string]qualifier.Error{
		// The file calls net/http h, and only b.go imports net/netip.
		"http": {Msg: "the file imports no package named http", Hint: "the file imports net/http, which it calls h; write h"},
		// math/rand/v2 goes by rand, which the file calls math/rand too.
		"v2":      {Msg: "the file imports no package named v2", Hint: "import it, as _ if nothing else uses it"},
		"netip":   {Msg: "the file imports no package named netip", Hint: "import it, as _ if nothing else uses it"},
		"tme":     {Msg: "the file imports no package named tme", Hint: "did you mean time?"},
		"strings": {Msg: "the file imports no package named strings", Hint: "import it, as _ if nothing else uses it"},
		"rand": {
			Msg:  "the file imports more than one package named rand: math/rand, math/rand/v2",
			Hint: "a package imported as _ goes by its own name, which these share",
		},
	} {
		_, err := s.Lookup(name)
		if e, ok := errors.AsType[*qualifier.Error](err); !ok || *e != want {
			t.Errorf("Lookup(%s) error = %#v, want %#v", name, err, want)
		}
	}
}

func TestOf_NoFile(t *testing.T) {
	t.Parallel()

	if _, err := qualifier.Of(nil, nil).Lookup("model"); err == nil {
		t.Error("Lookup in no file found a package")
	}
}
