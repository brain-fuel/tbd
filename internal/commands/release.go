package commands

import (
	"fmt"
	"strings"

	"goforge.dev/tbd/v2/internal/argv"
	"goforge.dev/tbd/v2/internal/cli"
	"goforge.dev/tbd/v2/internal/config"
)

func init() {
	cli.Register(&cli.Command{
		Name:    "release",
		Summary: "Cut releases from trunk (branch and/or tag)",
		Usage: "tbd release cut VERSION [from:REF] [strategy:branch|tag|branch,tag] [:no-push]\n" +
			"tbd release list\n\n" +
			"A release may only be cut from a commit that is on trunk.",
		Spec: argv.Spec{
			Named:       argv.Opts("from", "strategy"),
			Flags:       argv.Opts("no-push"),
			Positionals: []string{"subcommand", "version"},
		},
		Run: runRelease,
	})
}

func runRelease(c *cli.Context) error {
	switch c.Args.Pos(0) {
	case "cut":
		return releaseCut(c)
	case "list", "":
		return releaseList(c)
	default:
		return fmt.Errorf("unknown release subcommand %q (cut|list)", c.Args.Pos(0))
	}
}

func releaseCut(c *cli.Context) error {
	e, err := load(c)
	if err != nil {
		return err
	}
	version := c.Args.Pos(1)
	if version == "" {
		return fmt.Errorf("usage: tbd release cut VERSION")
	}

	strategy := e.cfg.ReleaseStrategy
	if s, ok := c.Args.Get("strategy"); ok {
		strategy, err = config.ParseStrategy(s)
		if err != nil {
			return err
		}
	}

	// Reject versions that would produce an invalid git ref up front, before any
	// fetch, rather than leaking raw git porcelain mid-operation.
	if strategy.Has("branch") {
		if branch := e.cfg.ReleaseBranchPrefix + version; !e.repo.ValidBranchName(branch) {
			return fmt.Errorf("version %q is not valid for a release (it would make the invalid branch %q)", version, branch)
		}
	}
	if strategy.Has("tag") {
		if !strings.Contains(e.cfg.ReleaseTagTemplate, "{version}") {
			return fmt.Errorf("release-tag-template %q has no {version} placeholder; every release would collide on the tag %q",
				e.cfg.ReleaseTagTemplate, e.cfg.ReleaseTagTemplate)
		}
		if tag := strings.ReplaceAll(e.cfg.ReleaseTagTemplate, "{version}", version); !e.repo.ValidTagName(tag) {
			return fmt.Errorf("version %q is not valid for a release (it would make the invalid tag %q)", version, tag)
		}
	}

	if e.fetch {
		if err := e.step("fetching "+e.remote, func() error { return e.repo.Fetch(e.remote) }); err != nil {
			return err
		}
	}
	from := c.Args.GetOr("from", e.trunkRef)
	if !e.repo.Exists(from) {
		return fmt.Errorf("from ref %q does not exist", from)
	}

	// Invariant: releases come only from commits on trunk.
	onTrunk, err := e.guard(false).OnTrunk(from)
	if err != nil {
		return err
	}
	if !onTrunk {
		return fmt.Errorf("%q is not on trunk %s; releases must be cut from trunk commits", from, e.trunkLocal)
	}
	fromShort, _ := e.repo.Short(from)
	fmt.Fprintln(e.out, e.okMark(from+" @ "+fromShort+" is on trunk"))

	if strategy.Has("branch") {
		branch := e.cfg.ReleaseBranchPrefix + version
		if e.repo.Exists(branch) {
			return fmt.Errorf("release branch %q already exists", branch)
		}
		if err := e.repo.BranchCreate(branch, from); err != nil {
			return err
		}
		fmt.Fprintln(e.out, e.okMark("created release branch "+branch))
		if e.remote != "" && !c.Args.Flag("no-push") {
			if err := e.step("pushing "+branch+" to "+e.remote, func() error { return e.repo.PushBranch(e.remote, branch) }); err != nil {
				return fmt.Errorf("push %s: %w", branch, err)
			}
			fmt.Fprintln(e.out, e.okMark("pushed "+branch))
		}
	}

	if strategy.Has("tag") {
		tag := strings.ReplaceAll(e.cfg.ReleaseTagTemplate, "{version}", version)
		if e.repo.Exists(tag) {
			return fmt.Errorf("release tag %q already exists", tag)
		}
		if err := e.repo.TagAnnotated(tag, from, "release "+version); err != nil {
			return err
		}
		fmt.Fprintln(e.out, e.okMark("created release tag "+tag))
		if e.remote != "" && !c.Args.Flag("no-push") {
			if err := e.step("pushing tag "+tag+" to "+e.remote, func() error { return e.repo.PushTag(e.remote, tag) }); err != nil {
				return fmt.Errorf("push tag %s: %w", tag, err)
			}
			fmt.Fprintln(e.out, e.okMark("pushed "+tag))
		}
	}
	return nil
}

func releaseList(c *cli.Context) error {
	e, err := load(c)
	if err != nil {
		return err
	}
	branches, _ := e.repo.ListBranches(e.cfg.ReleaseBranchPrefix + "*")
	if len(branches) == 0 {
		fmt.Fprintln(e.out, e.colors.Dim("no release branches"))
	}
	for _, b := range branches {
		head, _ := e.repo.Short(b)
		onTrunk, _ := e.guard(false).OnTrunk(b)
		marker := e.okMark("on trunk")
		if !onTrunk {
			marker = e.colors.Yellow("⚠ off trunk")
		}
		fmt.Fprintf(e.out, "%-28s %s  %s\n", b, head, marker)
	}
	return nil
}
