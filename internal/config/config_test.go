package config

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestStrategySetUnmarshal(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		want    []string
		wantErr bool
	}{
		{"scalar", "release-strategy: branch", []string{"branch"}, false},
		{"scalar-tag", "release-strategy: tag", []string{"tag"}, false},
		{"list", "release-strategy: [branch, tag]", []string{"branch", "tag"}, false},
		{"invalid-value", "release-strategy: merge", nil, true},
		{"invalid-kind", "release-strategy: {a: b}", nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var c struct {
				S StrategySet `yaml:"release-strategy"`
			}
			err := yaml.Unmarshal([]byte(tt.yaml), &c)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got none")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(c.S) != len(tt.want) {
				t.Fatalf("got %v, want %v", []string(c.S), tt.want)
			}
			for i := range tt.want {
				if c.S[i] != tt.want[i] {
					t.Fatalf("got %v, want %v", []string(c.S), tt.want)
				}
			}
		})
	}
}

func TestStrategyHas(t *testing.T) {
	s := StrategySet{"branch", "tag"}
	if !s.Has("branch") || !s.Has("tag") {
		t.Fatal("expected both strategies present")
	}
	if s.Has("merge") {
		t.Fatal("did not expect merge")
	}
}

func TestParseStrategy(t *testing.T) {
	s, err := ParseStrategy("branch,tag")
	if err != nil {
		t.Fatal(err)
	}
	if !s.Has("branch") || !s.Has("tag") {
		t.Fatalf("got %v", []string(s))
	}
	if _, err := ParseStrategy("nope"); err == nil {
		t.Fatal("expected error for invalid strategy")
	}
}

func TestDefaultAndAutoRebase(t *testing.T) {
	d := Default()
	if d.TrunkName != "develop" {
		t.Fatalf("trunk default = %q", d.TrunkName)
	}
	if !d.AutoRebaseEnabled() {
		t.Fatal("auto-rebase should default to true")
	}
	// nil pointer also means enabled.
	d.AutoRebase = nil
	if !d.AutoRebaseEnabled() {
		t.Fatal("nil auto-rebase should be enabled")
	}
	off := false
	d.AutoRebase = &off
	if d.AutoRebaseEnabled() {
		t.Fatal("explicit false should disable")
	}
}

func TestLoadMergesOverDefault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, FileName)
	content := "trunk-name: main\nrelease-strategy: [branch, tag]\nauto-rebase: false\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, found, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if found == "" {
		t.Fatal("expected to find config file")
	}
	if cfg.TrunkName != "main" {
		t.Fatalf("trunk = %q, want main", cfg.TrunkName)
	}
	// Unspecified fields keep defaults.
	if cfg.FeaturePrefix != "feature/" {
		t.Fatalf("feature-prefix = %q, want default", cfg.FeaturePrefix)
	}
	if !cfg.ReleaseStrategy.Has("tag") {
		t.Fatal("expected tag strategy")
	}
	if cfg.AutoRebaseEnabled() {
		t.Fatal("auto-rebase should be false from file")
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, FileName)
	orig := Default()
	orig.TrunkName = "trunk"
	orig.LeaseTags = []string{"dev", "uat"}
	if err := orig.Save(path); err != nil {
		t.Fatal(err)
	}
	got, _, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.TrunkName != "trunk" || len(got.LeaseTags) != 2 {
		t.Fatalf("round trip mismatch: %+v", got)
	}
}

func TestValidate(t *testing.T) {
	c := Default()
	c.LeaseTags = []string{"a", "a"}
	if err := c.Validate(); err == nil {
		t.Fatal("expected duplicate lease-tags error")
	}
	c = Default()
	c.TrunkName = ""
	if err := c.Validate(); err == nil {
		t.Fatal("expected empty trunk-name error")
	}
	c = Default()
	c.TagPush = "bogus"
	if err := c.Validate(); err == nil {
		t.Fatal("expected bad tag-push error")
	}

	for _, strat := range []string{"none", "tag", "ephemeral-branch", ""} {
		c = Default()
		c.LeaseStrategy = strat
		if err := c.Validate(); err != nil {
			t.Fatalf("lease-strategy %q should be valid: %v", strat, err)
		}
	}
	c = Default()
	c.LeaseStrategy = "bogus"
	if err := c.Validate(); err == nil {
		t.Fatal("expected bad lease-strategy error")
	}
	c = Default()
	c.LeaseBranches = []string{"deploy-now", "deploy-now"}
	if err := c.Validate(); err == nil {
		t.Fatal("expected duplicate lease-branches error")
	}
}
