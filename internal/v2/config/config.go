// Package config defines tbd v2's repository configuration.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	stdconfig "goforge.dev/goplus/std/config"
	"gopkg.in/yaml.v3"
)

const FileName = ".tbd.yaml"

// Config is the v2 .tbd.yaml schema. It intentionally models workflows rather
// than individual git commands.
type Config struct {
	Version    int             `yaml:"version"`
	TrunkName  string          `yaml:"trunk-name"`
	Remote     string          `yaml:"remote"`
	AutoRebase bool            `yaml:"auto-rebase"`
	Rerere     bool            `yaml:"rerere"`
	Branches   BranchConfig    `yaml:"branches"`
	Release    ReleaseConfig   `yaml:"release"`
	Deploy     DeployConfig    `yaml:"deploy"`
	Push       PushConfig      `yaml:"push"`
	Locks      LockConfig      `yaml:"locks"`
	Hooks      HookConfig      `yaml:"hooks"`
	Tasks      map[string]Task `yaml:"tasks,omitempty"`
	Visualize  VisualConfig    `yaml:"visualize"`
}

type BranchConfig struct {
	FeatureTemplate string `yaml:"feature-template"`
	FixTemplate     string `yaml:"fix-template"`
	CollabSuffix    string `yaml:"collab-suffix"`
	StackSuffix     string `yaml:"stack-suffix"`
}

type ReleaseConfig struct {
	Strategy            string `yaml:"strategy"` // tag | branch
	BranchPrefix        string `yaml:"branch-prefix"`
	TagTemplate         string `yaml:"tag-template"`
	RCTagTemplate       string `yaml:"rc-tag-template"`
	BadTagTemplate      string `yaml:"bad-tag-template"`
	DefaultRevertBump   string `yaml:"default-revert-bump"`
	DeleteRemoteRCOnUAT bool   `yaml:"delete-remote-rc-on-uat-reset"`
}

type DeployConfig struct {
	Strategy string   `yaml:"strategy"` // tag | branch
	Refs     []string `yaml:"refs"`
}

type PushConfig struct {
	Branch string `yaml:"branch"`
	Tag    string `yaml:"tag"`
}

type LockConfig struct {
	RefPrefix  string `yaml:"ref-prefix"`
	DefaultTTL string `yaml:"default-ttl"`
}

type HookConfig struct {
	PreCommit   []HookStep            `yaml:"pre-commit,omitempty"`
	PostCommit  []HookStep            `yaml:"post-commit,omitempty"`
	PrePush     []HookStep            `yaml:"pre-push,omitempty"`
	PostPush    []HookStep            `yaml:"post-push,omitempty"`
	PreLease    []HookStep            `yaml:"pre-lease,omitempty"`
	PostLease   []HookStep            `yaml:"post-lease,omitempty"`
	PreRelease  []HookStep            `yaml:"pre-release,omitempty"`
	PostRelease []HookStep            `yaml:"post-release,omitempty"`
	DeployRefs  map[string]DeployHook `yaml:"deploy-refs,omitempty"`
}

type DeployHook struct {
	PrePush  []HookStep `yaml:"pre-push,omitempty"`
	PostPush []HookStep `yaml:"post-push,omitempty"`
}

type Task struct {
	Desc     string            `yaml:"desc,omitempty"`
	Command  string            `yaml:"command"`
	Env      map[string]string `yaml:"env,omitempty"`
	Timeout  string            `yaml:"timeout,omitempty"`
	Optional bool              `yaml:"optional,omitempty"`
}

// HookStep may either inline a command or reference a named task.
type HookStep struct {
	Name     string `yaml:"name,omitempty"`
	Command  string `yaml:"command,omitempty"`
	Optional bool   `yaml:"optional,omitempty"`
	Timeout  string `yaml:"timeout,omitempty"`
}

type VisualConfig struct {
	FetchInterval string   `yaml:"fetch-interval"`
	Repos         []string `yaml:"repos,omitempty"`
}

func Default() Config {
	return Config{
		Version:    2,
		TrunkName:  "main",
		Remote:     "origin",
		AutoRebase: true,
		Rerere:     true,
		Branches: BranchConfig{
			FeatureTemplate: "feature/{id}-{slug}",
			FixTemplate:     "bugfix/{id-}{slug}",
			CollabSuffix:    "-collab",
			StackSuffix:     "-stack",
		},
		Release: ReleaseConfig{
			Strategy:            "tag",
			BranchPrefix:        "release/",
			TagTemplate:         "v{semver}",
			RCTagTemplate:       "rc-{semver}",
			BadTagTemplate:      "bad-{timestamp}",
			DefaultRevertBump:   "patch",
			DeleteRemoteRCOnUAT: true,
		},
		Deploy: DeployConfig{
			Strategy: "tag",
			Refs:     []string{"dev-deploy", "test-deploy", "prod-deploy"},
		},
		Push: PushConfig{
			Branch: "force-with-lease",
			Tag:    "force-with-lease",
		},
		Locks: LockConfig{
			RefPrefix:  "refs/tbd/locks/",
			DefaultTTL: "3h",
		},
		Tasks: map[string]Task{},
		Visualize: VisualConfig{
			FetchInterval: "15s",
		},
	}
}

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
	schema := stdconfig.Schema[Config]{
		Defaults:   Default,
		Decoder:    yamlDecoder{},
		Validators: []stdconfig.Validator[Config]{stdconfig.ValidateFunc[Config](func(c Config) error { return c.Validate() })},
	}
	cfg, err = schema.Load(data)
	if err != nil {
		return cfg, path, err
	}
	return cfg, path, nil
}

type yamlDecoder struct{}

func (yamlDecoder) Decode(data []byte, base Config) (Config, error) {
	if err := yaml.Unmarshal(data, &base); err != nil {
		return Config{}, fmt.Errorf("parse YAML: %w", err)
	}
	return base, nil
}

func (c Config) Save(path string) error {
	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func (c Config) Validate() error {
	if c.Version == 0 {
		c.Version = 2
	}
	if c.TrunkName == "" {
		return stdconfig.At("trunk-name", fmt.Errorf("must not be empty"))
	}
	if c.Remote == "" {
		return stdconfig.At("remote", fmt.Errorf("must not be empty"))
	}
	if c.Branches.FeatureTemplate == "" || c.Branches.FixTemplate == "" {
		return stdconfig.At("branches", fmt.Errorf("templates must not be empty"))
	}
	switch c.Release.Strategy {
	case "tag", "branch":
	default:
		return stdconfig.At("release.strategy", fmt.Errorf("must be tag or branch"))
	}
	if c.Release.BranchPrefix == "" {
		return stdconfig.At("release.branch-prefix", fmt.Errorf("must not be empty"))
	}
	if c.Release.TagTemplate == "" || c.Release.RCTagTemplate == "" {
		return stdconfig.At("release", fmt.Errorf("tag templates must not be empty"))
	}
	switch c.Deploy.Strategy {
	case "tag", "branch":
	default:
		return stdconfig.At("deploy.strategy", fmt.Errorf("must be tag or branch"))
	}
	if len(c.Deploy.Refs) == 0 {
		return stdconfig.At("deploy.refs", fmt.Errorf("must contain at least one ref"))
	}
	switch c.Push.Branch {
	case "", "force-with-lease", "force":
	default:
		return stdconfig.At("push.branch", fmt.Errorf("must be force-with-lease or force"))
	}
	switch c.Push.Tag {
	case "", "force-with-lease", "force":
	default:
		return stdconfig.At("push.tag", fmt.Errorf("must be force-with-lease or force"))
	}
	if c.Locks.RefPrefix == "" {
		return stdconfig.At("locks.ref-prefix", fmt.Errorf("must not be empty"))
	}
	if _, err := time.ParseDuration(c.Locks.DefaultTTL); err != nil {
		return stdconfig.At("locks.default-ttl", err)
	}
	return nil
}
