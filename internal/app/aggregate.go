package app

import (
	"fmt"

	"github.com/spf13/cobra"

	"goforge.dev/tbd/internal/git"
	"goforge.dev/tbd/internal/v2/gitops"
	v2state "goforge.dev/tbd/internal/v2/state"
)

func newCollabCommand(opts *rootOptions) *cobra.Command {
	return newAggregateCommand(opts, "collab")
}

func newStackCommand(opts *rootOptions) *cobra.Command {
	return newAggregateCommand(opts, "stack")
}

func newAggregateCommand(opts *rootOptions, kind string) *cobra.Command {
	var ids, descs []string
	cmd := &cobra.Command{
		Use:   kind,
		Short: fmt.Sprintf("start a %s aggregate workflow", kind),
		RunE: func(cmd *cobra.Command, args []string) error {
			items, err := bindIssues(ids, descs, true)
			if err != nil {
				return err
			}
			return startAggregate(cmd, opts, kind, items)
		},
	}
	cmd.Flags().StringArrayVar(&ids, "id", nil, "work item id; repeat with --desc")
	cmd.Flags().StringArrayVar(&descs, "desc", nil, "work item description; repeat with --id")
	cmd.AddCommand(newAggregateAddCommand(opts, kind), newStackRemoveCommand(opts, kind))
	if kind == "collab" {
		cmd.Commands()[1].Hidden = true
	}
	return cmd
}

func startAggregate(cmd *cobra.Command, opts *rootOptions, kind string, items []issueInput) error {
	e, err := loadEnv(cmd, opts)
	if err != nil {
		return err
	}
	if err := syncRemoteState(e); err != nil {
		return err
	}
	ids := make([]string, len(items))
	for i, it := range items {
		ids[i] = it.ID
	}
	branch := gitops.GroupBranch(e.Config, kind, ids)
	if err := e.CreateBranch(branch, e.TrunkRef); err != nil {
		return err
	}
	st, err := v2state.Load(e.Root)
	if err != nil {
		return err
	}
	group := v2state.Group{Name: branch, Kind: kind, Branch: branch, UpdatedAt: git.NowRFC3339()}
	for _, in := range items {
		key := v2state.ItemKey("feature", in.ID, in.Desc)
		st.Items[key] = v2state.Item{ID: in.ID, Kind: "feature", Desc: in.Desc, Branch: branch, Status: kind, TouchedAt: git.NowRFC3339()}
		group.ItemIDs = append(group.ItemIDs, key)
	}
	st.Groups[branch] = group
	if err := saveWorkflow(e, st, fmt.Sprintf("chore(tbd): start %s %s", kind, branch)); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "created %s %s\n", kind, branch)
	return nil
}

func newAggregateAddCommand(opts *rootOptions, kind string) *cobra.Command {
	var ids, descs []string
	cmd := &cobra.Command{
		Use:   "add",
		Short: fmt.Sprintf("add or touch items in the current %s", kind),
		RunE: func(cmd *cobra.Command, args []string) error {
			items, err := bindIssues(ids, descs, true)
			if err != nil {
				return err
			}
			e, err := loadEnv(cmd, opts)
			if err != nil {
				return err
			}
			branch, err := e.Repo.CurrentBranch()
			if err != nil {
				return err
			}
			st, err := v2state.Load(e.Root)
			if err != nil {
				return err
			}
			group, ok := st.Groups[branch]
			if !ok || group.Kind != kind {
				return fmt.Errorf("current branch %q is not a tbd %s branch", branch, kind)
			}
			for _, in := range items {
				key := v2state.ItemKey("feature", in.ID, in.Desc)
				st.Items[key] = v2state.Item{ID: in.ID, Kind: "feature", Desc: in.Desc, Branch: branch, Status: kind, TouchedAt: git.NowRFC3339()}
				group.ItemIDs = moveToEnd(group.ItemIDs, key)
			}
			group.UpdatedAt = git.NowRFC3339()
			st.Groups[branch] = group
			if err := saveWorkflow(e, st, fmt.Sprintf("chore(tbd): update %s %s", kind, branch)); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "updated %s %s\n", kind, branch)
			return nil
		},
	}
	cmd.Flags().StringArrayVar(&ids, "id", nil, "work item id; repeat with --desc")
	cmd.Flags().StringArrayVar(&descs, "desc", nil, "work item description; repeat with --id")
	return cmd
}

func newStackRemoveCommand(opts *rootOptions, kind string) *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:    "remove",
		Short:  "remove an item from a stack and rebuild from remaining commits",
		Hidden: kind != "stack",
		RunE: func(cmd *cobra.Command, args []string) error {
			if id == "" {
				return fmt.Errorf("--id is required")
			}
			e, err := loadEnv(cmd, opts)
			if err != nil {
				return err
			}
			branch, err := e.Repo.CurrentBranch()
			if err != nil {
				return err
			}
			st, err := v2state.Load(e.Root)
			if err != nil {
				return err
			}
			group, ok := st.Groups[branch]
			if !ok || group.Kind != "stack" {
				return fmt.Errorf("current branch %q is not a tbd stack branch", branch)
			}
			removeKey := ""
			for key, it := range st.Items {
				if it.ID == id && it.Branch == branch {
					removeKey = key
					delete(st.Items, key)
				}
			}
			if removeKey == "" {
				return fmt.Errorf("stack item %q not found", id)
			}
			group.ItemIDs = removeString(group.ItemIDs, removeKey)
			st.Groups[branch] = group
			if err := rebuildStack(e, group); err != nil {
				return err
			}
			if err := saveWorkflow(e, st, fmt.Sprintf("chore(tbd): remove %s from stack", id)); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "removed %s from %s\n", id, branch)
			return nil
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "work item id to remove")
	return cmd
}

func rebuildStack(e gitops.Env, group v2state.Group) error {
	if e.DryRun {
		fmt.Fprintf(e.Out, "dry-run: rebuild stack %s\n", group.Branch)
		return nil
	}
	st, err := v2state.Load(e.Root)
	if err != nil {
		return err
	}
	if err := e.Repo.Checkout(group.Branch); err != nil {
		return err
	}
	if err := e.Repo.ResetHard(e.TrunkRef); err != nil {
		return err
	}
	for _, key := range group.ItemIDs {
		it := st.Items[key]
		if it.Commit == "" {
			continue
		}
		if err := e.Repo.CherryPick(it.Commit); err != nil {
			return fmt.Errorf("rebuild stack cherry-pick %s: %w", key, err)
		}
	}
	return nil
}

func moveToEnd(xs []string, x string) []string {
	xs = removeString(xs, x)
	return append(xs, x)
}

func removeString(xs []string, x string) []string {
	out := xs[:0]
	for _, it := range xs {
		if it != x {
			out = append(out, it)
		}
	}
	return out
}
