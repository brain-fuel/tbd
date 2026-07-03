package app

import (
	"fmt"

	"github.com/spf13/cobra"

	"goforge.dev/tbd/v2/internal/git"
	"goforge.dev/tbd/v2/internal/v2/gitops"
	v2state "goforge.dev/tbd/v2/internal/v2/state"
)

func newReleaseCommand(opts *rootOptions) *cobra.Command {
	cmd := &cobra.Command{Use: "release", Short: "manage RCs and production releases"}
	cmd.AddCommand(releaseRCCommand(opts), releasePrepareCommand(opts), releaseCompleteCommand(opts))
	return cmd
}

func releaseRCCommand(opts *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "rc SEMVER",
		Short: "mark the current rebased candidate as UAT-good and move rc-<semver>",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			semver := args[0]
			e, err := loadEnv(cmd, opts)
			if err != nil {
				return err
			}
			if err := e.EnsureNotProtectedBranch(); err != nil {
				return err
			}
			if err := syncRemoteState(e); err != nil {
				return err
			}
			if err := e.EnsureOnTrunk("HEAD"); err != nil {
				return err
			}
			hr := hookRunner(e)
			if err := hr.Pre("pre-release"); err != nil {
				return err
			}
			st, err := v2state.Load(e.Root)
			if err != nil {
				return err
			}
			book, err := v2state.LoadRelease(e.Root)
			if err != nil {
				return err
			}
			head, _ := e.Repo.RevParse("HEAD")
			items := releaseItemsForHead(e, st)
			st.UAT[semver] = v2state.UATState{Semver: semver, CandidateRef: gitops.RCTag(e.Config, semver), Commit: head, Valid: true, UpdatedAt: git.NowRFC3339()}
			v2state.UpsertDraft(&book, v2state.ReleaseDraft{Semver: semver, Status: "rc", Items: items})
			v2state.AppendEvent(&book, v2state.ReleaseEvent{Type: "rc", Semver: semver, Ref: gitops.RCTag(e.Config, semver), Commit: head, Items: items})
			if err := v2state.Save(e.Root, st); err != nil {
				return err
			}
			if err := saveReleaseWorkflow(e, book, "chore(tbd): prepare rc "+semver); err != nil {
				return err
			}
			head, _ = e.Repo.RevParse("HEAD")
			rc := gitops.RCTag(e.Config, semver)
			if err := replaceTag(e, rc, head, "release candidate "+semver, true); err != nil {
				return err
			}
			hr.Post("post-release")
			fmt.Fprintf(cmd.OutOrStdout(), "%s -> %s\n", rc, head[:12])
			return nil
		},
	}
}

func releasePrepareCommand(opts *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "prepare SEMVER",
		Short: "create immutable release/<semver> branch in branch release mode",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			semver := args[0]
			e, err := loadEnv(cmd, opts)
			if err != nil {
				return err
			}
			if e.Config.Release.Strategy != "branch" {
				return fmt.Errorf("release prepare is only used when release.strategy is branch")
			}
			if err := syncRemoteState(e); err != nil {
				return err
			}
			if err := e.EnsureOnTrunk("HEAD"); err != nil {
				return err
			}
			branch := e.Config.Release.BranchPrefix + semver
			if e.Repo.Exists(branch) {
				return fmt.Errorf("release branch %q already exists and is immutable", branch)
			}
			if e.DryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "dry-run: create %s at HEAD\n", branch)
				return nil
			}
			if err := e.Repo.BranchCreate(branch, "HEAD"); err != nil {
				return err
			}
			if e.RemoteOK {
				if err := e.Repo.PushBranch(e.Config.Remote, branch); err != nil {
					return err
				}
			}
			fmt.Fprintf(cmd.OutOrStdout(), "created %s\n", branch)
			return nil
		},
	}
}

func releaseCompleteCommand(opts *rootOptions) *cobra.Command {
	var from string
	cmd := &cobra.Command{
		Use:   "complete SEMVER",
		Short: "mark a successful production deploy, tag v<semver>, and fast-forward trunk",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			semver := args[0]
			e, err := loadEnv(cmd, opts)
			if err != nil {
				return err
			}
			ref := from
			if ref == "" {
				if e.Config.Release.Strategy == "branch" {
					ref = e.Config.Release.BranchPrefix + semver
				} else {
					ref = "HEAD"
				}
			}
			if !e.Repo.Exists(ref) {
				return fmt.Errorf("release source %q does not exist", ref)
			}
			if err := syncRemoteState(e); err != nil {
				return err
			}
			if err := e.EnsureOnTrunk(ref); err != nil {
				return err
			}
			book, err := v2state.LoadRelease(e.Root)
			if err != nil {
				return err
			}
			head, _ := e.Repo.RevParse(ref)
			v2state.AppendEvent(&book, v2state.ReleaseEvent{Type: "release", Semver: semver, Ref: gitops.ReleaseTag(e.Config, semver), Commit: head})
			if err := saveReleaseWorkflow(e, book, "chore(tbd): release "+semver); err != nil {
				return err
			}
			ref = "HEAD"
			if e.Config.Release.Strategy == "branch" && from == "" {
				ref = e.Config.Release.BranchPrefix + semver
			}
			releaseCommit, err := e.Repo.RevParse(ref)
			if err != nil {
				return err
			}
			if err := replaceTag(e, gitops.ReleaseTag(e.Config, semver), releaseCommit, "release "+semver, false); err != nil {
				return err
			}
			if err := fastForwardTrunk(e, releaseCommit); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "released %s from %s\n", semver, releaseCommit[:12])
			return nil
		},
	}
	cmd.Flags().StringVar(&from, "from", "", "release source ref, defaults to release/<semver> in branch mode or HEAD in tag mode")
	return cmd
}

func replaceTag(e gitops.Env, tag, ref, msg string, deleteOld bool) error {
	if e.DryRun {
		fmt.Fprintf(e.Out, "dry-run: tag %s at %s\n", tag, ref)
		return nil
	}
	if deleteOld && e.Repo.Exists("refs/tags/"+tag) {
		_ = e.Repo.TagDelete(tag)
		if e.RemoteOK {
			_ = e.Repo.PushDeleteTag(e.Config.Remote, tag)
		}
	}
	expected := ""
	if e.RemoteOK {
		expected = e.Repo.RemoteTagSha(e.Config.Remote, tag)
	}
	if err := e.Repo.TagAnnotated(tag, ref, msg); err != nil {
		return err
	}
	if e.RemoteOK {
		if e.Config.Push.Tag == "force" || deleteOld {
			return e.Repo.PushTagForce(e.Config.Remote, tag)
		}
		return e.Repo.PushTagCAS(e.Config.Remote, tag, expected)
	}
	return nil
}

func fastForwardTrunk(e gitops.Env, ref string) error {
	if e.DryRun {
		fmt.Fprintf(e.Out, "dry-run: fast-forward %s to %s\n", e.Config.TrunkName, ref)
		return nil
	}
	cur, _ := e.Repo.CurrentBranch()
	if err := e.Repo.Checkout(e.Config.TrunkName); err != nil {
		return err
	}
	if err := e.Repo.FFMerge(ref); err != nil {
		_ = e.Repo.Checkout(cur)
		return fmt.Errorf("configured trunk %s cannot fast-forward to %s: %w", e.Config.TrunkName, ref, err)
	}
	if e.RemoteOK {
		if err := e.Repo.PushBranch(e.Config.Remote, e.Config.TrunkName); err != nil {
			return err
		}
	}
	if cur != "" && cur != e.Config.TrunkName {
		_ = e.Repo.Checkout(cur)
	}
	return nil
}

func releaseItemsForHead(e gitops.Env, st v2state.State) []v2state.ReleaseItem {
	cur, _ := e.Repo.CurrentBranch()
	var out []v2state.ReleaseItem
	for _, it := range st.Items {
		if it.Branch == cur {
			out = append(out, v2state.ReleaseItem{ID: it.ID, Kind: it.Kind, Desc: it.Desc, Commit: it.Commit})
		}
	}
	return out
}
