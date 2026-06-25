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
		Usage: "tbd cherry-put onto:BRANCH as:NEWBRANCH [message:\"...\"] [:edit]\n\n" +
			"Squashes the current branch's work into a single commit, then cherry-picks\n" +
			"that one commit onto onto:BRANCH as a new branch as:NEWBRANCH, so it sits on\n" +
			"top linearly with no merge commit, exactly as if you had rebased it there\n" +
			"yourself. If the replay conflicts, fix it and run \"tbd continue\".",
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

	// --- squash on YOUR branch into one commit ---
	if len(existing) > 1 {
		keep := msg
		if keep == "" {
			keep = e.repo.FullMessage(src)
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
		fmt.Fprintf(e.out, "%s\n", e.okMark(fmt.Sprintf("squashed %d commits into one on %s", len(existing), src)))
	} else if msg != "" || edit {
		if edit {
			if err := e.repo.CommitInteractive(true, msg); err != nil {
				return err
			}
		} else if err := e.repo.CommitAmend(msg); err != nil {
			return err
		}
	}

	// --- replay that single commit onto `onto` as `as` (linear, no merge commit) ---
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

	short, _ := e.repo.Short(as)
	fmt.Fprintln(e.out, e.okMark("cherry-put "+src+" onto "+onto+" as "+as+" @ "+short+" (one commit, no merge)"))
	fmt.Fprintln(e.out, e.colors.Dim("  you are on "+as+"; "+src+" keeps the squashed commit"))
	return nil
}
