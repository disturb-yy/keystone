package main

import (
	"fmt"
	"os"
)

func main() {
	command := NewRootCommand(DefaultDependencies())
	if err := command.Execute(); err != nil {
		fmt.Fprintln(command.ErrOrStderr(), err)
		os.Exit(1)
	}
}
