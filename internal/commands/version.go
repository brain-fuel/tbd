package commands

import (
	"fmt"

	"goforge.dev/tbd/v2/internal/cli"
)

// Version is the tbd version, overridable at build time via
// -ldflags "-X goforge.dev/tbd/v2/internal/commands.Version=...".
var Version = "1.12.0"

func init() {
	cli.Register(&cli.Command{
		Name:    "version",
		Summary: "Print the tbd version",
		Usage:   "tbd version",
		Run: func(c *cli.Context) error {
			fmt.Fprintf(c.Stdout, "tbd %s\n", Version)
			return nil
		},
	})
}
