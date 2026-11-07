package validation_test

import (
	"os"
	"testing"
)

// TestMain loads every module the tests write outside any workspace: a GOWORK
// that the person running them has set would leave such a module out.
func TestMain(m *testing.M) {
	// Before any test runs: t.Setenv would rule out t.Parallel.
	if err := os.Setenv("GOWORK", "off"); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}
