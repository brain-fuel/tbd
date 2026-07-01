package commands

import (
	"fmt"

	"goforge.dev/tbd/internal/argv"
	"goforge.dev/tbd/internal/cli"
)

func init() {
	cli.Register(&cli.Command{
		Name:    "sqr",
		Summary: "Squash the current branch into one commit and rebase it onto trunk (or onto:BRANCH)",
		Usage: "tbd sqr [onto:BRANCH] [message:\"...\"] [:edit] [:abort-on-conflict]\n\n" +
			"Collapses every commit on the current branch into a single commit and\n" +
			"rebases it onto the latest trunk head. Pass onto:BRANCH to rebase onto\n" +
			"another branch instead of trunk. Handy for adopting a \"normal\" git branch\n" +
			"into the one-commit shape. Keeps the latest message unless you pass\n" +
			"message:/m: or :edit. Requires a clean working tree.",
		Spec: argv.Spec{
			Named: argv.Opts("message", "m", "onto"),
			Flags: argv.Opts("edit", "abort-on-conflict"),
		},
		Run: runSqr,
	})
}

func runSqr(c *cli.Context) error {
	e, err := load(c)
	if err != nil {
		return err
	}
	branch, err := e.repo.CurrentBranch()
	if err != nil {
		return fmt.Errorf("HEAD is detached; check out a branch to squash-rebase")
	}
	if branch == e.trunkLocal || branch == e.trunkRef {
		return fmt.Errorf("refusing to squash-rebase the trunk branch %q onto itself", branch)
	}
	if clean, err := e.repo.IsClean(); err != nil {
		return err
	} else if !clean {
		return fmt.Errorf("working tree has uncommitted changes; commit or stash first")
	}

	// Default target is trunk; onto:BRANCH overrides it.
	onto := c.Args.GetOr("onto", "")
	targetRef := e.trunkRef
	if onto != "" {
		if !e.repo.Exists(onto) {
			return fmt.Errorf("onto branch %q does not exist", onto)
		}
		if onto == branch {
			return fmt.Errorf("onto: must differ from the current branch %q", branch)
		}
		targetRef = onto
	}

	msg := c.Args.GetOr("message", c.Args.GetOr("m", ""))
	edit := c.Args.Flag("edit")

	// Squash fork..branch into one commit against whichever target we rebase onto.
	fork, err := e.repo.MergeBase(targetRef, branch)
	if err != nil {
		fork = targetRef
	}
	existing, _ := e.repo.LogRange(fork + ".." + branch)
	if err := e.squashToOne(branch, fork, len(existing), msg, edit); err != nil {
		return err
	}

	// Trunk target keeps the guard + trunk graph; a named target rebases directly.
	if onto == "" {
		return finalizeOnTrunk(e, c, branch)
	}
	if rerr := e.visualizeRebase(branch, onto, onto); rerr != nil {
		return handleRebaseConflict(e, c, branch, rerr)
	}
	return nil
}
