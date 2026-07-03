package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"goforge.dev/tbd/v2/internal/git"
	v2config "goforge.dev/tbd/v2/internal/v2/config"
)

func newInitCommand(opts *rootOptions) *cobra.Command {
	cfg := v2config.Default()
	var yes, force, noRerere bool
	cmd := &cobra.Command{
		Use:   "init",
		Short: "initialize tbd v2 configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			repo, err := git.Open(".")
			if err != nil {
				return err
			}
			root, err := repo.Root()
			if err != nil {
				return err
			}
			if !yes {
				cfg.TrunkName = prompt(cfg.TrunkName, "trunk branch")
				cfg.Remote = prompt(cfg.Remote, "remote")
				cfg.Release.Strategy = prompt(cfg.Release.Strategy, "release strategy (tag|branch)")
				cfg.Deploy.Strategy = prompt(cfg.Deploy.Strategy, "deploy ref strategy (tag|branch)")
				refs := prompt(strings.Join(cfg.Deploy.Refs, ","), "deploy refs")
				cfg.Deploy.Refs = splitCSV(refs)
			}
			if err := cfg.Validate(); err != nil {
				return err
			}
			path := filepath.Join(root, v2config.FileName)
			if _, err := os.Stat(path); err == nil && !force {
				return fmt.Errorf("%s already exists; pass --force to overwrite", v2config.FileName)
			}
			if opts.dryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "dry-run: write %s\n", path)
				return nil
			}
			if err := cfg.Save(path); err != nil {
				return err
			}
			if cfg.Rerere {
				if err := repo.ConfigSet("rerere.enabled", "true"); err != nil {
					return err
				}
			}
			fmt.Fprintf(cmd.OutOrStdout(), "wrote %s\n", path)
			return nil
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "accept defaults")
	cmd.Flags().BoolVar(&yes, "default", false, "accept defaults")
	cmd.Flags().BoolVar(&force, "force", false, "overwrite existing .tbd.yaml")
	cmd.Flags().StringVar(&cfg.TrunkName, "trunk", cfg.TrunkName, "configured trunk branch")
	cmd.Flags().StringVar(&cfg.Remote, "remote", cfg.Remote, "git remote")
	cmd.Flags().StringVar(&cfg.Release.Strategy, "release-strategy", cfg.Release.Strategy, "tag or branch")
	cmd.Flags().StringVar(&cfg.Deploy.Strategy, "deploy-strategy", cfg.Deploy.Strategy, "tag or branch")
	cmd.Flags().StringSliceVar(&cfg.Deploy.Refs, "deploy-ref", cfg.Deploy.Refs, "deploy ref; repeatable")
	cmd.Flags().BoolVar(&cfg.Rerere, "rerere", cfg.Rerere, "enable git rerere for this repository")
	cmd.Flags().BoolVar(&noRerere, "no-rerere", false, "disable git rerere")
	cmd.PreRun = func(cmd *cobra.Command, args []string) {
		if noRerere {
			cfg.Rerere = false
		}
	}
	return cmd
}

func splitCSV(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
