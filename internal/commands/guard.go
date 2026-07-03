package commands

import (
	"errors"
	"fmt"

	"goforge.dev/tbd/v2/internal/argv"
	"goforge.dev/tbd/v2/internal/cli"
	"goforge.dev/tbd/v2/internal/invariant"
)

func init() {
	spec := argv.Spec{Named: argv.Opts("ref"), Flags: argv.Opts("fetch")}
	cmd := &cli.Command{
		Name:    "guard",
		Summary: "Check the trunk-ancestor invariant for a ref (exit 0/1)",
		Usage: "tbd guard [ref:REF] [:fetch] [:local]\n\n" +
			"Reports whether trunk head is an ancestor of REF (default: current branch).\n" +
			"Exits 0 when the invariant holds, 1 otherwise - handy in CI.",
		Spec: spec,
		Run:  runGuard,
	}
	cli.Register(cmd)
	// "check" is an alias for "guard".
	cli.Register(&cli.Command{Name: "check", Summary: "Alias for guard", Usage: cmd.Usage, Spec: spec, Run: runGuard})
}

func runGuard(c *cli.Context) error {
	e, err := load(c)
	if err != nil {
		return err
	}

	target := c.Args.GetOr("ref", "")
	if target == "" {
		br, err := e.repo.CurrentBranch()
		if err != nil {
			return invariant.ErrDetached
		}
		target = br
	}

	if (e.fetch || c.Args.Flag("fetch")) && e.remote != "" {
		_ = e.step("fetching "+e.remote, func() error { return e.repo.Fetch(e.remote) })
	}

	g := e.guard(false)
	rep, err := g.Check(target)
	if err != nil {
		if errors.Is(err, invariant.ErrNoTrunk) {
			fmt.Fprintln(e.errOut, e.badMark("trunk "+e.trunkRef+" does not exist"))
			return cli.ExitError{Code: 1}
		}
		return err
	}

	fmt.Fprintf(e.out, "trunk:  %s @ %s\n", e.trunkRef, short(rep.TrunkHead))
	fmt.Fprintf(e.out, "target: %s @ %s\n", target, short(rep.TargetHead))
	fmt.Fprintf(e.out, "ahead %d, behind %d\n", rep.Ahead, rep.Behind)

	if rep.Diverged {
		fmt.Fprintln(e.out, e.badMark("diverged: trunk head is NOT an ancestor of "+target))
		fmt.Fprintln(e.out, e.colors.Dim("  run \"tbd feature sync\" to rebase onto trunk"))
		return cli.ExitError{Code: 1}
	}
	fmt.Fprintln(e.out, e.okMark("trunk head is an ancestor of "+target))
	return nil
}

func short(sha string) string {
	if len(sha) > 12 {
		return sha[:12]
	}
	return sha
}
