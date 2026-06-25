package commands

import (
	"fmt"

	"goforge.dev/tbd/internal/argv"
	"goforge.dev/tbd/internal/cli"
)

func init() {
	cli.Register(&cli.Command{
		Name:    "rebase",
		Summary: "Squash the current branch into one commit and rebase it onto trunk",
		Usage: "tbd rebase [message:\"...\"] [:edit] [:abort-on-conflict]\n\n" +
			"Collapses every commit on the current branch into a single commit and\n" +
			"rebases it onto the latest trunk head. Handy for adopting a \"normal\" git\n" +
			"branch into the one-commit-on-trunk shape. Keeps the latest message unless\n" +
			"you pass message:/m: or :edit. Requires a clean working tree.",
		Spec: argv.Spec{
			Named: argv.Opts("message", "m"),
			Flags: argv.Opts("edit", "abort-on-conflict"),
		},
		Run: runRebase,
	})
}

func runRebase(c *cli.Context) error {
	e, err := load(c)
	if err != nil {
		return err
	}
	branch, err := e.repo.CurrentBranch()
	if err != nil {
		return fmt.Errorf("HEAD is detached; check out a branch to rebase")
	}
	if branch == e.trunkLocal || branch == e.trunkRef {
		return fmt.Errorf("refusing to rebase the trunk branch %q onto itself", branch)
	}
	if clean, err := e.repo.IsClean(); err != nil {
		return err
	} else if !clean {
		return fmt.Errorf("working tree has uncommitted changes; commit or stash first")
	}

	msg := c.Args.GetOr("message", c.Args.GetOr("m", ""))
	edit := c.Args.Flag("edit")

	fork, err := e.repo.MergeBase(e.trunkRef, branch)
	if err != nil {
		fork = e.trunkRef
	}
	if existing, _ := e.repo.LogRange(fork + ".." + branch); len(existing) > 1 {
		keep := msg
		if keep == "" {
			keep = e.repo.FullMessage(branch)
		}
		if err := e.repo.ResetSoft(fork); err != nil {
			return err
		}
		if edit {
			if err := e.repo.CommitInteractive(false, keep); err != nil {
				return err
			}
		} else if err := e.repo.Commit(keep); err != nil {
			return err
		}
		fmt.Fprintf(e.out, "%s\n", e.okMark(fmt.Sprintf("squashed %d commits into one", len(existing))))
	}

	return finalizeOnTrunk(e, c, branch)
}
