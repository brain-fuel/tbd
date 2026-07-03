package app

import (
	"fmt"

	"github.com/spf13/cobra"

	"goforge.dev/tbd/internal/git"
	"goforge.dev/tbd/internal/v2/gitops"
	v2state "goforge.dev/tbd/internal/v2/state"
)

func newFeatureCommand(opts *rootOptions) *cobra.Command {
	return newWorkItemCommand(opts, "feature", true)
}

func newFixCommand(opts *rootOptions) *cobra.Command {
	return newWorkItemCommand(opts, "fix", false)
}

func newWorkItemCommand(opts *rootOptions, kind string, requireID bool) *cobra.Command {
	var id, desc string
	cmd := &cobra.Command{
		Use:   kind,
		Short: fmt.Sprintf("start a %s branch and record workflow metadata", kind),
		RunE: func(cmd *cobra.Command, args []string) error {
			if requireID && id == "" {
				return fmt.Errorf("--id is required for %s", kind)
			}
			if desc == "" {
				return fmt.Errorf("--desc is required")
			}
			e, err := loadEnv(cmd, opts)
			if err != nil {
				return err
			}
			if err := syncRemoteState(e); err != nil {
				return err
			}
			branch := gitops.BranchName(e.Config, kind, id, desc)
			if err := e.CreateBranch(branch, e.TrunkRef); err != nil {
				return err
			}
			st, err := v2state.Load(e.Root)
			if err != nil {
				return err
			}
			key := v2state.ItemKey(kind, id, desc)
			st.Items[key] = v2state.Item{
				ID:        id,
				Kind:      kind,
				Desc:      desc,
				Branch:    branch,
				Status:    "started",
				TouchedAt: git.NowRFC3339(),
			}
			if err := saveWorkflow(e, st, fmt.Sprintf("chore(tbd): start %s %s", kind, empty(id, desc))); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "created %s %s\n", kind, branch)
			return nil
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "work item id, for example JIRA-123")
	cmd.Flags().StringVar(&desc, "desc", "", "human description")
	return cmd
}

func empty(v, fallback string) string {
	if v != "" {
		return v
	}
	return fallback
}
