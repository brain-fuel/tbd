// Package invariant enforces tbd's central rule: before any mutating operation,
// the head of the trunk must be an ancestor of the ref being operated on or
// produced. It is the single chokepoint every mutating command calls.
package invariant

import (
	"errors"

	"goforge.dev/tbd/v2/internal/git"
)

// Sentinel errors callers match to decide how to respond (refuse vs auto-fix).
var (
	ErrDiverged   = errors.New("trunk head is not an ancestor of target (history diverged)")
	ErrDirty      = errors.New("working tree has uncommitted changes")
	ErrDetached   = errors.New("HEAD is detached")
	ErrNoTrunk    = errors.New("trunk branch does not exist")
	ErrNotOnTrunk = errors.New("commit is not on trunk")
)

// Guard binds the repo, the resolved trunk ref, and preflight policy.
type Guard struct {
	Repo         *git.Repo
	Trunk        string // resolved trunk ref, e.g. "develop" or "origin/develop"
	RequireClean bool
	Fetch        bool
	Remote       string
	// Step, when set, wraps a slow operation (the fetch) so callers can
	// telegraph it and animate progress. nil runs the operation directly.
	Step func(label string, fn func() error) error
}

func (g Guard) step(label string, fn func() error) error {
	if g.Step == nil {
		return fn()
	}
	return g.Step(label, fn)
}

// Report is the read-only result of inspecting a target against trunk.
type Report struct {
	Trunk      string
	TrunkHead  string
	Target     string
	TargetHead string
	Ahead      int // target commits not on trunk
	Behind     int // trunk commits not on target
	Diverged   bool
	Dirty      bool
}

// Check inspects target against trunk without mutating anything. It powers the
// status and guard commands and the "before" picture of a rebase.
func (g Guard) Check(target string) (Report, error) {
	rep := Report{Trunk: g.Trunk, Target: target}
	if !g.Repo.Exists(g.Trunk) {
		return rep, ErrNoTrunk
	}
	trunkHead, err := g.Repo.RevParse(g.Trunk)
	if err != nil {
		return rep, ErrNoTrunk
	}
	rep.TrunkHead = trunkHead

	targetHead, err := g.Repo.RevParse(target)
	if err != nil {
		return rep, err
	}
	rep.TargetHead = targetHead

	if ahead, behind, err := g.Repo.AheadBehind(g.Trunk, target); err == nil {
		// rev-list left-right trunk...target: left = trunk-only (behind),
		// right = target-only (ahead of trunk).
		rep.Behind = ahead
		rep.Ahead = behind
	}
	rep.Diverged = !g.Repo.IsAncestor(trunkHead, targetHead)

	if clean, err := g.Repo.IsClean(); err == nil {
		rep.Dirty = !clean
	}
	return rep, nil
}

// Ensure enforces the invariant for a mutating operation:
//  1. optionally fetch so the check runs against the real remote trunk head
//  2. working tree clean when RequireClean
//  3. trunk head IS an ancestor of target
//
// It returns a sentinel error so the caller can choose to auto-rebase or refuse.
func (g Guard) Ensure(target string) error {
	if g.Fetch && g.Remote != "" && g.Repo.HasRemote(g.Remote) {
		if err := g.step("fetching "+g.Remote, func() error { return g.Repo.Fetch(g.Remote) }); err != nil {
			return err
		}
	}
	if !g.Repo.Exists(g.Trunk) {
		return ErrNoTrunk
	}
	trunkHead, err := g.Repo.RevParse(g.Trunk)
	if err != nil {
		return ErrNoTrunk
	}
	if g.RequireClean {
		clean, err := g.Repo.IsClean()
		if err != nil {
			return err
		}
		if !clean {
			return ErrDirty
		}
	}
	targetHead, err := g.Repo.RevParse(target)
	if err != nil {
		return err
	}
	if !g.Repo.IsAncestor(trunkHead, targetHead) {
		return ErrDiverged
	}
	return nil
}

// OnTrunk reports whether ref is contained in trunk's history (an ancestor of
// trunk head). Releases and lease moves require this.
func (g Guard) OnTrunk(ref string) (bool, error) {
	trunkHead, err := g.Repo.RevParse(g.Trunk)
	if err != nil {
		return false, ErrNoTrunk
	}
	refHead, err := g.Repo.RevParse(ref)
	if err != nil {
		return false, err
	}
	return g.Repo.IsAncestor(refHead, trunkHead), nil
}
