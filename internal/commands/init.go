package commands

import (
	"fmt"
	"path/filepath"

	"goforge.dev/tbd/internal/cli"
	"goforge.dev/tbd/internal/config"
	"goforge.dev/tbd/internal/git"
)

func init() {
	cli.Register(&cli.Command{
		Name:    "init",
		Summary: "Write a .tbd.yaml for this repository",
		Usage: "tbd init [trunk:NAME] [feature-prefix:P] [release-prefix:P]\n" +
			"         [release-strategy:branch|tag|branch,tag] [lease-tags:a,b,c]\n" +
			"         [:create-trunk] [:force]\n\n" +
			"Writes .tbd.yaml with defaults merged over any flags given.\n" +
			":create-trunk creates the trunk branch from HEAD if it is missing.",
		Run: runInit,
	})
}

func runInit(c *cli.Context) error {
	repo, err := git.Open(c.Dir)
	if err != nil {
		return err
	}

	cfg := config.Default()
	if v, ok := c.Args.Get("trunk"); ok {
		cfg.TrunkName = v
	}
	if v, ok := c.Args.Get("feature-prefix"); ok {
		cfg.FeaturePrefix = v
	}
	if v, ok := c.Args.Get("release-prefix"); ok {
		cfg.ReleaseBranchPrefix = v
	}
	if v, ok := c.Args.Get("release-strategy"); ok {
		set, err := config.ParseStrategy(v)
		if err != nil {
			return err
		}
		cfg.ReleaseStrategy = set
	}
	if v, ok := c.Args.Get("lease-tags"); ok {
		cfg.LeaseTags = splitCSV(v)
	}
	if err := cfg.Validate(); err != nil {
		return err
	}

	path := filepath.Join(c.Dir, config.FileName)
	if exists(path) && !c.Args.Flag("force") {
		return fmt.Errorf("%s already exists (use :force to overwrite)", config.FileName)
	}

	// Trunk handling.
	if !repo.Exists(cfg.TrunkName) {
		if c.Args.Flag("create-trunk") {
			if err := repo.BranchCreate(cfg.TrunkName, "HEAD"); err != nil {
				return fmt.Errorf("create trunk %q: %w", cfg.TrunkName, err)
			}
			fmt.Fprintf(c.Stdout, "created trunk branch %q at HEAD\n", cfg.TrunkName)
		} else {
			fmt.Fprintf(c.Stderr, "warning: trunk branch %q does not exist; run with :create-trunk or create it before using tbd\n", cfg.TrunkName)
		}
	}

	if err := cfg.Save(path); err != nil {
		return err
	}
	colors := c.Colors()
	fmt.Fprintf(c.Stdout, "%s wrote %s\n", colors.Green("✓"), config.FileName)
	fmt.Fprintf(c.Stdout, "  trunk-name:       %s\n", cfg.TrunkName)
	fmt.Fprintf(c.Stdout, "  feature-prefix:   %s\n", cfg.FeaturePrefix)
	fmt.Fprintf(c.Stdout, "  release-strategy: %v\n", []string(cfg.ReleaseStrategy))
	fmt.Fprintf(c.Stdout, "  lease-tags:       %v\n", cfg.LeaseTags)
	return nil
}
