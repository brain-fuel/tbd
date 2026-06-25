package commands

import (
	"fmt"

	"goforge.dev/tbd/internal/argv"
	"goforge.dev/tbd/internal/cli"
)

func init() {
	cli.Register(&cli.Command{
		Name:    "cherry-put",
		Summary: "Squash the current branch's work onto another branch as a new branch",
		Usage: "tbd cherry-put onto:BRANCH as:NEWBRANCH [message:\"...\"] [:edit]\n\n" +
			"Does for an arbitrary branch what rebase does for trunk: takes the current\n" +
			"branch's work, squashes it into one commit on top of onto:BRANCH, and saves\n" +
			"the result as a new branch as:NEWBRANCH (which keeps the ref alive). Your\n" +
			"current branch is left untouched.",
		Spec: argv.Spec{
			Named: argv.Opts("onto", "as", "message", "m"),
			Flags: argv.Opts("edit"),
		},
		Run: runCherryPut,
	})
}

func runCherryPut(c *cli.Context) error {
	e, err := load(c)
	if err != nil {
		return err
	}
	onto := c.Args.GetOr("onto", "")
	as := c.Args.GetOr("as", "")
	if onto == "" || as == "" {
		return fmt.Errorf("usage: tbd cherry-put onto:BRANCH as:NEWBRANCH")
	}

	src, err := e.repo.CurrentBranch()
	if err != nil {
		return fmt.Errorf("HEAD is detached; check out the branch whose work you want to put")
	}
	if !e.repo.Exists(onto) {
		return fmt.Errorf("onto branch %q does not exist", onto)
	}
	if e.repo.Exists(as) {
		return fmt.Errorf("branch %q already exists; pick a fresh as: name", as)
	}
	if as == src {
		return fmt.Errorf("as: must be a new branch name, not the current branch %q", src)
	}
	if clean, err := e.repo.IsClean(); err != nil {
		return err
	} else if !clean {
		return fmt.Errorf("working tree has uncommitted changes; commit or stash first")
	}

	msg := c.Args.GetOr("message", c.Args.GetOr("m", ""))
	if msg == "" {
		msg = e.repo.FullMessage(src)
	}

	// Create the new branch at onto and squash the source's work into one commit.
	if err := e.repo.BranchCreate(as, onto); err != nil {
		return err
	}
	if err := e.repo.Checkout(as); err != nil {
		return err
	}
	if err := e.repo.MergeSquash(src); err != nil {
		return fmt.Errorf("squashing %s onto %s hit a conflict; resolve it, \"git add\" and "+
			"\"git commit\" to finish on %s, or \"git merge --abort\" and delete %s: %w", src, onto, as, as, err)
	}
	if !e.repo.HasStaged() {
		fmt.Fprintln(e.out, e.okMark(as+" created at "+onto+" (nothing to put: "+src+" has no changes over "+onto+")"))
		return nil
	}
	if c.Args.Flag("edit") {
		if err := e.repo.CommitInteractive(false, msg); err != nil {
			return err
		}
	} else if err := e.repo.Commit(msg); err != nil {
		return err
	}
	short, _ := e.repo.Short(as)
	fmt.Fprintln(e.out, e.okMark("cherry-put "+src+" onto "+onto+" as "+as+" @ "+short+" (single commit)"))
	fmt.Fprintln(e.out, e.colors.Dim("  you are now on "+as+"; "+src+" is unchanged"))
	return nil
}
