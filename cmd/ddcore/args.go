package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
)

// flag.FlagSet.Parse stops reading flags at the first positional argument, so
// every documented example that puts options *after* the arguments silently did
// nothing:
//
//	ddcore exec app.services.mod.fn --args '{"a":1}'   → --args stayed in Args()
//	ddcore user add ana@x.com Ana --password s --role R → the name became "Ana --password s ..."
//	ddcore eval 'ddcore.db.count("User")' --commit       → --commit ended up in the code
//
// reorder moves the known flags (and their values) ahead of the positionals so
// a plain Parse sees them, and refuses anything that is not a declared flag
// instead of letting it through as an argument.
//
// It is a pure function over the FlagSet's declarations: no I/O, no globals.
func reorder(fs *flag.FlagSet, args []string) ([]string, error) {
	var flags, pos []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		// "--" ends option parsing; "-" is a positional (stdin)
		if a == "--" {
			pos = append(pos, args[i+1:]...)
			break
		}
		if len(a) < 2 || a[0] != '-' {
			pos = append(pos, a)
			continue
		}
		name := strings.TrimPrefix(strings.TrimPrefix(a, "-"), "-")
		value, inline := "", false
		if j := strings.IndexByte(name, '='); j >= 0 {
			name, value, inline = name[:j], name[j+1:], true
		}
		f := fs.Lookup(name)
		if f == nil && (name == "h" || name == "help") {
			return nil, flag.ErrHelp
		}
		if f == nil {
			return nil, fmt.Errorf("unknown flag: %s (run `ddcore %s -h` to see the options)", a, fs.Name())
		}
		switch {
		case inline:
			flags = append(flags, "-"+name+"="+value)
		case isBoolFlag(f):
			flags = append(flags, "-"+name)
		case i+1 >= len(args):
			return nil, fmt.Errorf("the --%s flag needs a value", name)
		default:
			i++
			flags = append(flags, "-"+name, args[i])
		}
	}
	return append(flags, pos...), nil
}

// isBoolFlag reports whether the flag can appear without a value (--commit).
func isBoolFlag(f *flag.Flag) bool {
	b, ok := f.Value.(interface{ IsBoolFlag() bool })
	return ok && b.IsBoolFlag()
}

// errHelpShown reports that -h or --help printed the command's options: main
// exits 0 without printing an error.
var errHelpShown = errors.New("help shown")

// parseFlags is the reordering replacement for fs.Parse. -h and --help, unless
// the command declares them, print the command's usage and return errHelpShown.
func parseFlags(fs *flag.FlagSet, args []string) error {
	ordered, err := reorder(fs, args)
	if errors.Is(err, flag.ErrHelp) {
		fs.Usage()
		return errHelpShown
	}
	if err != nil {
		return err
	}
	return fs.Parse(ordered)
}

// newFlagSet returns a FlagSet that reports errors instead of exiting, so the
// caller can print them the same way as any other CLI failure. Its usage lists
// the declared flags on stdout; a command with its own usage text replaces it.
func newFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintf(os.Stdout, "usage: ddcore %s [flags]\n\nflags:\n", name)
		fs.SetOutput(os.Stdout)
		fs.PrintDefaults()
		fs.SetOutput(nil)
	}
	return fs
}

// isHelp reports whether a subcommand slot asks for help.
func isHelp(a string) bool { return a == "-h" || a == "--help" || a == "-help" }
