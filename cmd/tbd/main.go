// Command tbd is a trunk-based development wrapper over git's DAG. Its central
// rule: before any mutating operation, the head of the trunk must be an ancestor
// of the ref being operated on or produced.
package main

import (
	"os"

	"goforge.dev/tbd/internal/cli"
	// Registers all subcommands via their init() functions.
	_ "goforge.dev/tbd/internal/commands"
)

func main() {
	os.Exit(cli.Run(os.Args[1:]))
}
