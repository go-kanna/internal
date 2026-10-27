// Package diag reports the problems a generator finds, each tied to the source
// position it is about.
package diag

import (
	"fmt"
	"go/token"
	"io"
	"path/filepath"
	"slices"
	"strings"
)

// Severity tells an error, which stops generation, from a warning, which does
// not.
type Severity int

const (
	Error Severity = iota
	Warning
)

func (s Severity) String() string {
	switch s {
	case Error:
		return "error"
	case Warning:
		return "warning"
	default:
		return fmt.Sprintf("Severity(%d)", int(s))
	}
}

// Diag is one problem, with hint lines that suggest what to do about it. A zero
// Pos means the problem belongs to no particular place in the source.
type Diag struct {
	Severity Severity
	Pos      token.Position
	Message  string
	Hints    []string
}

// Errorf builds an error with a formatted message.
func Errorf(pos token.Position, format string, args ...any) Diag {
	return Diag{Severity: Error, Pos: pos, Message: fmt.Sprintf(format, args...)}
}

// Warningf builds a warning with a formatted message.
func Warningf(pos token.Position, format string, args ...any) Diag {
	return Diag{Severity: Warning, Pos: pos, Message: fmt.Sprintf(format, args...)}
}

// WithHints returns d with hints appended. d itself is left as it was, so one
// Diag can serve as the base of several.
func (d Diag) WithHints(hints ...string) Diag {
	d.Hints = append(slices.Clip(d.Hints), hints...)
	return d
}

// String renders d as "file:line:col: severity: message", followed by one
// indented line per hint. A Diag without a file starts at the severity.
func (d Diag) String() string {
	var b strings.Builder
	if d.Pos.Filename != "" {
		b.WriteString(d.Pos.String())
		b.WriteString(": ")
	}
	fmt.Fprintf(&b, "%s: %s", d.Severity, d.Message)
	for _, h := range d.Hints {
		b.WriteString("\n  ")
		b.WriteString(h)
	}
	return b.String()
}

// HasErrors reports whether ds holds an error.
func HasErrors(ds []Diag) bool {
	return slices.ContainsFunc(ds, func(d Diag) bool { return d.Severity == Error })
}

// Write writes ds to w, one per line, naming files the way the go command does
// for the directory it runs in. A generator run by go generate then reports a
// file as ./user.go rather than by its absolute path. dir is that directory, as
// an absolute path; an empty dir leaves every path as it is.
func Write(w io.Writer, dir string, ds []Diag) {
	for _, d := range ds {
		d.Pos.Filename = ShortPath(dir, d.Pos.Filename)
		fmt.Fprintln(w, d)
	}
}

// ShortPath names path as the go command does when it runs in dir: the path
// relative to dir replaces the absolute one whenever it is shorter, even when it
// climbs out of dir, and a file directly inside dir is written as ./name. An
// empty dir, or a path that is not absolute, leaves path as it is.
func ShortPath(dir, path string) string {
	if dir == "" || !filepath.IsAbs(path) {
		return path
	}
	rel, err := filepath.Rel(dir, path)
	if err != nil || len(rel) >= len(path) {
		return path
	}
	if filepath.Dir(rel) == "." {
		return "." + string(filepath.Separator) + rel
	}
	return rel
}
