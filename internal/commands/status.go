package commands

import (
	"fmt"

	"goforge.dev/tbd/internal/argv"
	"goforge.dev/tbd/internal/cli"
)

func init() {
	cli.Register(&cli.Command{
		Name:    "status",
		Summary: "Show trunk, current branch, leases, and releases at a glance",
		Usage:   "tbd status [:fetch] [:local] [color-mode:none|always]",
		Spec:    argv.Spec{Flags: argv.Opts("fetch")},
		Run:     runStatus,
	})
}

func runStatus(c *cli.Context) error {
	e, err := load(c)
	if err != nil {
		return err
	}
	if (e.fetch || c.Args.Flag("fetch")) && e.remote != "" {
		_ = e.step("fetching "+e.remote, func() error {
			_ = e.repo.Fetch(e.remote)
			return e.repo.FetchTags(e.remote)
		})
	}

	col := e.colors
	fmt.Fprintln(e.out, col.Bold("trunk"))
	if !e.repo.Exists(e.trunkRef) {
		fmt.Fprintln(e.out, "  "+e.badMark("trunk "+e.trunkRef+" does not exist"))
		return nil
	}
	trunkHead, _ := e.repo.Short(e.trunkRef)
	fmt.Fprintf(e.out, "  %s @ %s\n", e.trunkRef, trunkHead)

	// Current branch vs trunk.
	fmt.Fprintln(e.out, col.Bold("current"))
	cur, cerr := e.repo.CurrentBranch()
	if cerr == nil && cur != "" {
		rep, err := e.guard(false).Check(cur)
		if err != nil {
			fmt.Fprintf(e.out, "  %s (%v)\n", cur, err)
		} else {
			head := short(rep.TargetHead)
			marker := e.okMark("on top of trunk")
			if rep.Diverged {
				marker = e.badMark("diverged - run \"tbd feature sync\"")
			}
			fmt.Fprintf(e.out, "  %s @ %s  +%d/-%d  %s\n", cur, head, rep.Ahead, rep.Behind, marker)
			if rep.Dirty {
				fmt.Fprintln(e.out, "  "+col.Yellow("⚠ working tree has uncommitted changes"))
			}
		}
	} else {
		fmt.Fprintln(e.out, "  "+col.Yellow("detached HEAD"))
	}

	// In-progress rebase / cherry-pick.
	switch {
	case e.repo.RebaseInProgress():
		fmt.Fprintln(e.out, "  "+col.Yellow("⚠ rebase in progress")+col.Dim(" - resolve, then \"tbd continue\" (or \"tbd continue :abort\")"))
	case e.repo.CherryPickInProgress():
		fmt.Fprintln(e.out, "  "+col.Yellow("⚠ cherry-pick in progress")+col.Dim(" - resolve, then \"tbd continue\" (or \"tbd continue :abort\")"))
	}

	// Feature branches.
	if features, _ := e.repo.ListBranches(e.cfg.FeaturePrefix + "*"); len(features) > 0 {
		fmt.Fprintln(e.out, col.Bold("features"))
		for _, b := range features {
			_, behind, _ := e.repo.AheadBehind(e.trunkRef, b)
			head, _ := e.repo.Short(b)
			marker := e.okMark("rebased")
			if behind > 0 {
				marker = e.badMark("behind")
			}
			fmt.Fprintf(e.out, "  %-26s %s  %s\n", b, head, marker)
		}
	}

	// Lease slots (strategy-aware; nothing to show when disabled).
	if e.cfg.LeaseStrategy != "none" {
		fmt.Fprintln(e.out, col.Bold("leases"))
		writeLeaseStatus(e, e.out, "  ")
	}

	// Release branches.
	if releases, _ := e.repo.ListBranches(e.cfg.ReleaseBranchPrefix + "*"); len(releases) > 0 {
		fmt.Fprintln(e.out, col.Bold("releases"))
		for _, b := range releases {
			head, _ := e.repo.Short(b)
			fmt.Fprintf(e.out, "  %-26s %s\n", b, head)
		}
	}
	return nil
}
