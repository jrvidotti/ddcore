package main

import (
	"flag"
	"fmt"
	"strings"
)

// flag.FlagSet.Parse stops reading flags at the first positional argument, so
// every documented example that puts options *after* the arguments silently did
// nothing:
//
//	ddcore exec app.services.mod.fn --args '{"a":1}'   → --args ficava em Args()
//	ddcore user add ana@x.com Ana --password s --role R → o nome virava "Ana --password s ..."
//	ddcore eval 'ddcore.db.count("User")' --commit       → --commit entrava no código
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

// parseFlags is the reordering replacement for fs.Parse.
func parseFlags(fs *flag.FlagSet, args []string) error {
	ordered, err := reorder(fs, args)
	if err != nil {
		return err
	}
	return fs.Parse(ordered)
}

// newFlagSet returns a FlagSet that reports errors instead of exiting, so the
// caller can print them the same way as any other CLI failure.
func newFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	return fs
}
