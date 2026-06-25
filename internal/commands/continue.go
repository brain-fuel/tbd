package commands

import (
	"fmt"

	"goforge.dev/tbd/internal/argv"
	"goforge.dev/tbd/internal/cli"
)

func init() {
	cli.Register(&cli.Command{
		Name:    "continue",
		Summary: "Resume a tbd rebase or cherry-put after resolving conflicts (or :abort it)",
		Usage: "tbd continue            resume once conflicts are staged\n" +
			"tbd continue :abort     abandon the operation and restore the branch\n\n" +
			"When tbd commit, rebase, feature sync/finish/push, or cherry-put hits a\n" +
			"conflict it leaves the rebase or cherry-pick in progress. Fix the files,\n" +
			"\"git add\" them, then run this. It resumes without opening an editor.",
		Spec: argv.Spec{Flags: argv.Opts("abort")},
		Run:  runContinue,
	})
}

func runContinue(c *cli.Context) error {
	e, err := load(c)
	if err != nil {
		return err
	}
	rebasing := e.repo.RebaseInProgress()
	cherryPicking := e.repo.CherryPickInProgress()
	if !rebasing && !cherryPicking {
		return fmt.Errorf("no rebase or cherry-pick in progress")
	}

	kind := "rebase"
	if cherryPicking {
		kind = "cherry-pick"
	}

	if c.Args.Flag("abort") {
		var aerr error
		if rebasing {
			aerr = e.repo.RebaseAbort()
		} else {
			aerr = e.repo.CherryPickAbort()
		}
		if aerr != nil {
			return aerr
		}
		fmt.Fprintln(e.out, e.colors.Yellow(kind+" aborted; the branch is restored to before it"))
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

	cont := e.repo.RebaseContinue
	if cherryPicking {
		cont = e.repo.CherryPickContinue
	}
	if err := e.step("continuing the "+kind, cont); err != nil {
		if e.repo.RebaseInProgress() || e.repo.CherryPickInProgress() {
			fmt.Fprintln(e.errOut, e.badMark("more conflicts to resolve"))
			fmt.Fprintln(e.errOut, e.colors.Dim("  resolve them, \"git add\", then \"tbd continue\" again, or \"tbd continue :abort\""))
			return cli.ExitError{Code: 1}
		}
		return err
	}

	br, err := e.repo.CurrentBranch()
	if err != nil {
		fmt.Fprintln(e.out, e.okMark(kind+" complete"))
		return nil
	}
	head, _ := e.repo.Short(br)
	fmt.Fprintln(e.out, e.okMark(kind+" complete; "+br+" @ "+head))
	return nil
}
