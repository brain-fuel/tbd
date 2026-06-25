package commands

import (
	"fmt"

	"goforge.dev/tbd/internal/argv"
	"goforge.dev/tbd/internal/cli"
)

func init() {
	cli.Register(&cli.Command{
		Name:    "cherry-put",
		Summary: "Squash the current branch and replay it as one commit onto another branch",
		Usage: "tbd cherry-put onto:BRANCH as:NEWBRANCH [message:\"...\"] [:edit] [:keep-source]\n\n" +
			"Squashes the current branch's work into a single commit, then replays that\n" +
			"one commit onto onto:BRANCH as a new branch as:NEWBRANCH, so it sits on top\n" +
			"linearly with no merge commit, exactly as if you had rebased it there\n" +
			"yourself. By default the current branch is squashed too; :keep-source leaves\n" +
			"it untouched (the squash happens on the new branch). If the replay conflicts,\n" +
			"fix it and run \"tbd continue\".",
		Spec: argv.Spec{
			Named: argv.Opts("onto", "as", "message", "m"),
			Flags: argv.Opts("edit", "keep-source"),
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

	fork, err := e.repo.MergeBase(onto, src)
	if err != nil {
		return fmt.Errorf("%q and %q have no common ancestor", src, onto)
	}
	existing, _ := e.repo.LogRange(fork + ".." + src)
	if len(existing) == 0 {
		return fmt.Errorf("nothing to put: %s has no commits over %s", src, onto)
	}

	msg := c.Args.GetOr("message", c.Args.GetOr("m", ""))
	edit := c.Args.Flag("edit")

	if c.Args.Flag("keep-source") {
		// Squash on a copy and rebase it; the source branch is never touched.
		if err := e.repo.BranchCreate(as, src); err != nil {
			return err
		}
		if err := e.repo.Checkout(as); err != nil {
			return err
		}
		if err := e.squashToOne(as, fork, len(existing), msg, edit); err != nil {
			return err
		}
		if rerr := e.step("rebasing "+as+" onto "+onto, func() error { return e.repo.Rebase(onto) }); rerr != nil {
			return handleRebaseConflict(e, c, as, fmt.Errorf("%w: %v", ErrRebaseConflict, rerr))
		}
	} else {
		// Squash the source in place, then cherry-pick the single commit.
		if err := e.squashToOne(src, fork, len(existing), msg, edit); err != nil {
			return err
		}
		payload, err := e.repo.RevParse(src)
		if err != nil {
			return err
		}
		if err := e.repo.BranchCreate(as, onto); err != nil {
			return err
		}
		if err := e.repo.Checkout(as); err != nil {
			return err
		}
		if err := e.step("placing the commit onto "+onto, func() error { return e.repo.CherryPick(payload) }); err != nil {
			if e.repo.CherryPickInProgress() {
				fmt.Fprintln(e.errOut, e.badMark("cherry-put hit a conflict placing the commit onto "+onto))
				fmt.Fprintln(e.errOut, e.colors.Dim("  fix the files, \"git add\" them, then \"tbd continue\","))
				fmt.Fprintln(e.errOut, e.colors.Dim("  or \"tbd continue :abort\" to undo (leaves "+as+" at "+onto+")"))
				return cli.ExitError{Code: 1}
			}
			return err
		}
	}

	short, _ := e.repo.Short(as)
	fmt.Fprintln(e.out, e.okMark("cherry-put "+src+" onto "+onto+" as "+as+" @ "+short+" (one commit, no merge)"))
	if c.Args.Flag("keep-source") {
		fmt.Fprintln(e.out, e.colors.Dim("  you are on "+as+"; "+src+" is unchanged"))
	} else {
		fmt.Fprintln(e.out, e.colors.Dim("  you are on "+as+"; "+src+" keeps the squashed commit"))
	}
	return nil
}

// squashToOne collapses the checked-out branch (fork..HEAD) into a single commit.
// With more than one commit it squashes via a soft reset; with one it only
// rewords when a message or :edit was given.
func (e env) squashToOne(branch, fork string, n int, msg string, edit bool) error {
	switch {
	case n > 1:
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
		fmt.Fprintf(e.out, "%s\n", e.okMark(fmt.Sprintf("squashed %d commits into one", n)))
	case msg != "" || edit:
		if edit {
			return e.repo.CommitInteractive(true, msg)
		}
		return e.repo.CommitAmend(msg)
	}
	return nil
}
