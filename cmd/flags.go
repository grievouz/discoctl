package cmd

import (
	"errors"
	"flag"
	"io"
)

type outputOptions struct {
	pretty bool
}

func addJSONFlag(flags *flag.FlagSet) *outputOptions {
	options := &outputOptions{}
	flags.Bool("json", false, "write structured JSON (currently the default)")
	flags.BoolVar(&options.pretty, "pretty", false, "indent JSON output for human readability")
	return options
}

func parseFlags(flags *flag.FlagSet, args []string) error {
	output := flags.Output()
	flags.SetOutput(io.Discard)
	defer flags.SetOutput(output)
	if err := flags.Parse(args); err != nil {
		return invalidArguments(err)
	}
	return nil
}

func requireNoPositionals(flags *flag.FlagSet) error {
	if flags.NArg() != 0 {
		return invalidArguments(errors.New("unexpected positional arguments"))
	}
	return nil
}
