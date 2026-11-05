package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/go-kanna/internal/diag"
)

// Vet is the vet subcommand of a generator, such as kanna-orm vet: it checks
// the code that uses what the generator generates, and writes nothing. Its
// flags are -tags and -version, and the packages it checks default to the one
// in the directory it runs in.
type Vet struct {
	// Name is the generator's name, such as "kanna-orm".
	Name string

	// Summary says in one line what the vet finds.
	Summary string

	// Run checks the packages patterns match in dir, which is absolute,
	// loading them with the build tags tags, and returns what it finds.
	Run func(dir, tags string, patterns []string) []diag.Diag
}

// Main runs the vet with args, the arguments after vet, reporting to stdout and
// stderr, and returns the code to exit with.
func (v Vet) Main(args []string) int {
	return v.Exec(Env{Out: os.Stdout, Err: os.Stderr, Getenv: os.Getenv}, args)
}

// Exec runs the vet with args, the arguments after vet, in env. What Run finds
// goes to stderr, and the run fails when any of it is an error.
func (v Vet) Exec(env Env, args []string) int {
	name := v.Name + " vet"
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(env.Err)
	fs.Usage = func() {}
	var (
		tags    string
		version bool
	)
	Tags(fs, &tags)
	fs.BoolVar(&version, "version", false, "print the version and exit")

	help := len(args) > 0 && args[0] == "help"
	err := fs.Parse(args)
	switch {
	case help || errors.Is(err, flag.ErrHelp):
		v.usage(env.Out, fs)
		return ExitOK
	case err != nil:
		v.usage(env.Err, fs)
		return ExitUsage
	case version:
		fmt.Fprintln(env.Out, v.Name, buildVersion())
		return ExitOK
	}

	dir, err := filepath.Abs(env.Dir)
	if err != nil {
		fmt.Fprintf(env.Err, "%s: %v\n", name, err)
		return ExitFailure
	}
	patterns := fs.Args()
	if len(patterns) == 0 {
		patterns = []string{"."}
	}
	ds := v.Run(dir, tags, patterns)
	diag.Write(env.Err, dir, ds)
	return exitCode(ds)
}

func (v Vet) usage(w io.Writer, fs *flag.FlagSet) {
	fmt.Fprintf(w, "%[1]s vet: %[2]s\n\nUsage:\n  %[1]s vet [flags] [packages]\n\n"+
		"The packages default to the one in the current directory. A package passing holds nothing\n"+
		"vet found; it does not prove there is nothing to find.\n\nFlags:\n", v.Name, v.Summary)
	fs.SetOutput(w)
	fs.PrintDefaults()
}
