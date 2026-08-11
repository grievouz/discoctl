package cmd

import (
	"errors"
	"flag"
)

func addJSONFlag(flags *flag.FlagSet) {
	flags.Bool("json", false, "write structured JSON (currently the default)")
}

func requireNoPositionals(flags *flag.FlagSet) error {
	if flags.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	return nil
}
