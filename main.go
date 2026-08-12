package main

import (
	"os"

	"github.com/grievouz/discoctl/cmd"
)

func main() {
	if err := cmd.Run(); err != nil {
		os.Exit(cmd.WriteError(os.Stderr, err))
	}
}
