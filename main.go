package main

import (
	"fmt"
	"os"

	"github.com/grievouz/discoctl/cmd"
)

func main() {
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "discoctl: %v\n", err)
		os.Exit(1)
	}
}
