package gofile

import (
	"runtime/debug"
	"testing"
)

// SetBuildVersion makes the binary report version as its own until t ends, so t
// cannot run in parallel with anything that calls Bytes.
func SetBuildVersion(t *testing.T, version string) {
	t.Helper()

	saved := readBuildInfo
	readBuildInfo = func() (*debug.BuildInfo, bool) {
		return &debug.BuildInfo{Main: debug.Module{Path: "github.com/go-kanna/test", Version: version}}, true
	}
	t.Cleanup(func() { readBuildInfo = saved })
}
