package app

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"goforge.dev/tbd/v2/internal/git"
	"goforge.dev/tbd/v2/internal/v2/gitops"
	"goforge.dev/tbd/v2/internal/v2/hooks"
	v2state "goforge.dev/tbd/v2/internal/v2/state"
)

type issueInput struct {
	ID   string
	Desc string
}

func loadEnv(cmd *cobra.Command, opts *rootOptions) (gitops.Env, error) {
	dir, _ := os.Getwd()
	return gitops.Load(dir, cmd.OutOrStdout(), cmd.ErrOrStderr(), opts.dryRun)
}

func hookRunner(e gitops.Env) hooks.Runner {
	return hooks.Runner{Root: e.Root, Config: e.Config, Stdout: e.Out, Stderr: e.Err, DryRun: e.DryRun}
}

func syncRemoteState(e gitops.Env) error {
	if err := e.UpdateLocalTrunk(); err != nil {
		return err
	}
	return e.InvalidateStaleUAT()
}

func bindIssues(ids, descs []string, requireID bool) ([]issueInput, error) {
	if len(descs) == 0 {
		return nil, fmt.Errorf("at least one --desc is required")
	}
	if len(ids) != 0 && len(ids) != len(descs) {
		return nil, fmt.Errorf("--id and --desc must be repeated in matching order")
	}
	if requireID && len(ids) != len(descs) {
		return nil, fmt.Errorf("--id is required for every feature")
	}
	out := make([]issueInput, len(descs))
	for i := range descs {
		id := ""
		if len(ids) > i {
			id = strings.TrimSpace(ids[i])
		}
		if requireID && id == "" {
			return nil, fmt.Errorf("--id is required for feature %d", i+1)
		}
		desc := strings.TrimSpace(descs[i])
		if desc == "" {
			return nil, fmt.Errorf("--desc must not be empty")
		}
		out[i] = issueInput{ID: id, Desc: desc}
	}
	return out, nil
}

func saveWorkflow(e gitops.Env, st v2state.State, message string) error {
	if err := v2state.Save(e.Root, st); err != nil {
		return err
	}
	return e.CommitWorkflow(message)
}

func saveReleaseWorkflow(e gitops.Env, book v2state.ReleaseBook, message string) error {
	if err := v2state.SaveRelease(e.Root, book); err != nil {
		return err
	}
	return e.CommitWorkflow(message)
}

func seedMessage(kind, id, desc string) string {
	typ := "feat"
	if kind == "fix" {
		typ = "fix"
	}
	scope := ""
	if id != "" {
		scope = "(" + id + ")"
	}
	return fmt.Sprintf("%s%s: %s", typ, scope, desc)
}

func currentItemSeed(e gitops.Env, branch string) string {
	st, err := v2state.Load(e.Root)
	if err != nil {
		return ""
	}
	for _, it := range st.Items {
		if it.Branch == branch {
			return seedMessage(it.Kind, it.ID, it.Desc)
		}
	}
	for _, g := range st.Groups {
		if g.Branch == branch {
			return fmt.Sprintf("feat: %s", strings.TrimPrefix(branch, "feature/"))
		}
	}
	return ""
}

func updateCurrentCommit(e gitops.Env, branch string) error {
	st, err := v2state.Load(e.Root)
	if err != nil {
		return err
	}
	head, _ := e.Repo.RevParse(branch)
	changed := false
	for k, it := range st.Items {
		if it.Branch == branch {
			it.Commit = head
			it.Status = "committed"
			it.TouchedAt = git.NowRFC3339()
			st.Items[k] = it
			changed = true
		}
	}
	for k, g := range st.Groups {
		if g.Branch == branch {
			g.UpdatedAt = git.NowRFC3339()
			st.Groups[k] = g
			changed = true
		}
	}
	if !changed {
		return nil
	}
	if err := v2state.Save(e.Root, st); err != nil {
		return err
	}
	if e.DryRun {
		return nil
	}
	if err := e.Repo.StageAll(); err != nil {
		return err
	}
	if e.Repo.HasStaged() {
		return e.Repo.CommitAmend("")
	}
	return nil
}

func prompt(defaultValue, label string) string {
	if !stdinIsTerminal() {
		return defaultValue
	}
	fmt.Fprintf(os.Stdout, "%s (%s): ", label, defaultValue)
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	line = strings.TrimSpace(line)
	if line == "" {
		return defaultValue
	}
	return line
}

func stdinIsTerminal() bool {
	fi, err := os.Stdin.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}
