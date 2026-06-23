package commands

import (
	"fmt"
	"io"
	"time"

	"goforge.dev/tbd/internal/argv"
	"goforge.dev/tbd/internal/cli"
)

func init() {
	cli.Register(&cli.Command{
		Name:    "lease",
		Summary: "Claim a deploy slot (tag or ephemeral branch), gated by who holds it",
		Usage: "tbd lease NAME [to:REF] [:no-advance] [:force]   claim/move the deploy slot\n" +
			"tbd lease take NAME [...]                        same (explicit verb)\n" +
			"tbd lease status                                 show each deploy slot and its holder\n\n" +
			"The lease-strategy in .tbd.yaml selects the mechanism:\n" +
			"  none             leasing is disabled\n" +
			"  tag              moves a lease-tag at the DAG level (bootstrap/advance/take)\n" +
			"  ephemeral-branch blows away a lease-branch and remakes it at your tip,\n" +
			"                   every time, so the branch never lives unless leased\n\n" +
			"Every move is compare-and-swap so two people cannot grab the same slot at once.",
		Spec: argv.Spec{
			Named: argv.Opts("to"),
			Flags: argv.Opts("no-advance", "force"),
			Hints: map[string]string{
				"strategy":       "the lease mechanism is set in .tbd.yaml (lease-strategy: none|tag|ephemeral-branch), not on the command line",
				"lease-strategy": "set lease-strategy in .tbd.yaml, not on the command line",
			},
		},
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
		return leaseAcquire(c, sub)
	}
}

func inList(list []string, name string) bool {
	for _, x := range list {
		if x == name {
			return true
		}
	}
	return false
}

func isConfiguredLease(e env, name string) bool { return inList(e.cfg.LeaseTags, name) }

// leaseAcquire dispatches on the configured strategy. The tag and
// ephemeral-branch paths share no logic.
func leaseAcquire(c *cli.Context, name string) error {
	e, err := load(c)
	if err != nil {
		return err
	}
	if name == "" {
		return fmt.Errorf("usage: tbd lease NAME")
	}
	switch e.cfg.LeaseStrategy {
	case "none":
		return fmt.Errorf("leasing is disabled (lease-strategy: none)")
	case "ephemeral-branch":
		return leaseEphemeral(e, c, name)
	default: // "tag" (and "" defaults to tag)
		return leaseTag(e, c, name)
	}
}

// ---------------------------------------------------------------------------
// ephemeral-branch strategy: every lease blows the branch away and remakes it
// at the caller's working-branch tip. The branch never exists unless leased.
// ---------------------------------------------------------------------------

func leaseEphemeral(e env, c *cli.Context, name string) error {
	if !inList(e.cfg.LeaseBranches, name) {
		return fmt.Errorf("%q is not a configured lease-branch %v", name, e.cfg.LeaseBranches)
	}
	working, err := e.repo.CurrentBranch()
	if err != nil {
		return fmt.Errorf("HEAD is detached; check out your feature branch to deploy from")
	}
	if working == name {
		return fmt.Errorf("you are on the ephemeral lease branch %q; check out your feature branch first", name)
	}
	wHead, err := e.repo.RevParse(working)
	if err != nil {
		return err
	}

	// The remote value we are taking from (also the compare-and-swap baseline).
	expected := ""
	if e.remote != "" {
		expected = e.repo.RemoteBranchSha(e.remote, name)
	}

	// Blow it away and remake it at our tip, locally.
	if e.repo.Exists("refs/heads/" + name) {
		if err := e.repo.BranchDelete(name); err != nil {
			return err
		}
	}
	if err := e.repo.BranchCreate(name, wHead); err != nil {
		return err
	}
	short, _ := e.repo.Short(name)

	if e.remote == "" {
		fmt.Fprintln(e.out, e.okMark("leased "+name+" -> "+short+" (ephemeral branch remade at your tip)"))
		fmt.Fprintln(e.out, e.colors.Dim("  (local only, no remote configured, so no cross-machine mutual exclusion)"))
		return nil
	}

	if c.Args.Flag("force") {
		if err := e.repo.PushBranchForce(e.remote, name); err != nil {
			return fmt.Errorf("force-push lease branch %s: %w", name, err)
		}
	} else if err := e.repo.PushBranchCAS(e.remote, name, expected); err != nil {
		now := e.repo.RemoteBranchSha(e.remote, name)
		fmt.Fprintln(e.errOut, e.badMark("lease branch "+name+" was taken by someone else"))
		if now != "" {
			fmt.Fprintf(e.errOut, "  remote now at %s\n", shorten(now))
		}
		fmt.Fprintln(e.errOut, e.colors.Dim("  re-run \"tbd lease "+name+"\" to take it from there, or pass :force"))
		return cli.ExitError{Code: 1}
	}
	fmt.Fprintln(e.out, e.okMark("leased "+name+" -> "+short+" (ephemeral branch remade at your tip)"))
	return nil
}

func shorten(sha string) string {
	if len(sha) > 12 {
		return sha[:12]
	}
	return sha
}

// ---------------------------------------------------------------------------
// tag strategy (unchanged): move a lease-tag at the DAG level.
// ---------------------------------------------------------------------------

type leaseClass int

const (
	leaseUnset leaseClass = iota
	leaseCurrent
	leaseStale
	leaseForeign
)

func leaseTag(e env, c *cli.Context, name string) error {
	if !isConfiguredLease(e, name) {
		return fmt.Errorf("%q is not a configured lease-tag %v", name, e.cfg.LeaseTags)
	}
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

	target := e.repo.CommitOf(name)
	expectedOld := e.repo.RefSha("refs/tags/" + name)

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
		dest, _ = e.repo.RevParse(e.trunkRef)
	default:
		dest = wHead
	}

	switch classifyLease(e, target, dest, working, wHead) {
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

	return moveLeaseTag(e, c, name, dest, expectedOld)
}

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

func moveLeaseTag(e env, c *cli.Context, name, dest, expectedOld string) error {
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

// ---------------------------------------------------------------------------
// status (strategy-aware)
// ---------------------------------------------------------------------------

func leaseStatus(c *cli.Context) error {
	e, err := load(c)
	if err != nil {
		return err
	}
	if e.cfg.LeaseStrategy == "tag" || e.cfg.LeaseStrategy == "" {
		if e.remote != "" {
			_ = e.repo.FetchTags(e.remote)
		}
	}
	writeLeaseStatus(e, e.out, "")
	return nil
}

// writeLeaseStatus renders the deploy slots for the active strategy. Shared by
// the status command and lease status, with a caller-supplied indent.
func writeLeaseStatus(e env, w io.Writer, indent string) {
	switch e.cfg.LeaseStrategy {
	case "none":
		fmt.Fprintln(w, indent+e.colors.Dim("leasing disabled (lease-strategy: none)"))
	case "ephemeral-branch":
		if len(e.cfg.LeaseBranches) == 0 {
			fmt.Fprintln(w, indent+e.colors.Dim("no lease-branches configured"))
			return
		}
		for _, name := range e.cfg.LeaseBranches {
			sha := e.repo.CommitOf(name)
			if sha == "" {
				fmt.Fprintf(w, "%s%-16s %s\n", indent, name, e.colors.Dim("(unleased)"))
				continue
			}
			short, _ := e.repo.Short(name)
			who := e.repo.CommitterName(name)
			fmt.Fprintf(w, "%s%-16s %s  %s  %s\n", indent, name, short,
				e.colors.Cyan("ephemeral"), e.colors.Dim("by "+who))
		}
	default: // tag
		if len(e.cfg.LeaseTags) == 0 {
			fmt.Fprintln(w, indent+e.colors.Dim("no lease-tags configured"))
			return
		}
		trunkHead, _ := e.repo.RevParse(e.trunkRef)
		for _, name := range e.cfg.LeaseTags {
			d, ok := e.repo.TagInfo(name)
			if !ok {
				fmt.Fprintf(w, "%s%-16s %s\n", indent, name, e.colors.Dim("(unset)"))
				continue
			}
			loc := leaseLocation(e, name, trunkHead)
			holder := d.Tagger
			if holder == "" {
				holder = "?"
			}
			fmt.Fprintf(w, "%s%-16s %s  %s  held by %s @ %s\n", indent, name, d.Short, loc, holder, d.Date)
		}
	}
}

// leaseLocation describes, for the tag strategy, where a lease sits in the DAG.
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
