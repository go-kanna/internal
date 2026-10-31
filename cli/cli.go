// Package cli runs a generator as a command: it parses the flags every
// generator shares, reports diagnostics, and checks or applies the changes the
// generator decided on. What is left to each generator is its own flags and
// working out those changes.
//
// The environment variable KANNA_CHECK, set to a true value as
// strconv.ParseBool reads it, turns on -check in every generator at once. The
// flags of a //go:generate line are fixed, so KANNA_CHECK=1 go generate ./...
// is how a build finds generated files that have drifted from their generators.
package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
	"strconv"

	"github.com/go-kanna/internal/deps"
	"github.com/go-kanna/internal/diag"
	"github.com/go-kanna/internal/output"
)

// The exit codes of a run.
const (
	ExitOK      = 0
	ExitFailure = 1 // the generator found errors, or could not apply what it generated
	ExitUsage   = 2 // the command line itself is wrong
)

// Env is where a run reads and reports.
type Env struct {
	Out, Err io.Writer

	// Dir is the directory the run works in, which go generate sets to the one
	// holding the directive. Empty means the working directory.
	Dir string

	// Getenv reads the environment. Nil means an empty one.
	Getenv func(key string) string
}

// Command is a generator run from the command line. C holds what its own flags
// set; every run starts from a zero C.
type Command[C any] struct {
	// Name is the command's name, such as "kanna-validate".
	Name string

	// Summary says in one line what the command does.
	Summary string

	// Args shows the arguments the command takes after its flags, such as
	// "[packages]". A command whose Args is empty takes none, and refuses any it
	// is given as a mistake in the command line.
	Args string

	// Help, if set, is printed in the usage below the synopsis.
	Help string

	// Flags registers the command's own flags, each storing into cfg.
	Flags func(fs *flag.FlagSet, cfg *C)

	// Run works out the changes to make, in dir, which is absolute, with the
	// arguments left after the flags. A diagnostic that is an error stops the
	// run before anything is checked or written.
	Run func(dir string, cfg C, args []string) ([]output.Change, []diag.Diag)

	// Deps tells what Run would read and write, generating nothing, for the
	// kanna command to order the run among the others. It is given what Run
	// is given, on a tree where what other runs generate may not be there yet,
	// so only an error it cannot tell anything past is one to return. Nil
	// means the command has no -deps.
	Deps func(dir string, cfg C, args []string) (deps.Report, error)
}

// Main runs the command on the command line, reporting to stdout and stderr,
// and returns the code to exit with.
func (c Command[C]) Main(args []string) int {
	return c.Exec(Env{Out: os.Stdout, Err: os.Stderr, Getenv: os.Getenv}, args)
}

// Exec runs the command with args, the arguments after its name, in env.
//
// The flags every command has are -check, which compares the files with what
// would be generated instead of writing them; -v, which lists what was written,
// removed, or left unchanged; and -version. A run that succeeds says nothing
// otherwise. A command with Deps has -deps too, which prints its report as JSON
// and generates nothing.
func (c Command[C]) Exec(env Env, args []string) int {
	fs := flag.NewFlagSet(c.Name, flag.ContinueOnError)
	fs.SetOutput(env.Err)
	// Exec prints the usage itself, knowing whether it answers -h, on stdout,
	// or a mistake, on stderr.
	fs.Usage = func() {}

	var (
		cfg                           C
		check, verbose, version, plan bool
	)
	fs.BoolVar(&check, "check", false,
		"check that the generated files are up to date instead of writing them; KANNA_CHECK=1 does the same")
	fs.BoolVar(&verbose, "v", false, "list the files written, removed, or left unchanged")
	fs.BoolVar(&version, "version", false, "print the version and exit")
	if c.Deps != nil {
		fs.BoolVar(&plan, "deps", false, "print what a run reads and writes as JSON, for the kanna command, and exit")
	}
	if c.Flags != nil {
		c.Flags(fs, &cfg)
	}

	// Only a leading "help" is the subcommand, so that a package named help
	// cannot turn a run into a no-op.
	help := len(args) > 0 && args[0] == "help"
	err := fs.Parse(args)
	switch {
	case help || errors.Is(err, flag.ErrHelp):
		c.usage(env.Out, fs)
		return ExitOK
	case err != nil:
		c.usage(env.Err, fs)
		return ExitUsage
	case c.Args == "" && fs.NArg() > 0:
		fmt.Fprintf(env.Err, "unexpected argument %q\n", fs.Arg(0))
		c.usage(env.Err, fs)
		return ExitUsage
	}

	if version {
		fmt.Fprintln(env.Out, c.Name, buildVersion())
		return ExitOK
	}

	byEnv, err := checkEnv(env)
	if err != nil {
		fmt.Fprintf(env.Err, "%s: %v\n", c.Name, err)
		return ExitUsage
	}
	check = check || byEnv

	dir, err := filepath.Abs(env.Dir)
	if err != nil {
		fmt.Fprintf(env.Err, "%s: %v\n", c.Name, err)
		return ExitFailure
	}

	if plan {
		return c.answerDeps(env, dir, cfg, fs.Args())
	}

	changes, ds := c.Run(dir, cfg, fs.Args())
	diag.Write(env.Err, dir, ds)
	if diag.HasErrors(ds) {
		return ExitFailure
	}

	if check {
		ds := output.Check(changes)
		diag.Write(env.Err, dir, ds)
		return exitCode(ds)
	}

	applied, ds := output.Apply(changes)
	if verbose {
		for _, a := range applied {
			fmt.Fprintln(env.Out, a.Action, diag.ShortPath(dir, a.Path))
		}
	}
	diag.Write(env.Err, dir, ds)
	return exitCode(ds)
}

// answerDeps prints the report of a run in dir, for -deps.
func (c Command[C]) answerDeps(env Env, dir string, cfg C, args []string) int {
	r, err := c.Deps(dir, cfg, args)
	if err != nil {
		fmt.Fprintf(env.Err, "%s: %v\n", c.Name, err)
		return ExitFailure
	}
	r.Protocol = deps.Protocol
	r.Sort()
	if err := json.NewEncoder(env.Out).Encode(r); err != nil {
		fmt.Fprintf(env.Err, "%s: %v\n", c.Name, err)
		return ExitFailure
	}
	return ExitOK
}

func (c Command[C]) usage(w io.Writer, fs *flag.FlagSet) {
	synopsis := c.Name + " [flags]"
	if c.Args != "" {
		synopsis += " " + c.Args
	}
	fmt.Fprintf(w, "%s: %s\n\nUsage:\n  %s\n", c.Name, c.Summary, synopsis)
	if c.Help != "" {
		fmt.Fprintf(w, "\n%s\n", c.Help)
	}
	fmt.Fprint(w, "\nFlags:\n")
	fs.SetOutput(w)
	fs.PrintDefaults()
}

// checkEnv reports whether KANNA_CHECK turns on -check. A value that is not a
// boolean is an error, rather than a run that writes what it was meant to check.
func checkEnv(env Env) (bool, error) {
	if env.Getenv == nil {
		return false, nil
	}
	v := env.Getenv("KANNA_CHECK")
	if v == "" {
		return false, nil
	}
	check, err := strconv.ParseBool(v)
	if err != nil {
		return false, fmt.Errorf("KANNA_CHECK=%q is not a boolean such as 1 or true", v)
	}
	return check, nil
}

func exitCode(ds []diag.Diag) int {
	if diag.HasErrors(ds) {
		return ExitFailure
	}
	return ExitOK
}

// buildVersion is the version the go command stamped into the binary: the
// module version a go get -tool installed, or a pseudo-version for a build from
// a checkout.
func buildVersion() string {
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
		return info.Main.Version
	}
	return "unknown"
}
