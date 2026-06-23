package commands

import (
	"fmt"
	"time"

	"goforge.dev/tbd/internal/cli"
)

func init() {
	cli.Register(&cli.Command{
		Name:    "lease",
		Summary: "Move a deploy tag to your work, gated on who currently holds it",
		Usage: "tbd lease NAME [to:REF] [:no-advance] [:force]   acquire/advance the deploy tag\n" +
			"tbd lease take NAME [...]                        same (explicit verb)\n" +
			"tbd lease status                                 show each deploy tag and its holder\n\n" +
			"The move is decided at the DAG level from where NAME points now (T) relative\n" +
			"to your working branch (W):\n" +
			"  unset            -> bootstrap to trunk head\n" +
			"  already at target-> no-op\n" +
			"  on W / in W's reflog (your earlier or pre-amend commit) -> advance to W's tip\n" +
			"                      (:no-advance leaves it where it is)\n" +
			"  on someone else's branch -> take it to W's tip\n\n" +
			"The move is compare-and-swap (force-with-lease) so two people cannot grab the\n" +
			"same deploy slot at once. NAME must be one of the configured lease-tags.",
		Run: runLease,
	})
}

func runLease(c *cli.Context) error {
	switch sub := c.Args.Pos(0); sub {
	case "status", "":
		return leaseStatus(c)
	case "take":
		return leaseAcquire(c, c.Args.Pos(1))
	default:
		// "tbd lease NAME" shorthand.
		return leaseAcquire(c, sub)
	}
}

func isConfiguredLease(e env, name string) bool {
	for _, t := range e.cfg.LeaseTags {
		if t == name {
			return true
		}
	}
	return false
}

// leaseClass is where the current lease target sits relative to the working branch.
type leaseClass int

const (
	leaseUnset   leaseClass = iota // tag does not exist yet
	leaseCurrent                   // already at the target we would move to
	leaseStale                     // an earlier/pre-amend commit of the working branch
	leaseForeign                   // on someone else's branch (or an unrelated/orphaned commit)
)

func leaseAcquire(c *cli.Context, name string) error {
	e, err := load(c)
	if err != nil {
		return err
	}
	if name == "" {
		return fmt.Errorf("usage: tbd lease NAME [to:REF]")
	}
	if !isConfiguredLease(e, name) {
		return fmt.Errorf("%q is not a configured lease-tag %v", name, e.cfg.LeaseTags)
	}

	// Fetch truth: commits (for trunk head) and tags (the authoritative current
	// lease position, also our compare-and-swap baseline).
	if e.remote != "" {
		if err := e.repo.Fetch(e.remote); err != nil {
			return err
		}
		if err := e.repo.FetchTags(e.remote); err != nil {
			return err
		}
	}

	working, err := e.repo.CurrentBranch()
	if err != nil {
		return fmt.Errorf("HEAD is detached; check out a working branch to deploy from")
	}
	wHead, err := e.repo.RevParse(working)
	if err != nil {
		return err
	}

	// Current lease target commit (peeled), and the tag-object sha used as the
	// CAS baseline. Empty when the lease is unset.
	target := e.repo.CommitOf(name) // commit the tag points at, or ""
	expectedOld := e.repo.RefSha("refs/tags/" + name)

	// Decide where the lease should move to.
	var dest string
	switch {
	case c.Args.GetOr("to", "") != "":
		dest = c.Args.GetOr("to", "")
		if !e.repo.Exists(dest) {
			return fmt.Errorf("target ref %q does not exist", dest)
		}
		d, _ := e.repo.RevParse(dest)
		dest = d
	case target == "":
		dest, _ = e.repo.RevParse(e.trunkRef) // bootstrap to trunk head
	default:
		dest = wHead // advance/take to your working branch tip
	}

	// Classify the current target.
	class := classifyLease(e, target, dest, working, wHead)

	switch class {
	case leaseCurrent:
		fmt.Fprintln(e.out, e.okMark(name+" already deployed to "+shortOf(e.repo, dest)))
		return nil
	case leaseStale:
		if c.Args.Flag("no-advance") {
			fmt.Fprintln(e.out, e.colors.Dim(name+" is on your earlier commit; :no-advance set, leaving it"))
			return nil
		}
		fmt.Fprintln(e.out, e.colors.Bold("advancing "+name+" to your latest commit"))
	case leaseForeign:
		where := "another branch"
		if bs := e.repo.BranchesContaining(target); len(bs) > 0 {
			where = bs[0]
		} else if target != "" {
			where = "an orphaned commit"
		}
		fmt.Fprintln(e.out, e.colors.Bold("taking "+name+" (currently on "+where+")"))
	case leaseUnset:
		fmt.Fprintln(e.out, e.colors.Bold("initializing "+name+" at trunk head"))
	}

	return moveLease(e, c, name, dest, expectedOld)
}

// classifyLease decides where the current target sits relative to the working branch.
func classifyLease(e env, target, dest, working, wHead string) leaseClass {
	if target == "" {
		return leaseUnset
	}
	if target == dest {
		return leaseCurrent
	}
	if e.repo.IsAncestor(target, wHead) || e.repo.ReflogContains(working, target) {
		return leaseStale
	}
	return leaseForeign
}

// moveLease writes and publishes the tag via compare-and-swap.
func moveLease(e env, c *cli.Context, name, dest, expectedOld string) error {
	short := shortOf(e.repo, dest)
	msg := fmt.Sprintf("deploy lease %s -> %s at %s", name, short, time.Now().Format(time.RFC3339))
	if err := e.repo.TagAnnotated(name, dest, msg); err != nil {
		return err
	}

	if e.remote != "" {
		if e.cfg.TagPush == "force" || c.Args.Flag("force") {
			if err := e.repo.PushTagForce(e.remote, name); err != nil {
				return fmt.Errorf("force-push lease %s: %w", name, err)
			}
		} else if err := e.repo.PushTagCAS(e.remote, name, expectedOld); err != nil {
			_ = e.repo.FetchTags(e.remote)
			if d, ok := e.repo.TagInfo(name); ok {
				fmt.Fprintln(e.errOut, e.badMark("lease "+name+" was taken by someone else"))
				fmt.Fprintf(e.errOut, "  now held by %s (%s) at %s\n", d.Tagger, d.Short, d.Date)
				fmt.Fprintln(e.errOut, e.colors.Dim("  re-run \"tbd lease "+name+"\" to take it from the new position, or pass :force"))
			} else {
				fmt.Fprintln(e.errOut, e.badMark("lease push for "+name+" was rejected: "+err.Error()))
			}
			return cli.ExitError{Code: 1}
		}
	}

	d, _ := e.repo.TagInfo(name)
	fmt.Fprintln(e.out, e.okMark(name+" -> "+short))
	if d.Tagger != "" {
		fmt.Fprintf(e.out, "  held by %s at %s\n", d.Tagger, d.Date)
	}
	if e.remote == "" {
		fmt.Fprintln(e.out, e.colors.Dim("  (local only, no remote configured, so no cross-machine mutual exclusion)"))
	}
	return nil
}

func shortOf(repo interface{ Short(string) (string, error) }, ref string) string {
	s, _ := repo.Short(ref)
	if s == "" {
		return ref
	}
	return s
}

func leaseStatus(c *cli.Context) error {
	e, err := load(c)
	if err != nil {
		return err
	}
	if e.remote != "" {
		_ = e.repo.FetchTags(e.remote)
	}
	if len(e.cfg.LeaseTags) == 0 {
		fmt.Fprintln(e.out, e.colors.Dim("no lease-tags configured"))
		return nil
	}
	trunkHead, _ := e.repo.RevParse(e.trunkRef)
	for _, name := range e.cfg.LeaseTags {
		d, ok := e.repo.TagInfo(name)
		if !ok {
			fmt.Fprintf(e.out, "%-16s %s\n", name, e.colors.Dim("(unset)"))
			continue
		}
		loc := leaseLocation(e, name, trunkHead)
		holder := d.Tagger
		if holder == "" {
			holder = "?"
		}
		fmt.Fprintf(e.out, "%-16s %s  %s  held by %s @ %s\n", name, d.Short, loc, holder, d.Date)
	}
	return nil
}

// leaseLocation describes, for status, where a lease sits in the DAG.
func leaseLocation(e env, name, trunkHead string) string {
	target := e.repo.CommitOf(name)
	if target != "" && trunkHead != "" && e.repo.IsAncestor(target, trunkHead) {
		return e.okMark("on trunk")
	}
	if bs := e.repo.BranchesContaining(target); len(bs) > 0 {
		return e.colors.Cyan("on " + bs[0])
	}
	return e.colors.Yellow("orphaned")
}
