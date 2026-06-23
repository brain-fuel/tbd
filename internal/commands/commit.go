package commands

import (
	"errors"
	"fmt"

	"goforge.dev/tbd/internal/cli"
	"goforge.dev/tbd/internal/invariant"
)

func init() {
	cli.Register(&cli.Command{
		Name:    "commit",
		Summary: "Fold all work into the feature's single commit, then rebase onto trunk",
		Usage: "tbd commit [message:\"...\"] [m:\"...\"] [:local] [:no-fetch] [:abort-on-conflict]\n\n" +
			"Every invocation does the same three things, always:\n" +
			"  1. stage all changes and collapse the feature to exactly ONE commit\n" +
			"     (create it, amend it, or squash several into one)\n" +
			"  2. fetch the trunk\n" +
			"  3. rebase that single commit onto the latest trunk head\n\n" +
			"A message is required only for the feature's first commit; later commits\n" +
			"keep the existing message unless you pass one.\n\n" +
			"If the rebase in step 3 conflicts, the commit is already made; fix the\n" +
			"files, \"git add\" them, and run \"tbd continue\" (or :abort-on-conflict to\n" +
			"back the rebase out).",
		Run: runCommit,
	})
}

func runCommit(c *cli.Context) error {
	e, err := load(c)
	if err != nil {
		return err
	}
	branch, err := e.repo.CurrentBranch()
	if err != nil {
		return fmt.Errorf("HEAD is detached; check out a feature branch first")
	}
	if branch == e.trunkLocal || branch == e.trunkRef {
		return fmt.Errorf("refusing to commit on the trunk branch %q; check out a feature branch", branch)
	}

	msg := c.Args.GetOr("message", c.Args.GetOr("m", ""))

	// --- 1. collapse the feature to a single commit ---
	if err := e.repo.StageAll(); err != nil {
		return err
	}
	fork, err := e.repo.MergeBase(e.trunkRef, branch)
	if err != nil {
		// No common ancestor yet (e.g. trunk unfetched); fall back to trunk ref.
		fork = e.trunkRef
	}
	existing, _ := e.repo.LogRange(fork + ".." + branch)
	switch n := len(existing); {
	case n == 0:
		if !e.repo.HasStaged() {
			return fmt.Errorf("nothing to commit: make some changes first")
		}
		if msg == "" {
			return fmt.Errorf("the feature's first commit needs a message: tbd commit message:\"...\"")
		}
		if err := e.repo.Commit(msg); err != nil {
			return err
		}
		fmt.Fprintln(e.out, e.okMark("created the feature commit"))
	case n == 1:
		if err := e.repo.CommitAmend(msg); err != nil {
			return err
		}
		fmt.Fprintln(e.out, e.okMark("amended the feature commit"))
	default:
		// Squash several commits (and any staged changes) into one.
		keep := msg
		if keep == "" {
			keep = e.repo.FullMessage(branch) // reuse the latest message
		}
		if err := e.repo.ResetSoft(fork); err != nil {
			return err
		}
		if err := e.repo.Commit(keep); err != nil {
			return err
		}
		fmt.Fprintf(e.out, "%s\n", e.okMark(fmt.Sprintf("squashed %d commits into one", n)))
	}

	// --- 2 + 3. fetch trunk and rebase the single commit onto it ---
	g := e.guard(true)
	switch err := g.Ensure(branch); {
	case err == nil:
		head, _ := e.repo.Short(branch)
		fmt.Fprintln(e.out, e.okMark(branch+" @ "+head+" sits on top of "+e.trunkLocal))
		return nil
	case errors.Is(err, invariant.ErrDiverged):
		if rerr := e.visualizeRebase(branch); rerr != nil {
			return handleRebaseConflict(e, c, branch, rerr)
		}
		return nil
	case errors.Is(err, invariant.ErrDirty):
		// Should not happen (we just committed), but report honestly if it does.
		return fmt.Errorf("working tree still has uncommitted changes after commit")
	default:
		return err
	}
}
