package app

import (
	"fmt"

	"github.com/spf13/cobra"

	"goforge.dev/tbd/internal/v2/gitops"
)

func newLeaseCommand(opts *rootOptions) *cobra.Command {
	var force bool
	var to string
	cmd := &cobra.Command{
		Use:     "lease DEPLOY_REF",
		Aliases: []string{"deploy"},
		Short:   "move a configured deploy ref using tag or branch lease semantics",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			e, err := loadEnv(cmd, opts)
			if err != nil {
				return err
			}
			if err := e.EnsureNotProtectedBranch(); err != nil {
				return err
			}
			ref := args[0]
			if !configuredDeploy(e.Config.Deploy.Refs, ref) {
				return fmt.Errorf("%q is not configured as a deploy ref", ref)
			}
			target := "HEAD"
			if to != "" {
				target = to
			}
			if err := syncRemoteState(e); err != nil {
				return err
			}
			if err := e.EnsureOnTrunk(target); err != nil && !force {
				return err
			}
			hr := hookRunner(e)
			if err := hr.Pre("pre-lease"); err != nil {
				return err
			}
			if err := hr.DeployPre(ref); err != nil {
				return err
			}
			if e.Config.Deploy.Strategy == "branch" {
				if err := leaseBranch(e, ref, target, force); err != nil {
					return err
				}
			} else if err := leaseTag(e, ref, target, force); err != nil {
				return err
			}
			hr.DeployPost(ref)
			hr.Post("post-lease")
			fmt.Fprintf(cmd.OutOrStdout(), "%s -> %s\n", ref, target)
			return nil
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "override trunk guard or force push mode")
	cmd.Flags().StringVar(&to, "to", "", "target ref, defaults to HEAD")
	return cmd
}

func configuredDeploy(refs []string, ref string) bool {
	for _, r := range refs {
		if r == ref {
			return true
		}
	}
	return false
}

func leaseTag(e gitops.Env, ref, target string, force bool) error {
	if e.DryRun {
		fmt.Fprintf(e.Out, "dry-run: move deploy tag %s to %s\n", ref, target)
		return nil
	}
	expected := ""
	if e.RemoteOK {
		expected = e.Repo.RemoteTagSha(e.Config.Remote, ref)
	}
	if err := e.Repo.TagAnnotated(ref, target, "deploy "+ref+" -> "+target); err != nil {
		return err
	}
	if e.RemoteOK {
		if force || e.Config.Push.Tag == "force" {
			return e.Repo.PushTagForce(e.Config.Remote, ref)
		}
		return e.Repo.PushTagCAS(e.Config.Remote, ref, expected)
	}
	return nil
}

func leaseBranch(e gitops.Env, ref, target string, force bool) error {
	if e.DryRun {
		fmt.Fprintf(e.Out, "dry-run: move deploy branch %s to %s\n", ref, target)
		return nil
	}
	expected := ""
	if e.RemoteOK {
		expected = e.Repo.RemoteBranchSha(e.Config.Remote, ref)
	}
	if err := e.Repo.BranchCreateForce(ref, target); err != nil {
		return err
	}
	if e.RemoteOK {
		if force || e.Config.Push.Branch == "force" {
			return e.Repo.PushBranchForce(e.Config.Remote, ref)
		}
		return e.Repo.PushBranchCAS(e.Config.Remote, ref, expected)
	}
	return nil
}
