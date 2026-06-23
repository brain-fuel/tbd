package commands

import (
	"fmt"
	"time"

	"goforge.dev/tbd/internal/cli"
)

func init() {
	cli.Register(&cli.Command{
		Name:    "lease",
		Summary: "Borrow a deploy tag (CAS-guarded) to trigger CD safely",
		Usage: "tbd lease take NAME [to:REF] [:force]   move the deploy tag to a trunk commit\n" +
			"tbd lease status                       show each deploy tag and its holder\n\n" +
			"A lease moves NAME to a trunk commit via compare-and-swap: the push only\n" +
			"succeeds if no one moved the tag since you fetched, so two people cannot\n" +
			"kick off the same deployment at once. The holder is recorded in the tag's\n" +
			"tagger field. NAME must be one of the configured lease-tags.",
		Run: runLease,
	})
}

func runLease(c *cli.Context) error {
	switch c.Args.Pos(0) {
	case "take":
		return leaseTake(c)
	case "status", "":
		return leaseStatus(c)
	default:
		return fmt.Errorf("unknown lease subcommand %q (take|status)", c.Args.Pos(0))
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

func leaseTake(c *cli.Context) error {
	e, err := load(c)
	if err != nil {
		return err
	}
	name := c.Args.Pos(1)
	if name == "" {
		return fmt.Errorf("usage: tbd lease take NAME [to:REF]")
	}
	if !isConfiguredLease(e, name) {
		return fmt.Errorf("%q is not a configured lease-tag %v", name, e.cfg.LeaseTags)
	}

	// Fetch trunk commits so we can target the latest trunk head. This does not
	// clobber the existing local lease tag, so the value we last saw for it
	// remains our compare-and-swap baseline (the point of --force-with-lease).
	if e.remote != "" {
		if err := e.repo.Fetch(e.remote); err != nil {
			return err
		}
	}
	to := c.Args.GetOr("to", e.trunkRef)
	if !e.repo.Exists(to) {
		return fmt.Errorf("target ref %q does not exist", to)
	}

	// Invariant: you may only deploy a commit that is on trunk.
	onTrunk, err := e.guard(false).OnTrunk(to)
	if err != nil {
		return err
	}
	if !onTrunk {
		return fmt.Errorf("%q is not on trunk %s; you can only deploy trunk commits", to, e.trunkLocal)
	}

	// expectedOld is the lease tag value we last observed (empty if we have never
	// seen it). The push is rejected if the remote has moved past it.
	expectedOld := e.repo.RefSha("refs/tags/" + name)

	toShort, _ := e.repo.Short(to)
	msg := fmt.Sprintf("deploy lease %s -> %s at %s", name, toShort, time.Now().Format(time.RFC3339))
	if err := e.repo.TagAnnotated(name, to, msg); err != nil {
		return err
	}

	if e.remote != "" {
		forcePush := e.cfg.TagPush == "force" || c.Args.Flag("force")
		if forcePush {
			if err := e.repo.PushTagForce(e.remote, name); err != nil {
				return fmt.Errorf("force-push lease %s: %w", name, err)
			}
		} else if err := e.repo.PushTagCAS(e.remote, name, expectedOld); err != nil {
			// Someone moved the tag since our fetch: show who holds it now.
			_ = e.repo.FetchTags(e.remote)
			if d, ok := e.repo.TagInfo(name); ok {
				fmt.Fprintln(e.errOut, e.badMark("lease "+name+" was taken by someone else"))
				fmt.Fprintf(e.errOut, "  now held by %s (%s) at %s\n", d.Tagger, d.Short, d.Date)
				fmt.Fprintln(e.errOut, e.colors.Dim("  re-run \"tbd lease take "+name+"\" to take it from the new position, or pass :force to override"))
			} else {
				fmt.Fprintln(e.errOut, e.badMark("lease push for "+name+" was rejected: "+err.Error()))
			}
			return cli.ExitError{Code: 1}
		}
	}

	d, _ := e.repo.TagInfo(name)
	fmt.Fprintln(e.out, e.okMark("took lease "+name+" -> "+toShort))
	if d.Tagger != "" {
		fmt.Fprintf(e.out, "  held by %s at %s\n", d.Tagger, d.Date)
	}
	if e.remote == "" {
		fmt.Fprintln(e.out, e.colors.Dim("  (local only — no remote configured, so no cross-machine mutual exclusion)"))
	}
	return nil
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
	for _, name := range e.cfg.LeaseTags {
		d, ok := e.repo.TagInfo(name)
		if !ok {
			fmt.Fprintf(e.out, "%-16s %s\n", name, e.colors.Dim("(unset)"))
			continue
		}
		onTrunk, _ := e.guard(false).OnTrunk(name)
		marker := e.okMark("on trunk")
		if !onTrunk {
			marker = e.colors.Yellow("⚠ off trunk")
		}
		holder := d.Tagger
		if holder == "" {
			holder = "?"
		}
		fmt.Fprintf(e.out, "%-16s %s  %s  held by %s @ %s\n", name, d.Short, marker, holder, d.Date)
	}
	return nil
}
