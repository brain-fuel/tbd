package commands

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"goforge.dev/tbd/internal/cli"
	"goforge.dev/tbd/internal/config"
	"goforge.dev/tbd/internal/git"
	"goforge.dev/tbd/internal/invariant"
	"goforge.dev/tbd/internal/render"
)

// env bundles the resolved configuration, repository, and trunk identity shared
// by every command. trunkRef is what invariant checks run against (the remote
// trunk when a remote exists, so we compare against what others have pushed);
// trunkLocal is the local branch name used for fast-forward merges.
type env struct {
	cfg        config.Config
	repo       *git.Repo
	colors     render.Colors
	out        io.Writer
	errOut     io.Writer
	remote     string // "" when no remote is in play (offline or :local)
	trunkLocal string // local trunk branch name
	trunkRef   string // resolved trunk ref for checks
	fetch      bool   // whether mutating ops should fetch first
}

// load resolves the env for a command invocation.
func load(c *cli.Context) (env, error) {
	cfg, _, err := config.Load(c.Dir)
	if err != nil {
		return env{}, err
	}
	repo, err := git.Open(c.Dir)
	if err != nil {
		return env{}, err
	}

	e := env{
		cfg:        cfg,
		repo:       repo,
		colors:     c.Colors(),
		out:        c.Stdout,
		errOut:     c.Stderr,
		trunkLocal: cfg.TrunkName,
	}

	useRemote := !c.Args.Flag("local") && repo.HasRemote(cfg.Remote)
	if useRemote {
		e.remote = cfg.Remote
		e.trunkRef = cfg.Remote + "/" + cfg.TrunkName
	} else {
		e.trunkRef = cfg.TrunkName
	}
	// Fetch by default for network-aware commands unless asked not to.
	e.fetch = useRemote && !c.Args.Flag("no-fetch")
	return e, nil
}

// guard builds the invariant guard from the env.
func (e env) guard(requireClean bool) invariant.Guard {
	return invariant.Guard{
		Repo:         e.repo,
		Trunk:        e.trunkRef,
		RequireClean: requireClean,
		Fetch:        e.fetch,
		Remote:       e.remote,
	}
}

// ensureLocalTrunk makes sure a local trunk branch exists, creating it from the
// remote trunk when missing. Returns the trunk head sha for convenience.
func (e env) ensureLocalTrunk() error {
	if e.repo.Exists(e.trunkLocal) {
		return nil
	}
	if e.remote != "" && e.repo.Exists(e.trunkRef) {
		return e.repo.BranchCreate(e.trunkLocal, e.trunkRef)
	}
	return invariant.ErrNoTrunk
}

// rebasePlan builds the before/after picture for replaying feature onto trunkRef.
func (e env) rebasePlan(feature string) (render.RebasePlan, error) {
	fork, err := e.repo.MergeBase(e.trunkRef, feature)
	if err != nil {
		return render.RebasePlan{}, err
	}
	forkShort, _ := e.repo.Short(fork)
	forkSubj := e.repo.Subject(fork)

	trunkCommits, _ := e.repo.LogRange(fork + ".." + e.trunkRef)
	featCommits, _ := e.repo.LogRange(fork + ".." + feature)

	return render.RebasePlan{
		Feature:   feature,
		Trunk:     e.trunkLocal,
		Fork:      render.Commit{Short: forkShort, Subject: forkSubj},
		TrunkLine: toRenderCommits(trunkCommits),
		FeatLine:  toRenderCommits(featCommits),
	}, nil
}

// ErrRebaseConflict marks a rebase that stopped on a conflict, so callers can
// decide whether to leave it in progress or abort.
var ErrRebaseConflict = errors.New("rebase stopped on a conflict")

// visualizeRebase prints the rebase plan, then performs the rebase on the
// currently checked-out branch. The feature must already be checked out. A
// conflict is returned wrapped in ErrRebaseConflict (the rebase is left in
// progress for the caller to resolve or abort).
func (e env) visualizeRebase(feature string) error {
	plan, err := e.rebasePlan(feature)
	if err != nil {
		return err
	}
	fmt.Fprintln(e.out, e.colors.Bold("Rebasing "+feature+" onto "+e.trunkLocal+":"))
	fmt.Fprintln(e.out)
	fmt.Fprint(e.out, plan.Render(e.colors))
	fmt.Fprintln(e.out)
	if err := e.repo.Rebase(e.trunkRef); err != nil {
		return fmt.Errorf("%w: %v", ErrRebaseConflict, err)
	}
	fmt.Fprintln(e.out, e.colors.Green("✓ rebased - "+feature+" now sits on top of "+e.trunkLocal))
	return nil
}

// handleRebaseConflict centralizes the response to a conflicting rebase: with
// :abort-on-conflict the rebase is rolled back and the branch left unchanged;
// otherwise it is left in progress with guidance. Non-conflict errors pass
// through unchanged.
func handleRebaseConflict(e env, c *cli.Context, branch string, rerr error) error {
	if !errors.Is(rerr, ErrRebaseConflict) {
		return rerr
	}
	if c.Args.Flag("abort-on-conflict") {
		_ = e.repo.RebaseAbort()
		fmt.Fprintln(e.errOut, e.colors.Yellow("rebase aborted; "+branch+" is unchanged"))
		return cli.ExitError{Code: 1}
	}
	fmt.Fprintln(e.errOut, e.badMark("rebase of "+branch+" hit a conflict"))
	fmt.Fprintln(e.errOut, e.colors.Dim("  fix the files, \"git add\" them, then run \"tbd continue\","))
	fmt.Fprintln(e.errOut, e.colors.Dim("  or re-run with :abort-on-conflict to back out"))
	return cli.ExitError{Code: 1}
}

func toRenderCommits(in []git.Commit) []render.Commit {
	out := make([]render.Commit, len(in))
	for i, c := range in {
		out[i] = render.Commit{Short: c.Short, Subject: c.Subject}
	}
	return out
}

// okMark / badMark render the green/red invariant indicators.
func (e env) okMark(s string) string  { return e.colors.Green("✓ " + s) }
func (e env) badMark(s string) string { return e.colors.Red("✗ " + s) }

// exists reports whether a path is present on disk.
func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// splitCSV splits "a,b,c" into ["a","b","c"], dropping empty entries.
func splitCSV(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}
