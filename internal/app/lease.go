package app

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"goforge.dev/tbd/v2/internal/v2/gitops"
)

func newLeaseCommand(opts *rootOptions) *cobra.Command {
	var to string
	cmd := &cobra.Command{
		Use:     "lease DEPLOY_REF",
		Aliases: []string{"deploy"},
		Short:   "borrow an unheld deploy mutex or advance your own",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			target := "HEAD"
			if to != "" {
				target = to
			}
			return runDeployMutex(cmd, opts, deployMutexRequest{Verb: "lease", Ref: args[0], Target: target})
		},
	}
	cmd.Flags().StringVar(&to, "to", "", "target ref, defaults to HEAD")
	return cmd
}

func newStealCommand(opts *rootOptions) *cobra.Command {
	var to string
	cmd := &cobra.Command{
		Use:   "steal DEPLOY_REF",
		Short: "explicitly take a deploy mutex from its current holder",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			target := "HEAD"
			if to != "" {
				target = to
			}
			return runDeployMutex(cmd, opts, deployMutexRequest{Verb: "steal", Ref: args[0], Target: target, Steal: true})
		},
	}
	cmd.Flags().StringVar(&to, "to", "", "target ref, defaults to HEAD")
	return cmd
}

func newRelinquishCommand(opts *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "relinquish DEPLOY_REF",
		Short: "release your deploy mutex back to trunk head",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDeployMutex(cmd, opts, deployMutexRequest{Verb: "relinquish", Ref: args[0], Relinquish: true})
		},
	}
}

type deployMutexRequest struct {
	Verb       string
	Ref        string
	Target     string
	Steal      bool
	Relinquish bool
}

type deployMutexState struct {
	Exists bool
	SHA    string
	Held   bool
	Mine   bool
	Holder string
}

func runDeployMutex(cmd *cobra.Command, opts *rootOptions, req deployMutexRequest) error {
	e, err := loadEnv(cmd, opts)
	if err != nil {
		return err
	}
	if err := e.EnsureNotProtectedBranch(); err != nil {
		return err
	}
	ref := req.Ref
	if !configuredDeploy(e.Config.Deploy.Refs, ref) {
		return fmt.Errorf("%q is not configured as a deploy ref", ref)
	}
	if err := syncRemoteState(e); err != nil {
		return err
	}
	trunkHead, err := e.Repo.RevParse(e.TrunkRef)
	if err != nil {
		return err
	}
	target := req.Target
	if req.Relinquish {
		target = trunkHead
	} else if target == "" {
		target = "HEAD"
	}
	if err := e.EnsureOnTrunk(target); err != nil {
		return err
	}
	state := classifyDeployMutex(e, ref, trunkHead)
	if req.Relinquish {
		if !state.Exists || !state.Held {
			fmt.Fprintf(cmd.OutOrStdout(), "%s already relinquished at %s\n", ref, e.TrunkRef)
			return nil
		}
		if !state.Mine {
			return fmt.Errorf("%s is held by %s; use tbd steal %s before moving it", ref, state.Holder, ref)
		}
	} else if state.Held && !state.Mine && !req.Steal {
		return fmt.Errorf("%s is held by %s; use tbd steal %s to take it explicitly", ref, state.Holder, ref)
	}

	hr := hookRunner(e)
	if err := hr.Pre("pre-lease"); err != nil {
		return err
	}
	if err := hr.DeployPre(ref); err != nil {
		return err
	}
	if e.Config.Deploy.Strategy == "branch" {
		if err := moveDeployBranch(e, ref, target, req.Steal); err != nil {
			return err
		}
	} else if err := moveDeployTag(e, ref, target, req.Steal); err != nil {
		return err
	}
	hr.DeployPost(ref)
	hr.Post("post-lease")
	if req.Relinquish {
		fmt.Fprintf(cmd.OutOrStdout(), "%s relinquished to %s\n", ref, e.Config.TrunkName)
	} else {
		fmt.Fprintf(cmd.OutOrStdout(), "%s %s -> %s\n", req.Verb, ref, target)
	}
	return nil
}

func configuredDeploy(refs []string, ref string) bool {
	for _, r := range refs {
		if r == ref {
			return true
		}
	}
	return false
}

func classifyDeployMutex(e gitops.Env, ref, trunkHead string) deployMutexState {
	sha, exists := deployRefCommit(e, ref)
	if !exists {
		return deployMutexState{}
	}
	state := deployMutexState{Exists: true, SHA: sha}
	if sha == trunkHead || e.Repo.IsAncestor(sha, trunkHead) {
		return state
	}
	state.Held = true
	head, err := e.Repo.RevParse("HEAD")
	if err == nil && (sha == head || e.Repo.IsAncestor(sha, head)) {
		state.Mine = true
		state.Holder = "this branch"
		return state
	}
	holders := e.Repo.BranchesContaining(sha)
	if len(holders) == 0 {
		state.Holder = sha[:shortLen(sha)]
		return state
	}
	state.Holder = strings.Join(holders, ", ")
	return state
}

func deployRefCommit(e gitops.Env, ref string) (string, bool) {
	names := []string{ref}
	if e.Config.Deploy.Strategy == "branch" && e.RemoteOK {
		names = append(names, e.Config.Remote+"/"+ref)
	}
	for _, name := range names {
		if sha := e.Repo.CommitOf(name); sha != "" {
			return sha, true
		}
	}
	return "", false
}

func shortLen(s string) int {
	if len(s) < 12 {
		return len(s)
	}
	return 12
}

func moveDeployTag(e gitops.Env, ref, target string, steal bool) error {
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
		if e.Config.Push.Tag == "force" {
			return e.Repo.PushTagForce(e.Config.Remote, ref)
		}
		return e.Repo.PushTagCAS(e.Config.Remote, ref, expected)
	}
	return nil
}

func moveDeployBranch(e gitops.Env, ref, target string, steal bool) error {
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
		if e.Config.Push.Branch == "force" {
			return e.Repo.PushBranchForce(e.Config.Remote, ref)
		}
		return e.Repo.PushBranchCAS(e.Config.Remote, ref, expected)
	}
	return nil
}
