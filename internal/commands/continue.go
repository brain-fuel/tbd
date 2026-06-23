package commands

import (
	"fmt"

	"goforge.dev/tbd/internal/cli"
)

func init() {
	cli.Register(&cli.Command{
		Name:    "continue",
		Summary: "Resume a tbd rebase after resolving conflicts (or :abort it)",
		Usage: "tbd continue            resume the rebase once conflicts are staged\n" +
			"tbd continue :abort     abandon the rebase and restore the branch\n\n" +
			"When tbd commit, feature sync, finish, or push hits a conflict it leaves\n" +
			"the rebase in progress. Fix the files, \"git add\" them, then run this. It\n" +
			"resumes without opening an editor, keeping the existing commit message.",
		Run: runContinue,
	})
}

func runContinue(c *cli.Context) error {
	e, err := load(c)
	if err != nil {
		return err
	}
	if !e.repo.RebaseInProgress() {
		return fmt.Errorf("no rebase in progress")
	}

	if c.Args.Flag("abort") {
		if err := e.repo.RebaseAbort(); err != nil {
			return err
		}
		fmt.Fprintln(e.out, e.colors.Yellow("rebase aborted; the branch is restored to before the rebase"))
		return nil
	}

	// Refuse while conflicts are still unresolved, so we never "continue" over
	// files that still contain conflict markers.
	if unmerged, _ := e.repo.UnmergedPaths(); len(unmerged) > 0 {
		fmt.Fprintln(e.errOut, e.badMark("unresolved conflicts remain:"))
		for _, f := range unmerged {
			fmt.Fprintln(e.errOut, "  "+f)
		}
		fmt.Fprintln(e.errOut, e.colors.Dim("  edit them, run \"git add <file>\", then \"tbd continue\""))
		return cli.ExitError{Code: 1}
	}

	if err := e.repo.RebaseContinue(); err != nil {
		if e.repo.RebaseInProgress() {
			fmt.Fprintln(e.errOut, e.badMark("more conflicts to resolve"))
			fmt.Fprintln(e.errOut, e.colors.Dim("  resolve them, \"git add\", then \"tbd continue\" again, or \"tbd continue :abort\""))
			return cli.ExitError{Code: 1}
		}
		return err
	}

	br, err := e.repo.CurrentBranch()
	if err != nil {
		// HEAD reattached but not to a branch; still report success.
		fmt.Fprintln(e.out, e.okMark("rebase complete"))
		return nil
	}
	head, _ := e.repo.Short(br)
	fmt.Fprintln(e.out, e.okMark("rebase complete; "+br+" @ "+head))
	return nil
}
