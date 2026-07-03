package app

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newCommitCommand(opts *rootOptions) *cobra.Command {
	var message string
	var noEdit bool
	cmd := &cobra.Command{
		Use:   "commit",
		Short: "stage all work, squash/amend to one commit, fetch trunk, and rebase",
		RunE: func(cmd *cobra.Command, args []string) error {
			e, err := loadEnv(cmd, opts)
			if err != nil {
				return err
			}
			if err := e.EnsureNotProtectedBranch(); err != nil {
				return err
			}
			hr := hookRunner(e)
			if err := hr.Pre("pre-commit"); err != nil {
				return err
			}
			branch, err := e.Repo.CurrentBranch()
			if err != nil {
				return err
			}
			if err := e.Repo.StageAll(); err != nil {
				return err
			}
			seed := message
			if seed == "" {
				seed = currentItemSeed(e, branch)
			}
			if seed == "" {
				seed = "feat: work"
			}
			if err := e.SquashToOne(branch, e.TrunkRef, seed, !noEdit); err != nil {
				return err
			}
			if err := syncRemoteState(e); err != nil {
				return err
			}
			if err := e.RebaseOnto(branch, e.TrunkRef); err != nil {
				return err
			}
			if err := updateCurrentCommit(e, branch); err != nil {
				return err
			}
			hr.Post("post-commit")
			fmt.Fprintf(cmd.OutOrStdout(), "%s is one commit on top of %s\n", branch, e.TrunkRef)
			return nil
		},
	}
	cmd.Flags().StringVarP(&message, "message", "m", "", "seed commit message")
	cmd.Flags().BoolVar(&noEdit, "no-edit", false, "do not open the editor for the commit message")
	return cmd
}

func newSRCommand(opts *rootOptions) *cobra.Command {
	return srCommand(opts, "sr")
}

func srCommand(opts *rootOptions, name string) *cobra.Command {
	var message string
	var edit bool
	cmd := &cobra.Command{
		Use:   name + " [branch]",
		Short: "squash current branch to one commit and rebase",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			e, err := loadEnv(cmd, opts)
			if err != nil {
				return err
			}
			if err := e.EnsureNotProtectedBranch(); err != nil {
				return err
			}
			branch, err := e.Repo.CurrentBranch()
			if err != nil {
				return err
			}
			target := e.TrunkRef
			if len(args) == 1 {
				target = args[0]
			} else if err := syncRemoteState(e); err != nil {
				return err
			}
			if err := e.Repo.StageAll(); err != nil {
				return err
			}
			seed := message
			if seed == "" {
				seed = currentItemSeed(e, branch)
			}
			if seed == "" {
				seed = e.Repo.FullMessage(branch)
			}
			if seed == "" {
				seed = "feat: work"
			}
			if err := e.SquashToOne(branch, target, seed, edit); err != nil {
				return err
			}
			if err := e.RebaseOnto(branch, target); err != nil {
				return err
			}
			if err := updateCurrentCommit(e, branch); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s is one commit on top of %s\n", branch, target)
			return nil
		},
	}
	cmd.Flags().StringVarP(&message, "message", "m", "", "seed squashed commit message")
	cmd.Flags().BoolVar(&edit, "edit", false, "open editor for the squashed commit message")
	return cmd
}
