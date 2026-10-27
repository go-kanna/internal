// Package cli runs a generator as a command: it parses the flags every
// generator shares, reports diagnostics, and checks or applies the changes the
// generator decided on. What is left to each generator is its own flags and
// working out those changes.
package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"

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
}

// Main runs the command on the command line, reporting to stdout and stderr,
// and returns the code to exit with.
func (c Command[C]) Main(args []string) int {
	return c.Exec(Env{Out: os.Stdout, Err: os.Stderr}, args)
}

// Exec runs the command with args, the arguments after its name, in env.
//
// The flags every command has are -check, which compares the files with what
// would be generated instead of writing them; -v, which lists what was written
// or removed; and -version. A run that succeeds says nothing otherwise.
func (c Command[C]) Exec(env Env, args []string) int {
	fs := flag.NewFlagSet(c.Name, flag.ContinueOnError)
	fs.SetOutput(env.Err)
	// Exec prints the usage itself, knowing whether it answers -h, on stdout,
	// or a mistake, on stderr.
	fs.Usage = func() {}

	var (
		cfg                     C
		check, verbose, version bool
	)
	fs.BoolVar(&check, "check", false, "check that the generated files are up to date instead of writing them")
	fs.BoolVar(&verbose, "v", false, "list the files written, removed, or left unchanged")
	fs.BoolVar(&version, "version", false, "print the version and exit")
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

	dir, err := filepath.Abs(env.Dir)
	if err != nil {
		fmt.Fprintf(env.Err, "%s: %v\n", c.Name, err)
		return ExitFailure
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
