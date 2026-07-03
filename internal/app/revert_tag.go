package app

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"goforge.dev/tbd/internal/v2/gitops"
	v2state "goforge.dev/tbd/internal/v2/state"
)

func newRevertCommand(opts *rootOptions) *cobra.Command {
	var ref, allPast, explanation string
	var major, minor bool
	cmd := &cobra.Command{
		Use:   "revert",
		Short: "revert commits, tags, items, semver entries, or everything after a known-good point",
		RunE: func(cmd *cobra.Command, args []string) error {
			e, err := loadEnv(cmd, opts)
			if err != nil {
				return err
			}
			if err := e.EnsureNotProtectedBranch(); err != nil {
				return err
			}
			if explanation == "" {
				explanation = "removed by tbd revert"
			}
			var refs []string
			switch {
			case allPast != "":
				base := resolveReleaseRef(e, allPast)
				refs, err = e.Repo.RevList(base + "..HEAD")
				if err != nil {
					return err
				}
			case ref != "":
				refs = []string{resolveReleaseRef(e, ref)}
			default:
				return fmt.Errorf("pass --ref or --all-past")
			}
			if len(refs) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "nothing to revert")
				return nil
			}
			if e.DryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "dry-run: revert %s\n", strings.Join(refs, ", "))
				return nil
			}
			for _, r := range refs {
				if err := e.Repo.RevertNoEdit(r); err != nil {
					return err
				}
			}
			bump := "patch"
			if major {
				bump = "major"
			} else if minor {
				bump = "minor"
			}
			book, err := v2state.LoadRelease(e.Root)
			if err != nil {
				return err
			}
			v2state.AppendEvent(&book, v2state.ReleaseEvent{Type: "revert", Ref: empty(ref, allPast), Explanation: bump + ": " + explanation})
			return saveReleaseWorkflow(e, book, "chore(tbd): record revert")
		},
	}
	cmd.Flags().StringVar(&ref, "ref", "", "commit sha, tag, queue/stack item ref, or semver")
	cmd.Flags().StringVar(&allPast, "all-past", "", "revert every commit after version/tag/ref")
	cmd.Flags().StringVar(&explanation, "why", "", "explanation for release notes")
	cmd.Flags().BoolVar(&major, "major", false, "record a major semver bump")
	cmd.Flags().BoolVar(&minor, "minor", false, "record a minor semver bump")
	return cmd
}

func newTagCommand(opts *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "tag REF bad",
		Short: "tag a bad commit as bad-<timestamp>",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if args[1] != "bad" {
				return fmt.Errorf("only bad tagging is supported")
			}
			e, err := loadEnv(cmd, opts)
			if err != nil {
				return err
			}
			tag := gitops.BadTag(e.Config)
			if err := replaceTag(e, tag, args[0], "bad release marker", false); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s -> %s\n", tag, args[0])
			return nil
		},
	}
}

func resolveReleaseRef(e gitops.Env, v string) string {
	if e.Repo.Exists(v) {
		return v
	}
	tag := gitops.ReleaseTag(e.Config, v)
	if e.Repo.Exists(tag) {
		return tag
	}
	branch := e.Config.Release.BranchPrefix + v
	if e.Repo.Exists(branch) {
		return branch
	}
	return v
}
