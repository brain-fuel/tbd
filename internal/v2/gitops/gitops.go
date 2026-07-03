// Package gitops contains reusable tbd v2 operations over git.
package gitops

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"goforge.dev/tbd/internal/git"
	v2config "goforge.dev/tbd/internal/v2/config"
	"goforge.dev/tbd/internal/v2/state"
)

type Env struct {
	Repo     *git.Repo
	Root     string
	Config   v2config.Config
	Out      io.Writer
	Err      io.Writer
	DryRun   bool
	RemoteOK bool
	TrunkRef string
}

func Load(dir string, out, errOut io.Writer, dryRun bool) (Env, error) {
	repo, err := git.Open(dir)
	if err != nil {
		return Env{}, err
	}
	root, err := repo.Root()
	if err != nil {
		return Env{}, err
	}
	cfg, _, err := v2config.Load(root)
	if err != nil {
		return Env{}, err
	}
	remoteOK := repo.HasRemote(cfg.Remote)
	trunkRef := cfg.TrunkName
	if remoteOK {
		trunkRef = cfg.Remote + "/" + cfg.TrunkName
	}
	return Env{Repo: repo, Root: root, Config: cfg, Out: out, Err: errOut, DryRun: dryRun, RemoteOK: remoteOK, TrunkRef: trunkRef}, nil
}

func (e Env) Fetch() error {
	if !e.RemoteOK {
		return nil
	}
	if e.DryRun {
		fmt.Fprintf(e.Out, "dry-run: git fetch --prune %s\n", e.Config.Remote)
		return nil
	}
	if err := e.Repo.Fetch(e.Config.Remote); err != nil {
		return err
	}
	_ = e.Repo.FetchTags(e.Config.Remote)
	return nil
}

func (e Env) UpdateLocalTrunk() error {
	if !e.RemoteOK {
		return nil
	}
	if err := e.Fetch(); err != nil {
		return err
	}
	remote := e.Config.Remote + "/" + e.Config.TrunkName
	if !e.Repo.Exists(remote) {
		return fmt.Errorf("remote trunk %q does not exist", remote)
	}
	remoteHead, err := e.Repo.RevParse(remote)
	if err != nil {
		return err
	}
	if !e.Repo.Exists(e.Config.TrunkName) {
		if e.DryRun {
			fmt.Fprintf(e.Out, "dry-run: create local trunk %s at %s\n", e.Config.TrunkName, remote)
			return nil
		}
		return e.Repo.BranchCreate(e.Config.TrunkName, remote)
	}
	localHead, err := e.Repo.RevParse(e.Config.TrunkName)
	if err != nil {
		return err
	}
	if localHead == remoteHead {
		return nil
	}
	if !e.Repo.IsAncestor(localHead, remoteHead) {
		return fmt.Errorf("local trunk %s and %s diverged; resolve before tbd can continue", e.Config.TrunkName, remote)
	}
	if e.DryRun {
		fmt.Fprintf(e.Out, "dry-run: fast-forward %s to %s\n", e.Config.TrunkName, remoteHead[:12])
		return nil
	}
	return e.Repo.UpdateRef("refs/heads/"+e.Config.TrunkName, remoteHead)
}

func (e Env) EnsureNotProtectedBranch() error {
	cur, err := e.Repo.CurrentBranch()
	if err != nil {
		return err
	}
	if cur == e.Config.TrunkName {
		return fmt.Errorf("refusing to run from trunk branch %q", cur)
	}
	for _, ref := range e.Config.Deploy.Refs {
		if cur == ref {
			return fmt.Errorf("refusing to run from deploy ref %q; check out a work branch first", cur)
		}
	}
	if strings.HasPrefix(cur, e.Config.Release.BranchPrefix) {
		return fmt.Errorf("refusing to run from immutable release branch %q", cur)
	}
	return nil
}

func BranchName(cfg v2config.Config, kind, id, desc string) string {
	slug := state.Slug(desc)
	switch kind {
	case "feature":
		return renderTemplate(cfg.Branches.FeatureTemplate, id, slug)
	case "fix":
		return renderTemplate(cfg.Branches.FixTemplate, id, slug)
	default:
		return "feature/" + slug
	}
}

func GroupBranch(cfg v2config.Config, kind string, ids []string) string {
	suffix := cfg.Branches.StackSuffix
	if kind == "collab" {
		suffix = cfg.Branches.CollabSuffix
	}
	return state.GroupName(ids, suffix)
}

func renderTemplate(tpl, id, slug string) string {
	out := strings.ReplaceAll(tpl, "{id}", id)
	if id == "" {
		out = strings.ReplaceAll(out, "{id-}", "")
	} else {
		out = strings.ReplaceAll(out, "{id-}", id+"-")
	}
	out = strings.ReplaceAll(out, "{slug}", slug)
	return strings.TrimRight(out, "-/")
}

func ReleaseTag(cfg v2config.Config, semver string) string {
	return strings.ReplaceAll(cfg.Release.TagTemplate, "{semver}", semver)
}

func RCTag(cfg v2config.Config, semver string) string {
	return strings.ReplaceAll(cfg.Release.RCTagTemplate, "{semver}", semver)
}

func BadTag(cfg v2config.Config) string {
	ts := time.Now().UTC().Format("20060102T150405Z")
	return strings.ReplaceAll(cfg.Release.BadTagTemplate, "{timestamp}", ts)
}

func (e Env) CreateBranch(branch, start string) error {
	if !e.Repo.ValidBranchName(branch) {
		return fmt.Errorf("%q is not a valid branch name", branch)
	}
	if e.Repo.Exists(branch) {
		return fmt.Errorf("branch %q already exists", branch)
	}
	if other, clash := e.Repo.ConflictingBranch(branch); clash {
		return fmt.Errorf("cannot create branch %q: collides with existing branch %q", branch, other)
	}
	if e.DryRun {
		fmt.Fprintf(e.Out, "dry-run: create branch %s at %s\n", branch, start)
		return nil
	}
	if err := e.Repo.BranchCreate(branch, start); err != nil {
		return err
	}
	return e.Repo.Checkout(branch)
}

func (e Env) CommitWorkflow(message string) error {
	if e.DryRun {
		fmt.Fprintf(e.Out, "dry-run: commit workflow metadata: %s\n", message)
		return nil
	}
	if err := e.Repo.StageAll(); err != nil {
		return err
	}
	if !e.Repo.HasStaged() {
		return nil
	}
	return e.Repo.Commit(message)
}

func (e Env) SquashToOne(branch, target, seed string, edit bool) error {
	fork, err := e.Repo.MergeBase(target, branch)
	if err != nil {
		fork = target
	}
	existing, _ := e.Repo.LogRange(fork + ".." + branch)
	switch n := len(existing); {
	case n == 0:
		if !e.Repo.HasStaged() {
			return fmt.Errorf("nothing to squash on %s", branch)
		}
		if edit {
			return e.Repo.CommitInteractive(false, seed)
		}
		return e.Repo.Commit(seed)
	case n == 1:
		if edit {
			return e.Repo.CommitInteractive(true, seed)
		}
		return e.Repo.CommitAmend(seed)
	default:
		keep := seed
		if keep == "" {
			keep = e.Repo.FullMessage(branch)
		}
		if err := e.Repo.ResetSoft(fork); err != nil {
			return err
		}
		if edit {
			return e.Repo.CommitInteractive(false, keep)
		}
		return e.Repo.Commit(keep)
	}
}

func (e Env) RebaseOnto(branch, target string) error {
	if e.DryRun {
		fmt.Fprintf(e.Out, "dry-run: rebase %s onto %s\n", branch, target)
		return nil
	}
	if err := e.Repo.Checkout(branch); err != nil {
		return err
	}
	targetHead, err := e.Repo.RevParse(target)
	if err != nil {
		return err
	}
	head, err := e.Repo.RevParse(branch)
	if err != nil {
		return err
	}
	if e.Repo.IsAncestor(targetHead, head) {
		return nil
	}
	return e.Repo.Rebase(target)
}

func (e Env) EnsureOnTrunk(ref string) error {
	trunkHead, err := e.Repo.RevParse(e.TrunkRef)
	if err != nil {
		return err
	}
	refHead, err := e.Repo.RevParse(ref)
	if err != nil {
		return err
	}
	if !e.Repo.IsAncestor(trunkHead, refHead) {
		return fmt.Errorf("%s is not rebased onto current trunk %s", ref, e.TrunkRef)
	}
	return nil
}

func (e Env) InvalidateStaleUAT() error {
	st, err := state.Load(e.Root)
	if err != nil {
		return err
	}
	changed := false
	for semver, u := range st.UAT {
		if !u.Valid || u.Commit == "" {
			continue
		}
		trunkHead, err := e.Repo.RevParse(e.TrunkRef)
		if err != nil {
			continue
		}
		if e.Repo.IsAncestor(trunkHead, u.Commit) {
			continue
		}
		u.Valid = false
		u.Reason = "trunk moved after UAT started"
		u.UpdatedAt = git.NowRFC3339()
		st.UAT[semver] = u
		changed = true
		rc := RCTag(e.Config, semver)
		if e.Repo.Exists("refs/tags/"+rc) && !e.DryRun {
			_ = e.Repo.TagDelete(rc)
		}
		if e.RemoteOK && e.Config.Release.DeleteRemoteRCOnUAT && !e.DryRun {
			_ = e.Repo.PushDeleteTag(e.Config.Remote, rc)
		}
		fmt.Fprintf(e.Err, "UAT for %s invalidated; removed %s\n", semver, rc)
	}
	if !changed {
		return nil
	}
	if err := state.Save(e.Root, st); err != nil {
		return err
	}
	return e.CommitWorkflow("chore(tbd): invalidate stale UAT")
}

type LockInfo struct {
	Name     string `json:"name"`
	Owner    string `json:"owner"`
	Email    string `json:"email"`
	Host     string `json:"host"`
	Commit   string `json:"commit"`
	Acquired string `json:"acquired"`
	Expires  string `json:"expires"`
}

func (e Env) AcquireLock(name string, ttl time.Duration, steal bool) error {
	ref := e.Config.Locks.RefPrefix + name
	_ = e.Fetch()
	remoteOld := ""
	if e.RemoteOK {
		remoteOld = e.Repo.RemoteRefSha(e.Config.Remote, ref)
	}
	if e.Repo.Exists(ref) {
		if info, ok := e.LockInfo(name); ok {
			exp, _ := time.Parse(time.RFC3339, info.Expires)
			if time.Now().UTC().Before(exp) && !steal {
				return fmt.Errorf("lock %q is held by %s until %s; pass --steal to override", name, info.Owner, info.Expires)
			}
			if !steal {
				return fmt.Errorf("lock %q is expired; pass --steal to take it", name)
			}
		}
	}
	head, _ := e.Repo.RevParse("HEAD")
	host, _ := os.Hostname()
	info := LockInfo{
		Name:     name,
		Owner:    e.Repo.ConfigGet("user.name"),
		Email:    e.Repo.ConfigGet("user.email"),
		Host:     host,
		Commit:   head,
		Acquired: git.NowRFC3339(),
		Expires:  time.Now().UTC().Add(ttl).Format(time.RFC3339),
	}
	msg, _ := json.MarshalIndent(info, "", "  ")
	tree, _ := e.Repo.EmptyTree()
	commit, err := e.Repo.CommitTree(tree, string(msg))
	if err != nil {
		return err
	}
	if e.DryRun {
		fmt.Fprintf(e.Out, "dry-run: acquire lock %s at %s\n", name, ref)
		return nil
	}
	if err := e.Repo.UpdateRef(ref, commit); err != nil {
		return err
	}
	if e.RemoteOK {
		if err := e.Repo.PushRefCAS(e.Config.Remote, ref, remoteOld); err != nil {
			return fmt.Errorf("lock CAS push rejected: %w", err)
		}
	}
	return nil
}

func (e Env) ReleaseLock(name string, force bool) error {
	ref := e.Config.Locks.RefPrefix + name
	if !e.Repo.Exists(ref) {
		return fmt.Errorf("lock %q is not held", name)
	}
	if !force {
		if info, ok := e.LockInfo(name); ok {
			me := e.Repo.ConfigGet("user.email")
			if info.Email != "" && me != "" && info.Email != me {
				return fmt.Errorf("lock %q is held by %s; pass --force to release", name, info.Owner)
			}
		}
	}
	if e.DryRun {
		fmt.Fprintf(e.Out, "dry-run: release lock %s\n", name)
		return nil
	}
	if e.RemoteOK {
		_ = e.Repo.PushDeleteRef(e.Config.Remote, ref)
	}
	return e.Repo.DeleteRef(ref)
}

func (e Env) LockInfo(name string) (LockInfo, bool) {
	ref := e.Config.Locks.RefPrefix + name
	if !e.Repo.Exists(ref) {
		return LockInfo{}, false
	}
	msg := e.Repo.RefMessage(ref)
	var info LockInfo
	if err := json.Unmarshal([]byte(msg), &info); err != nil {
		return LockInfo{}, false
	}
	return info, true
}
