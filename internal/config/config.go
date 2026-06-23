// Package config loads and saves tbd's per-repository configuration from a
// .tbd.yaml file found by walking up from the working directory.
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// FileName is the per-repo configuration file tbd looks for.
const FileName = ".tbd.yaml"

// Config mirrors .tbd.yaml. Defaults come from Default(); a loaded file is
// merged over those defaults so partial configs are valid.
type Config struct {
	TrunkName           string      `yaml:"trunk-name"`
	FeaturePrefix       string      `yaml:"feature-prefix"`
	ReleaseStrategy     StrategySet `yaml:"release-strategy"`
	ReleaseBranchPrefix string      `yaml:"release-branch-prefix"`
	ReleaseTagTemplate  string      `yaml:"release-tag-template"`
	LeaseStrategy       string      `yaml:"lease-strategy"` // none | tag | ephemeral-branch
	LeaseTags           []string    `yaml:"lease-tags"`     // used only when lease-strategy: tag
	LeaseBranches       []string    `yaml:"lease-branches"` // used only when lease-strategy: ephemeral-branch
	Remote              string      `yaml:"remote"`
	AutoRebase          *bool       `yaml:"auto-rebase"`
	TagPush             string      `yaml:"tag-push"`
}

// StrategySet is the value of release-strategy, which may be written as a single
// scalar ("branch") or a list (["branch", "tag"]).
type StrategySet []string

// UnmarshalYAML accepts either a scalar or a sequence of strategy names.
func (s *StrategySet) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		var single string
		if err := value.Decode(&single); err != nil {
			return err
		}
		*s = StrategySet{single}
	case yaml.SequenceNode:
		var list []string
		if err := value.Decode(&list); err != nil {
			return err
		}
		*s = StrategySet(list)
	default:
		return fmt.Errorf("release-strategy: expected a string or list, got yaml kind %d", value.Kind)
	}
	return s.validate()
}

func (s StrategySet) validate() error {
	for _, k := range s {
		if k != "branch" && k != "tag" {
			return fmt.Errorf("release-strategy: %q must be \"branch\" or \"tag\"", k)
		}
	}
	return nil
}

// Has reports whether the set contains a given strategy ("branch" or "tag").
func (s StrategySet) Has(kind string) bool {
	for _, k := range s {
		if k == kind {
			return true
		}
	}
	return false
}

// ParseStrategy turns a comma-separated CLI value ("branch,tag") into a set.
func ParseStrategy(csv string) (StrategySet, error) {
	var out StrategySet
	for _, part := range splitComma(csv) {
		out = append(out, part)
	}
	if err := out.validate(); err != nil {
		return nil, err
	}
	return out, nil
}

func boolPtr(b bool) *bool { return &b }

// Default returns the built-in configuration tbd uses when no file overrides it.
func Default() Config {
	return Config{
		TrunkName:           "develop",
		FeaturePrefix:       "feature/",
		ReleaseStrategy:     StrategySet{"branch"},
		ReleaseBranchPrefix: "release/",
		ReleaseTagTemplate:  "v{version}",
		LeaseStrategy:       "tag",
		LeaseTags:           []string{"dev-deploy", "uat1-deploy", "uat2-deploy"},
		Remote:              "origin",
		AutoRebase:          boolPtr(true),
		TagPush:             "with-lease",
	}
}

// AutoRebaseEnabled reports the effective auto-rebase setting (default true).
func (c Config) AutoRebaseEnabled() bool { return c.AutoRebase == nil || *c.AutoRebase }

// Find walks up from startDir looking for a .tbd.yaml, returning its path and
// whether one was found.
func Find(startDir string) (string, bool) {
	dir := startDir
	for {
		p := filepath.Join(dir, FileName)
		if _, err := os.Stat(p); err == nil {
			return p, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

// Load finds and reads the config nearest startDir, merged over Default(). The
// returned path is empty when no file exists (defaults are returned).
func Load(startDir string) (Config, string, error) {
	cfg := Default()
	path, ok := Find(startDir)
	if !ok {
		return cfg, "", nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, path, err
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, path, fmt.Errorf("parse %s: %w", path, err)
	}
	if err := cfg.Validate(); err != nil {
		return cfg, path, err
	}
	return cfg, path, nil
}

// Save writes the config as YAML to path.
func (c Config) Save(path string) error {
	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// Validate checks the config is internally consistent.
func (c Config) Validate() error {
	if c.TrunkName == "" {
		return fmt.Errorf("trunk-name must not be empty")
	}
	if c.FeaturePrefix == "" {
		return fmt.Errorf("feature-prefix must not be empty")
	}
	if c.ReleaseBranchPrefix == "" && c.ReleaseStrategy.Has("branch") {
		return fmt.Errorf("release-branch-prefix must not be empty when release-strategy includes \"branch\"")
	}
	if err := c.ReleaseStrategy.validate(); err != nil {
		return err
	}
	switch c.LeaseStrategy {
	case "", "none", "tag", "ephemeral-branch":
	default:
		return fmt.Errorf("lease-strategy must be \"none\", \"tag\", or \"ephemeral-branch\", got %q", c.LeaseStrategy)
	}
	if err := uniqueNonEmpty("lease-tags", c.LeaseTags); err != nil {
		return err
	}
	if err := uniqueNonEmpty("lease-branches", c.LeaseBranches); err != nil {
		return err
	}
	switch c.TagPush {
	case "", "with-lease", "force":
	default:
		return fmt.Errorf("tag-push must be \"with-lease\" or \"force\", got %q", c.TagPush)
	}
	return nil
}

// uniqueNonEmpty checks a name list has no empty or duplicate entries.
func uniqueNonEmpty(field string, items []string) error {
	seen := map[string]bool{}
	for _, it := range items {
		if it == "" {
			return fmt.Errorf("%s must not contain empty entries", field)
		}
		if seen[it] {
			return fmt.Errorf("%s has duplicate %q", field, it)
		}
		seen[it] = true
	}
	return nil
}

func splitComma(s string) []string {
	var out []string
	cur := ""
	for _, r := range s {
		if r == ',' {
			if cur != "" {
				out = append(out, cur)
			}
			cur = ""
			continue
		}
		cur += string(r)
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}
