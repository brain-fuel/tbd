package commands

import (
	"fmt"

	"goforge.dev/tbd/v2/internal/argv"
	"goforge.dev/tbd/v2/internal/cli"
	"goforge.dev/tbd/v2/internal/config"
)

func init() {
	cli.Register(&cli.Command{
		Name:    "config",
		Summary: "Show the resolved tbd configuration",
		Usage: "tbd config list\n" +
			"tbd config get KEY\n\n" +
			"KEY is one of: trunk-name, feature-prefix, release-strategy,\n" +
			"release-branch-prefix, release-tag-template, lease-strategy,\n" +
			"lease-tags, remote, auto-rebase, tag-push.",
		Spec: argv.Spec{Positionals: []string{"subcommand", "key"}},
		Run:  runConfig,
	})
}

func runConfig(c *cli.Context) error {
	cfg, path, err := config.Load(c.Dir)
	if err != nil {
		return err
	}
	values := map[string]string{
		"trunk-name":            cfg.TrunkName,
		"feature-prefix":        cfg.FeaturePrefix,
		"release-strategy":      fmt.Sprintf("%v", []string(cfg.ReleaseStrategy)),
		"release-branch-prefix": cfg.ReleaseBranchPrefix,
		"release-tag-template":  cfg.ReleaseTagTemplate,
		"lease-strategy":        cfg.LeaseStrategy,
		"lease-tags":            fmt.Sprintf("%v", cfg.LeaseTags),
		"lease-branches":        fmt.Sprintf("%v", cfg.LeaseBranches),
		"remote":                cfg.Remote,
		"auto-rebase":           fmt.Sprintf("%t", cfg.AutoRebaseEnabled()),
		"tag-push":              cfg.TagPush,
		"branch-push":           cfg.BranchPush,
	}
	order := []string{
		"trunk-name", "feature-prefix", "release-strategy", "release-branch-prefix",
		"release-tag-template", "lease-strategy", "lease-tags", "lease-branches",
		"remote", "auto-rebase", "tag-push", "branch-push",
	}

	switch c.Args.Pos(0) {
	case "", "list":
		if path != "" {
			fmt.Fprintf(c.Stdout, "%s\n", c.Colors().Dim("# "+path))
		} else {
			fmt.Fprintf(c.Stdout, "%s\n", c.Colors().Dim("# defaults (no .tbd.yaml found)"))
		}
		for _, k := range order {
			fmt.Fprintf(c.Stdout, "%-22s %s\n", k+":", values[k])
		}
		return nil
	case "get":
		key := c.Args.Pos(1)
		v, ok := values[key]
		if !ok {
			return fmt.Errorf("unknown config key %q", key)
		}
		fmt.Fprintln(c.Stdout, v)
		return nil
	default:
		return fmt.Errorf("usage: tbd config list | tbd config get KEY")
	}
}
