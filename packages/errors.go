package packages

import (
	"go/token"
	"path/filepath"
	"regexp"
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
func diagnose(roots []*Package, dir string) []diag.Diag {
	var ds []diag.Diag
	packages.Visit(roots, nil, func(p *Package) {
		ds = append(ds, diagnosePackage(p, dir)...)
	})
	return ds
}

// diagnosePackage reports the errors recorded on p.
//
// When the go command compiled p, the compiler's output comes back as a single
// error holding every message at once. p is also parsed and type-checked from
// source, which reports most of those messages a second time. The compiler's
// account is the one kept: it is what go build prints, its syntax errors read
// better than go/parser's cascade, and for a dependency it is the only account
// there is.
func diagnosePackage(p *Package, dir string) []diag.Diag {
	var compiled, listed, source []diag.Diag
	for _, e := range p.Errors {
		switch {
		case e.Kind == packages.ListError && strings.HasPrefix(e.Msg, "# "):
			compiled = append(compiled, compilerDiags(e.Msg, dir)...)
		case e.Kind == packages.ParseError || e.Kind == packages.TypeError:
			source = append(source, diag.Errorf(position(e.Pos), "%s", e.Msg))
		default:
			listed = append(listed, diag.Errorf(position(e.Pos), "%s", e.Msg))
		}
	}

	if len(compiled) > 0 {
		return append(listed, compiled...)
	}
	return append(listed, source...)
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
