package packages

import (
	"cmp"
	"go/token"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/go-kanna/internal/diag"
)

// diagnose reports what went wrong loading roots and everything they import,
// dependencies first, as go build orders its output.
//
// A broken dependency has to be looked for: its errors are recorded on the
// dependency alone, and a package importing it comes back with none of its own.
//
// With Config.Tests, a package another root imports comes back twice, as the
// test variant kept among the roots and as the package itself, which share
// their files. An error in one of those is reported once.
func diagnose(roots []*Package, dir string) []diag.Diag {
	type key struct {
		pos token.Position
		msg string
	}
	var (
		ds   []diag.Diag
		seen = make(map[key]bool)
	)
	packages.Visit(roots, nil, func(p *Package) {
		for _, d := range diagnosePackage(p, dir) {
			if k := (key{d.Pos, d.Message}); !seen[k] {
				seen[k] = true
				ds = append(ds, d)
			}
		}
	})
	return ds
}

// diagnosePackage reports the errors recorded on p, sorted by position.
//
// When the go command compiled p, the compiler's output comes back as a single
// error holding every message at once, and p parsed and type-checked from
// source reports the same problems again. The account from source is kept
// whenever there is one, because it names files as they were read: the
// compiler names the temporary copy the go command makes of an overlaid file.
// The compiler's output stands only for a package not read from source, a
// dependency, for which it is the only account there is.
//
// go/packages asks go/parser for every error, so one syntax error comes with
// the cascade that follows it on the same line. Only the first error of a line
// is kept, which is what go/parser does when not asked for all of them.
func diagnosePackage(p *Package, dir string) []diag.Diag {
	var listed, compiled, source []diag.Diag
	parsed := make(map[token.Position]bool)
	for _, e := range p.Errors {
		pos := position(e.Pos)
		switch {
		case e.Kind == packages.ListError && strings.HasPrefix(e.Msg, "# "):
			compiled = append(compiled, compilerDiags(e.Msg, dir)...)
		case e.Kind == packages.ParseError:
			line := token.Position{Filename: pos.Filename, Line: pos.Line}
			if !parsed[line] {
				parsed[line] = true
				source = append(source, diag.Errorf(pos, "%s", e.Msg))
			}
		case e.Kind == packages.TypeError:
			source = append(source, diag.Errorf(pos, "%s", e.Msg))
		default:
			// The go command names a file relative to dir, where it ran, as
			// when a file does not parse as far as its package clause, which
			// go/parser reports again by its absolute path.
			if pos.Filename != "" && !filepath.IsAbs(pos.Filename) {
				pos.Filename = filepath.Join(dir, pos.Filename)
			}
			listed = append(listed, diag.Errorf(pos, "%s", e.Msg))
		}
	}

	if len(source) == 0 {
		source = compiled
	}
	ds := slices.Concat(listed, source)
	slices.SortStableFunc(ds, func(a, b diag.Diag) int {
		return cmp.Or(
			strings.Compare(a.Pos.Filename, b.Pos.Filename),
			cmp.Compare(a.Pos.Line, b.Pos.Line),
			cmp.Compare(a.Pos.Column, b.Pos.Column),
		)
	})
	return ds
}

// compilerLine matches the line a compiler message starts on,
// "file:line:col: message"; some messages have no column.
var compilerLine = regexp.MustCompile(`^(.+?:\d+(?::\d+)?): (.*)$`)

// compilerDiags splits compiler output, a "# <package>" line followed by the
// messages, into one diagnostic per message. A tab-indented line continues the
// message above it, as in the have/want lines of a type mismatch, and stays
// part of it. File names are relative to dir, where the go command ran.
func compilerDiags(out, dir string) []diag.Diag {
	_, body, _ := strings.Cut(out, "\n")

	var ds []diag.Diag
	for line := range strings.SplitSeq(body, "\n") {
		switch m := compilerLine.FindStringSubmatch(line); {
		case strings.HasPrefix(line, "\t") && len(ds) > 0:
			ds[len(ds)-1].Message += "\n" + line
		case m != nil:
			pos := position(m[1])
			if !filepath.IsAbs(pos.Filename) {
				pos.Filename = filepath.Join(dir, pos.Filename)
			}
			ds = append(ds, diag.Errorf(pos, "%s", m[2]))
		case strings.TrimSpace(line) != "":
			ds = append(ds, diag.Errorf(token.Position{}, "%s", line))
		}
	}
	return ds
}

// position parses a position as go/packages records it: "file:line:col",
// "file:line", "file", or "" or "-" for none. Numbers are taken from the
// right, which keeps a Windows drive letter in the file name.
func position(s string) token.Position {
	if s == "" || s == "-" {
		return token.Position{}
	}

	rest, last, ok := cutNumber(s)
	if !ok {
		return token.Position{Filename: s}
	}
	file, line, ok := cutNumber(rest)
	if !ok {
		return token.Position{Filename: rest, Line: last}
	}
	return token.Position{Filename: file, Line: line, Column: last}
}

// cutNumber splits ":<number>" off the end of s.
func cutNumber(s string) (string, int, bool) {
	i := strings.LastIndexByte(s, ':')
	if i < 0 {
		return s, 0, false
	}
	n, err := strconv.Atoi(s[i+1:])
	if err != nil {
		return s, 0, false
	}
	return s[:i], n, true
}
