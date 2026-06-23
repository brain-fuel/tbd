package commands

import (
	"errors"
	"fmt"

	"goforge.dev/tbd/internal/cli"
	"goforge.dev/tbd/internal/invariant"
)

func init() {
	cli.Register(&cli.Command{
		Name:    "feature",
		Summary: "Manage short-lived feature branches off trunk",
		Usage: "tbd feature start NAME       create feature/NAME from trunk head\n" +
			"tbd feature sync [BRANCH]    rebase a feature onto the latest trunk\n" +
			"tbd feature push [BRANCH]    publish the feature branch (force-with-lease)\n" +
			"tbd feature finish [BRANCH]  rebase, fast-forward trunk, push, delete\n" +
			"tbd feature list             list feature branches and their status\n\n" +
			"Flags: :local (skip network) :no-fetch :no-push :keep :no-sync\n" +
			"       :abort-on-conflict :force",
		Run: runFeature,
	})
}

func runFeature(c *cli.Context) error {
	switch c.Args.Pos(0) {
	case "start":
		return featureStart(c)
	case "sync":
		return featureSync(c)
	case "push":
		return featurePush(c)
	case "finish":
		return featureFinish(c)
	case "list", "":
		return featureList(c)
	default:
		return fmt.Errorf("unknown feature subcommand %q (start|sync|push|finish|list)", c.Args.Pos(0))
	}
}

func featureStart(c *cli.Context) error {
	e, err := load(c)
	if err != nil {
		return err
	}
	name := c.Args.Pos(1)
	if name == "" {
		return fmt.Errorf("usage: tbd feature start NAME")
	}
	branch := e.cfg.FeaturePrefix + name
	if e.repo.Exists(branch) {
		return fmt.Errorf("branch %q already exists", branch)
	}
	if e.fetch {
		if err := e.repo.Fetch(e.remote); err != nil {
			return err
		}
	}
	if !e.repo.Exists(e.trunkRef) {
		return fmt.Errorf("trunk %q does not exist; run \"tbd init :create-trunk\"", e.trunkRef)
	}
	if err := e.repo.BranchCreate(branch, e.trunkRef); err != nil {
		return err
	}
	if err := e.repo.Checkout(branch); err != nil {
		return err
	}
	head, _ := e.repo.Short(branch)
	fmt.Fprintln(e.out, e.okMark("created "+branch+" at "+head+" (on top of "+e.trunkLocal+")"))
	return nil
}

func featureSync(c *cli.Context) error {
	e, err := load(c)
	if err != nil {
		return err
	}
	branch, err := resolveFeatureBranch(e, c.Args.Pos(1))
	if err != nil {
		return err
	}
	if err := checkoutIfNeeded(e, branch); err != nil {
		return err
	}

	g := e.guard(true)
	switch err := g.Ensure(branch); {
	case err == nil:
		fmt.Fprintln(e.out, e.okMark(branch+" is already on top of "+e.trunkLocal))
		return nil
	case errors.Is(err, invariant.ErrDiverged):
		// fall through to rebase
	case errors.Is(err, invariant.ErrDirty):
		return fmt.Errorf("working tree has uncommitted changes; commit or stash first")
	default:
		return err
	}

	if err := e.visualizeRebase(branch); err != nil {
		return handleRebaseConflict(e, c, branch, err)
	}
	return nil
}

func featurePush(c *cli.Context) error {
	e, err := load(c)
	if err != nil {
		return err
	}
	branch, err := resolveFeatureBranch(e, c.Args.Pos(1))
	if err != nil {
		return err
	}
	if branch == e.trunkLocal || branch == e.trunkRef {
		return fmt.Errorf("refusing to push the trunk branch %q as a feature; check out a feature branch", branch)
	}
	if e.remote == "" {
		return fmt.Errorf("no remote to push to (a remote is required; do not pass :local)")
	}
	if err := checkoutIfNeeded(e, branch); err != nil {
		return err
	}

	// Keep the published branch honest: fetch and ensure it sits on trunk,
	// rebasing if it has drifted (same policy as finish).
	g := e.guard(true)
	switch err := g.Ensure(branch); {
	case err == nil:
		// already on top of trunk
	case errors.Is(err, invariant.ErrDiverged) && !c.Args.Flag("no-sync"):
		if !e.cfg.AutoRebaseEnabled() {
			return refuseDiverged(e, branch)
		}
		if rerr := e.visualizeRebase(branch); rerr != nil {
			return handleRebaseConflict(e, c, branch, rerr)
		}
	case errors.Is(err, invariant.ErrDiverged):
		return refuseDiverged(e, branch)
	case errors.Is(err, invariant.ErrDirty):
		return fmt.Errorf("working tree has uncommitted changes; run \"tbd commit\" or stash first")
	default:
		return err
	}

	// Force is required because tbd rewrites feature history on every commit.
	if c.Args.Flag("force") {
		if err := e.repo.PushBranchForce(e.remote, branch); err != nil {
			return fmt.Errorf("push %s: %w", branch, err)
		}
	} else if err := e.repo.PushBranchLease(e.remote, branch); err != nil {
		return fmt.Errorf("push %s (someone else may have pushed to it; re-fetch or use :force): %w", branch, err)
	}
	head, _ := e.repo.Short(branch)
	fmt.Fprintln(e.out, e.okMark("pushed "+branch+" @ "+head+" to "+e.remote))
	return nil
}

func featureFinish(c *cli.Context) error {
	e, err := load(c)
	if err != nil {
		return err
	}
	branch, err := resolveFeatureBranch(e, c.Args.Pos(1))
	if err != nil {
		return err
	}
	if branch == e.trunkLocal || branch == e.trunkRef {
		return fmt.Errorf("refusing to finish the trunk branch %q; check out a feature branch first", branch)
	}
	if err := checkoutIfNeeded(e, branch); err != nil {
		return err
	}

	g := e.guard(true)
	err = g.Ensure(branch)
	switch {
	case err == nil:
		// invariant already holds
	case errors.Is(err, invariant.ErrDiverged) && !c.Args.Flag("no-sync"):
		if !e.cfg.AutoRebaseEnabled() {
			return refuseDiverged(e, branch)
		}
		if rerr := e.visualizeRebase(branch); rerr != nil {
			return handleRebaseConflict(e, c, branch, rerr)
		}
	case errors.Is(err, invariant.ErrDiverged):
		return refuseDiverged(e, branch)
	case errors.Is(err, invariant.ErrDirty):
		return fmt.Errorf("working tree has uncommitted changes; commit or stash first")
	default:
		return err
	}

	// Fast-forward trunk to the (now rebased) feature.
	if err := e.ensureLocalTrunk(); err != nil {
		return err
	}
	if err := e.repo.Checkout(e.trunkLocal); err != nil {
		return err
	}
	if err := e.repo.FFMerge(branch); err != nil {
		return fmt.Errorf("could not fast-forward %s (trunk moved again?); re-run finish: %w", e.trunkLocal, err)
	}
	head, _ := e.repo.Short(e.trunkLocal)
	fmt.Fprintln(e.out, e.okMark(e.trunkLocal+" fast-forwarded to "+head))

	if e.remote != "" && !c.Args.Flag("no-push") {
		if err := e.repo.PushBranch(e.remote, e.trunkLocal); err != nil {
			return fmt.Errorf("push %s: %w", e.trunkLocal, err)
		}
		fmt.Fprintln(e.out, e.okMark("pushed "+e.trunkLocal+" to "+e.remote))
	}

	if !c.Args.Flag("keep") {
		if err := e.repo.BranchDelete(branch); err != nil {
			return err
		}
		if e.remote != "" && !c.Args.Flag("no-push") && e.repo.RemoteHasBranch(e.remote, branch) {
			if err := e.repo.PushDeleteBranch(e.remote, branch); err != nil {
				fmt.Fprintln(e.errOut, e.colors.Dim("note: remote branch "+branch+" not deleted ("+err.Error()+")"))
			}
		}
		fmt.Fprintln(e.out, e.okMark("deleted "+branch))
	}
	return nil
}

func featureList(c *cli.Context) error {
	e, err := load(c)
	if err != nil {
		return err
	}
	branches, err := e.repo.ListBranches(e.cfg.FeaturePrefix + "*")
	if err != nil {
		return err
	}
	if len(branches) == 0 {
		fmt.Fprintln(e.out, e.colors.Dim("no feature branches"))
		return nil
	}
	for _, b := range branches {
		ahead, behind, _ := e.repo.AheadBehind(e.trunkRef, b)
		head, _ := e.repo.Short(b)
		marker := e.okMark("rebased")
		if behind > 0 {
			marker = e.badMark("behind trunk")
		}
		fmt.Fprintf(e.out, "%-28s %s  +%d/-%d  %s\n", b, head, ahead, behind, marker)
	}
	return nil
}

// resolveFeatureBranch returns the explicit branch arg, or the current branch.
func resolveFeatureBranch(e env, arg string) (string, error) {
	if arg != "" {
		if !e.repo.Exists(arg) {
			return "", fmt.Errorf("branch %q does not exist", arg)
		}
		return arg, nil
	}
	br, err := e.repo.CurrentBranch()
	if err != nil {
		return "", fmt.Errorf("HEAD is detached; pass a branch name")
	}
	return br, nil
}

func checkoutIfNeeded(e env, branch string) error {
	cur, err := e.repo.CurrentBranch()
	if err == nil && cur == branch {
		return nil
	}
	return e.repo.Checkout(branch)
}

func refuseDiverged(e env, branch string) error {
	rep, _ := e.guard(false).Check(branch)
	fmt.Fprintln(e.errOut, e.badMark(branch+" has diverged from "+e.trunkLocal))
	fmt.Fprintf(e.errOut, "  trunk %s @ %s, %s ahead %d / behind %d\n",
		e.trunkRef, short(rep.TrunkHead), branch, rep.Ahead, rep.Behind)
	fmt.Fprintln(e.errOut, e.colors.Dim("  auto-rebase is off; run \"tbd feature sync "+branch+"\" first"))
	return cli.ExitError{Code: 1}
}
