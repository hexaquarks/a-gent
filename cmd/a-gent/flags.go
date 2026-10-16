package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
)

// Release builds replace version with the Git tag through the Go linker.
var version = "dev"

func showCommandInfo(arguments []string, output io.Writer) (bool, error) {
	flags := flag.NewFlagSet("a-gent", flag.ContinueOnError)
	flags.SetOutput(output)
	printVersion := flags.Bool("version", false, "Print version and exit")
	if err := flags.Parse(arguments); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return true, nil
		}
		return true, err
	}
	if flags.NArg() != 0 {
		return true, fmt.Errorf("unexpected argument: %s", flags.Arg(0))
	}

	if *printVersion {
		_, err := fmt.Fprintf(output, "a-gent %s\n", version)
		return true, err
	}
	return false, nil
}
