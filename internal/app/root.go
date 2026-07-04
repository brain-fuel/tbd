// Package app is the Cobra-based tbd v2 command surface.
package app

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
)

type rootOptions struct {
	dryRun bool
}

func New() *cobra.Command {
	opts := &rootOptions{}
	cmd := &cobra.Command{
		Use:           "tbd",
		Short:         "workflow-aware trunk and release automation over git",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.PersistentFlags().BoolVar(&opts.dryRun, "dry-run", false, "print planned git/workflow changes without mutating")
	cmd.AddCommand(
		newInitCommand(opts),
		newFeatureCommand(opts),
		newFixCommand(opts),
		newCollabCommand(opts),
		newStackCommand(opts),
		newCommitCommand(opts),
		newSRCommand(opts),
		srCommand(opts, "squash-rebase"),
		newLeaseCommand(opts),
		newStealCommand(opts),
		newRelinquishCommand(opts),
		newReleaseCommand(opts),
		newRevertCommand(opts),
		newTagCommand(opts),
		newLockCommand(opts),
		newSyncCommand(opts),
		newGraphCommand(opts),
		newServeCommand(opts),
		newDemoCommand(opts),
	)
	return cmd
}

func Run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	cmd := New()
	cmd.SetArgs(args)
	cmd.SetIn(stdin)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	if err := cmd.Execute(); err != nil {
		fmt.Fprintf(stderr, "tbd: %v\n", err)
		return 1
	}
	return 0
}

func Main() {
	os.Exit(Run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}
