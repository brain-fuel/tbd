package commands

import (
	"errors"
	"fmt"

	"goforge.dev/tbd/v2/internal/argv"
	"goforge.dev/tbd/v2/internal/cli"
	"goforge.dev/tbd/v2/internal/invariant"
)

func init() {
	cli.Register(&cli.Command{
		Name:    "commit",
		Summary: "Fold all work into the feature's single commit, then rebase onto trunk",
		Usage: "tbd commit [message:\"...\"] [m:\"...\"] [:edit] [:local] [:no-fetch] [:abort-on-conflict]\n\n" +
			"Every invocation does the same three things, always:\n" +
			"  1. stage all changes and collapse the feature to exactly ONE commit\n" +
			"     (create it, amend it, or squash several into one)\n" +
			"  2. fetch the trunk\n" +
			"  3. rebase that single commit onto the latest trunk head\n\n" +
			"A message is required only for the feature's first commit; later commits\n" +
			"keep the existing message unless you pass one. Use :edit to open your\n" +
			"editor on the message (e.g. to reword); it works on the first commit,\n" +
			"an amend, or a squash, and still collapses-to-one and rebases.\n\n" +
			"If the rebase in step 3 conflicts, the commit is already made; fix the\n" +
			"files, \"git add\" them, and run \"tbd continue\" (or :abort-on-conflict to\n" +
			"back the rebase out).",
		Spec: argv.Spec{
			Named: argv.Opts("message", "m"),
			Flags: argv.Opts("edit", "abort-on-conflict"),
		},
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
	edit := c.Args.Flag("edit")
	existing, _ := e.repo.LogRange(fork + ".." + branch)
	switch n := len(existing); {
	case n == 0:
		if !e.repo.HasStaged() {
			return fmt.Errorf("nothing to commit: make some changes first")
		}
		if edit {
			if err := e.repo.CommitInteractive(false, msg); err != nil {
				return err
			}
		} else {
			if msg == "" {
				return fmt.Errorf("the feature's first commit needs a message: tbd commit message:\"...\" (or :edit)")
			}
			if err := e.repo.Commit(msg); err != nil {
				return err
			}
		}
		fmt.Fprintln(e.out, e.okMark("created the feature commit"))
	case n == 1:
		if edit {
			if err := e.repo.CommitInteractive(true, msg); err != nil {
				return err
			}
		} else if err := e.repo.CommitAmend(msg); err != nil {
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
		if edit {
			if err := e.repo.CommitInteractive(false, keep); err != nil {
				return err
			}
		} else if err := e.repo.Commit(keep); err != nil {
			return err
		}
		fmt.Fprintf(e.out, "%s\n", e.okMark(fmt.Sprintf("squashed %d commits into one", n)))
	}

	// --- 2 + 3. fetch trunk and rebase the single commit onto it ---
	return finalizeOnTrunk(e, c, branch)
}

// finalizeOnTrunk fetches the trunk and ensures branch sits on top of it,
// rebasing (with the visualization and conflict handling) when it has diverged.
// Shared by commit and rebase.
func finalizeOnTrunk(e env, c *cli.Context, branch string) error {
	g := e.guard(true)
	switch err := g.Ensure(branch); {
	case err == nil:
		head, _ := e.repo.Short(branch)
		fmt.Fprintln(e.out, e.okMark(branch+" @ "+head+" sits on top of "+e.trunkLocal))
		return nil
	case errors.Is(err, invariant.ErrDiverged):
		if rerr := e.visualizeRebase(branch, e.trunkRef, e.trunkLocal); rerr != nil {
			return handleRebaseConflict(e, c, branch, rerr)
		}
		return nil
	case errors.Is(err, invariant.ErrDirty):
		return fmt.Errorf("working tree has uncommitted changes; commit or stash first")
	default:
		return err
	}
}
