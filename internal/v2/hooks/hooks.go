// Package hooks runs configured workflow hooks from the repository root.
package hooks

import (
	"context"
	"fmt"
	"io"
	"time"

	"goforge.dev/goplus/std/process"
	v2config "goforge.dev/tbd/v2/internal/v2/config"
)

type Runner struct {
	Root   string
	Config v2config.Config
	Stdout io.Writer
	Stderr io.Writer
	DryRun bool
}

func (r Runner) Pre(name string) error {
	return r.run(name, false)
}

func (r Runner) Post(name string) {
	if err := r.run(name, true); err != nil && r.Stderr != nil {
		fmt.Fprintf(r.Stderr, "warning: post-hook %s failed: %v\n", name, err)
	}
}

func (r Runner) DeployPre(ref string) error {
	for _, step := range r.Config.Hooks.DeployRefs[ref].PrePush {
		if err := r.runStep("deploy."+ref+".pre-push", step, false); err != nil {
			return err
		}
	}
	return nil
}

func (r Runner) DeployPost(ref string) {
	for _, step := range r.Config.Hooks.DeployRefs[ref].PostPush {
		if err := r.runStep("deploy."+ref+".post-push", step, true); err != nil && r.Stderr != nil {
			fmt.Fprintf(r.Stderr, "warning: post-hook deploy.%s.post-push failed: %v\n", ref, err)
		}
	}
}

func (r Runner) run(name string, warnOnly bool) error {
	for _, step := range r.steps(name) {
		if err := r.runStep(name, step, warnOnly); err != nil {
			return err
		}
	}
	return nil
}

func (r Runner) steps(name string) []v2config.HookStep {
	switch name {
	case "pre-commit":
		return r.Config.Hooks.PreCommit
	case "post-commit":
		return r.Config.Hooks.PostCommit
	case "pre-push":
		return r.Config.Hooks.PrePush
	case "post-push":
		return r.Config.Hooks.PostPush
	case "pre-lease":
		return r.Config.Hooks.PreLease
	case "post-lease":
		return r.Config.Hooks.PostLease
	case "pre-release":
		return r.Config.Hooks.PreRelease
	case "post-release":
		return r.Config.Hooks.PostRelease
	default:
		return nil
	}
}

func (r Runner) runStep(hookName string, step v2config.HookStep, warnOnly bool) error {
	command := step.Command
	timeout := step.Timeout
	optional := step.Optional
	if step.Name != "" {
		task, ok := r.Config.Tasks[step.Name]
		if !ok {
			return fmt.Errorf("%s references unknown task %q", hookName, step.Name)
		}
		command = task.Command
		if timeout == "" {
			timeout = task.Timeout
		}
		optional = optional || task.Optional
	}
	if command == "" {
		return nil
	}
	if r.DryRun {
		if r.Stdout != nil {
			fmt.Fprintf(r.Stdout, "dry-run: hook %s: %s\n", hookName, command)
		}
		return nil
	}
	ctx := context.Background()
	cancel := func() {}
	if timeout != "" {
		if d, err := time.ParseDuration(timeout); err == nil {
			ctx, cancel = context.WithTimeout(ctx, d)
		}
	}
	defer cancel()
	_, err := process.Run(ctx, process.Spec{Path: "sh", Args: []string{"-c", command}, Dir: r.Root, Stdout: r.Stdout, Stderr: r.Stderr})
	if err != nil {
		if optional || warnOnly {
			if r.Stderr != nil {
				fmt.Fprintf(r.Stderr, "warning: optional hook %s failed: %v\n", hookName, err)
			}
			return nil
		}
		return err
	}
	return nil
}
