// Package git is a thin wrapper over the git CLI. It is the only place in tbd
// that shells out to git; every other package works through these primitives so
// behavior is easy to test against throwaway repositories.
package git

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// Repo binds a working directory so callers need not repeat it.
type Repo struct{ Dir string }

// Open returns a Repo for dir after verifying it is inside a git work tree.
func Open(dir string) (*Repo, error) {
	r := &Repo{Dir: dir}
	if _, err := r.run("rev-parse", "--is-inside-work-tree"); err != nil {
		return nil, fmt.Errorf("%s is not inside a git work tree", dir)
	}
	return r, nil
}

// run executes git with combined behavior: stdout is returned trimmed; on a
// non-zero exit the error wraps git's stderr.
func (r *Repo) run(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = r.Dir
	var out, errb strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(errb.String())
		if msg == "" {
			msg = err.Error()
		}
		return strings.TrimRight(out.String(), "\n"), fmt.Errorf("git %s: %s", strings.Join(args, " "), msg)
	}
	return strings.TrimRight(out.String(), "\n"), nil
}

// runOK runs git for its exit status only (used by predicates like
// merge-base --is-ancestor).
func (r *Repo) runOK(args ...string) bool {
	cmd := exec.Command("git", args...)
	cmd.Dir = r.Dir
	return cmd.Run() == nil
}

// IsAncestor reports whether a is an ancestor of (or equal to) b. This is the
// core invariant primitive.
func (r *Repo) IsAncestor(a, b string) bool {
	return r.runOK("merge-base", "--is-ancestor", a, b)
}

// RevParse resolves ref to a full commit sha, erroring if it does not exist.
func (r *Repo) RevParse(ref string) (string, error) {
	return r.run("rev-parse", "--verify", "--quiet", ref+"^{commit}")
}

// RefSha returns the raw object sha a ref points at, without peeling (so an
// annotated tag yields the tag object, which is what --force-with-lease compares
// against). Returns "" when the ref does not exist.
func (r *Repo) RefSha(ref string) string {
	out, err := r.run("rev-parse", "--verify", "--quiet", ref)
	if err != nil {
		return ""
	}
	return out
}

// CommitOf returns the commit ref points at, or "" if it does not resolve.
func (r *Repo) CommitOf(ref string) string {
	sha, err := r.RevParse(ref)
	if err != nil {
		return ""
	}
	return sha
}

// Exists reports whether ref resolves to anything.
func (r *Repo) Exists(ref string) bool {
	return r.runOK("rev-parse", "--verify", "--quiet", ref)
}

// HasRemote reports whether the named remote is configured.
func (r *Repo) HasRemote(remote string) bool {
	return r.runOK("remote", "get-url", remote)
}

// Fetch updates remote-tracking refs and prunes deleted ones.
func (r *Repo) Fetch(remote string) error {
	_, err := r.run("fetch", "--prune", remote)
	return err
}

// FetchTags updates local tags from the remote, overwriting moved ones so lease
// status reflects the authoritative remote position.
func (r *Repo) FetchTags(remote string) error {
	_, err := r.run("fetch", "--tags", "--force", remote)
	return err
}

// CurrentBranch returns the checked-out branch name, erroring on detached HEAD.
func (r *Repo) CurrentBranch() (string, error) {
	out, err := r.run("symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil {
		return "", fmt.Errorf("HEAD is detached")
	}
	return out, nil
}

// Short returns the abbreviated sha for ref.
func (r *Repo) Short(ref string) (string, error) {
	return r.run("rev-parse", "--short", ref)
}

// IsClean reports whether the working tree has no uncommitted changes.
func (r *Repo) IsClean() (bool, error) {
	out, err := r.run("status", "--porcelain")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) == "", nil
}

// BranchCreate creates a branch at start without checking it out.
func (r *Repo) BranchCreate(name, start string) error {
	_, err := r.run("branch", name, start)
	return err
}

// BranchDelete removes a local branch (force).
func (r *Repo) BranchDelete(name string) error {
	_, err := r.run("branch", "-D", name)
	return err
}

// Checkout switches the working tree to ref.
func (r *Repo) Checkout(ref string) error {
	_, err := r.run("switch", ref)
	return err
}

// Rebase replays the current branch onto onto.
func (r *Repo) Rebase(onto string) error {
	_, err := r.run("rebase", onto)
	return err
}

// RebaseAbort cancels an in-progress rebase (best effort).
func (r *Repo) RebaseAbort() error {
	_, err := r.run("rebase", "--abort")
	return err
}

// RebaseInProgress reports whether a rebase is currently stopped (e.g. waiting
// on conflict resolution).
func (r *Repo) RebaseInProgress() bool {
	for _, name := range []string{"rebase-merge", "rebase-apply"} {
		p, err := r.run("rev-parse", "--git-path", name)
		if err != nil {
			continue
		}
		if !filepath.IsAbs(p) {
			p = filepath.Join(r.Dir, p)
		}
		if _, err := os.Stat(p); err == nil {
			return true
		}
	}
	return false
}

// UnmergedPaths lists files that still have unresolved conflicts.
func (r *Repo) UnmergedPaths() ([]string, error) {
	out, err := r.run("diff", "--name-only", "--diff-filter=U")
	if err != nil {
		return nil, err
	}
	return nonEmptyLines(out), nil
}

// RebaseContinue resumes a stopped rebase without opening an editor (the
// existing commit message is kept).
func (r *Repo) RebaseContinue() error {
	cmd := exec.Command("git", "rebase", "--continue")
	cmd.Dir = r.Dir
	cmd.Env = append(os.Environ(), "GIT_EDITOR=true")
	var out, errb strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(errb.String())
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("git rebase --continue: %s", msg)
	}
	return nil
}

// MergeSquash stages the combined changes of ref relative to the current branch
// without committing or recording a merge, so they can be committed as one.
func (r *Repo) MergeSquash(ref string) error {
	_, err := r.run("merge", "--squash", ref)
	return err
}

// FFMerge fast-forwards the current branch to ref, refusing a merge commit.
func (r *Repo) FFMerge(ref string) error {
	_, err := r.run("merge", "--ff-only", ref)
	return err
}

// TagAnnotated creates or moves an annotated tag at ref, recording msg. The
// tagger identity (git config user) becomes the lease holder record.
func (r *Repo) TagAnnotated(name, ref, msg string) error {
	_, err := r.run("tag", "-f", "-a", name, ref, "-m", msg)
	return err
}

// TagDetail is the resolved state of a tag.
type TagDetail struct {
	Name    string
	Short   string
	Tagger  string
	Date    string
	Subject string
}

// TagInfo returns details for a tag, or ok=false if it does not exist.
func (r *Repo) TagInfo(name string) (TagDetail, bool) {
	const format = "%(objectname:short)%09%(taggername)%09%(taggerdate:short)%09%(contents:subject)"
	out, err := r.run("for-each-ref", "--format="+format, "refs/tags/"+name)
	if err != nil || strings.TrimSpace(out) == "" {
		return TagDetail{}, false
	}
	parts := strings.Split(out, "\t")
	d := TagDetail{Name: name}
	if len(parts) > 0 {
		d.Short = parts[0]
	}
	if len(parts) > 1 {
		d.Tagger = parts[1]
	}
	if len(parts) > 2 {
		d.Date = parts[2]
	}
	if len(parts) > 3 {
		d.Subject = parts[3]
	}
	return d, true
}

// ListBranches returns local branch names matching pattern (glob, e.g.
// "feature/*"). An empty result is not an error.
func (r *Repo) ListBranches(pattern string) ([]string, error) {
	out, err := r.run("for-each-ref", "--format=%(refname:short)", "refs/heads/"+pattern)
	if err != nil {
		return nil, err
	}
	return nonEmptyLines(out), nil
}

// ListTags returns tag names matching pattern (glob).
func (r *Repo) ListTags(pattern string) ([]string, error) {
	out, err := r.run("tag", "--list", pattern)
	if err != nil {
		return nil, err
	}
	return nonEmptyLines(out), nil
}

// ReflogContains reports whether branch's reflog ever pointed at sha. This is
// how we recognize a commit that used to be on the working branch before an
// amend or rebase rewrote it (a now-orphaned "ours"). Best effort: the reflog is
// local and expires.
func (r *Repo) ReflogContains(branch, sha string) bool {
	out, err := r.run("log", "-g", "--format=%H", branch)
	if err != nil {
		return false
	}
	for _, line := range nonEmptyLines(out) {
		if line == sha {
			return true
		}
	}
	return false
}

// BranchesContaining returns the local branches whose history includes sha.
func (r *Repo) BranchesContaining(sha string) []string {
	out, err := r.run("branch", "--contains", sha, "--format=%(refname:short)")
	if err != nil {
		return nil
	}
	return nonEmptyLines(out)
}

// AheadBehind returns how many commits a is ahead of and behind b.
func (r *Repo) AheadBehind(a, b string) (ahead, behind int, err error) {
	out, err := r.run("rev-list", "--left-right", "--count", a+"..."+b)
	if err != nil {
		return 0, 0, err
	}
	fields := strings.Fields(out)
	if len(fields) != 2 {
		return 0, 0, fmt.Errorf("unexpected rev-list output %q", out)
	}
	ahead, _ = strconv.Atoi(fields[0])
	behind, _ = strconv.Atoi(fields[1])
	return ahead, behind, nil
}

// MergeBase returns the best common ancestor of a and b.
func (r *Repo) MergeBase(a, b string) (string, error) {
	return r.run("merge-base", a, b)
}

// Commit is a short sha plus its subject line.
type Commit struct {
	Short   string
	Subject string
}

// StageAll stages every change in the working tree (tracked and untracked).
func (r *Repo) StageAll() error {
	_, err := r.run("add", "-A")
	return err
}

// HasStaged reports whether the index has staged changes.
func (r *Repo) HasStaged() bool {
	return !r.runOK("diff", "--cached", "--quiet")
}

// Commit creates a commit from the staged changes with the given message.
func (r *Repo) Commit(msg string) error {
	_, err := r.run("commit", "-m", msg)
	return err
}

// CommitAmend folds staged changes into the current commit. An empty msg keeps
// the existing message; otherwise it replaces it. --allow-empty so a pure
// re-sync (no staged changes) still succeeds.
func (r *Repo) CommitAmend(msg string) error {
	args := []string{"commit", "--amend", "--allow-empty"}
	if msg == "" {
		args = append(args, "--no-edit")
	} else {
		args = append(args, "-m", msg)
	}
	_, err := r.run(args...)
	return err
}

// CommitInteractive makes a commit that opens the user's editor for the message,
// inheriting the terminal. With amend it rewords/folds into the current commit;
// a non-empty seed pre-fills the editor (used when squashing, to start from the
// kept message).
func (r *Repo) CommitInteractive(amend bool, seed string) error {
	args := []string{"commit"}
	if amend {
		args = append(args, "--amend")
	}
	if seed != "" {
		args = append(args, "-e", "-m", seed)
	}
	cmd := exec.Command("git", args...)
	cmd.Dir = r.Dir
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return nil
}

// ResetSoft moves HEAD to ref while keeping the index and working tree, so the
// combined content can be re-committed as one.
func (r *Repo) ResetSoft(ref string) error {
	_, err := r.run("reset", "--soft", ref)
	return err
}

// FullMessage returns ref's complete commit message ("" on error).
func (r *Repo) FullMessage(ref string) string {
	out, err := r.run("log", "-1", "--format=%B", ref)
	if err != nil {
		return ""
	}
	return strings.TrimRight(out, "\n")
}

// Subject returns the first line of ref's commit message ("" on error).
func (r *Repo) Subject(ref string) string {
	out, err := r.run("log", "-1", "--format=%s", ref)
	if err != nil {
		return ""
	}
	return out
}

// LogRange returns commits in revRange (e.g. "fork..head"), newest first.
func (r *Repo) LogRange(revRange string) ([]Commit, error) {
	out, err := r.run("log", "--format=%h%x09%s", revRange)
	if err != nil {
		return nil, err
	}
	var commits []Commit
	for _, line := range nonEmptyLines(out) {
		sha, subj, _ := strings.Cut(line, "\t")
		commits = append(commits, Commit{Short: sha, Subject: subj})
	}
	return commits, nil
}

// PushBranch pushes a local branch to remote (never forced; trunk must
// fast-forward).
func (r *Repo) PushBranch(remote, branch string) error {
	_, err := r.run("push", remote, branch)
	return err
}

// UpdateRef sets a ref to a value (creates it if absent).
func (r *Repo) UpdateRef(name, value string) error {
	_, err := r.run("update-ref", name, value)
	return err
}

// RemoteHasBranch reports whether the remote currently has the named branch.
func (r *Repo) RemoteHasBranch(remote, branch string) bool {
	out, err := r.run("ls-remote", "--heads", remote, branch)
	if err != nil {
		return false
	}
	return strings.TrimSpace(out) != ""
}

// PushBranchCAS publishes a branch with --force-with-lease using an explicit
// expected remote value (empty means "expect the branch to be absent"), setting
// upstream. Force is required because tbd rewrites feature history on every
// commit; the explicit lease makes it a true compare-and-swap that survives a
// preceding fetch, so a teammate's push is never clobbered.
func (r *Repo) PushBranchCAS(remote, branch, expected string) error {
	lease := "--force-with-lease=refs/heads/" + branch + ":" + expected
	_, err := r.run("push", "-u", lease, remote, branch)
	return err
}

// PushBranchForce publishes a branch with an unconditional force (the escape
// hatch when the lease check is in the way).
func (r *Repo) PushBranchForce(remote, branch string) error {
	_, err := r.run("push", "-u", "--force", remote, branch)
	return err
}

// RemoteBranchSha returns the sha the remote currently has for a branch, or ""
// if the remote does not have it.
func (r *Repo) RemoteBranchSha(remote, branch string) string {
	out, err := r.run("ls-remote", "--heads", remote, "refs/heads/"+branch)
	if err != nil {
		return ""
	}
	fields := strings.Fields(strings.TrimSpace(out))
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

// CommitterName returns the committer name of ref's tip ("" on error).
func (r *Repo) CommitterName(ref string) string {
	out, err := r.run("log", "-1", "--format=%cn", ref)
	if err != nil {
		return ""
	}
	return out
}

// PushDeleteBranch deletes a branch on the remote.
func (r *Repo) PushDeleteBranch(remote, branch string) error {
	_, err := r.run("push", remote, "--delete", branch)
	return err
}

// PushTag pushes a tag to the remote without forcing (for fresh release tags).
func (r *Repo) PushTag(remote, tag string) error {
	_, err := r.run("push", remote, "refs/tags/"+tag)
	return err
}

// TagLightweight creates or moves a lightweight tag at ref.
func (r *Repo) TagLightweight(name, ref string) error {
	_, err := r.run("tag", "-f", name, ref)
	return err
}

// PushTagCAS pushes a tag using compare-and-swap: the push succeeds only if the
// remote tag is still at expectedOld (use "" when the tag should not yet exist).
// This is the sole mutual-exclusion primitive a lease relies on.
func (r *Repo) PushTagCAS(remote, tag, expectedOld string) error {
	lease := "--force-with-lease=refs/tags/" + tag + ":" + expectedOld
	_, err := r.run("push", "--atomic", lease, remote, "refs/tags/"+tag)
	return err
}

// PushTagForce pushes a tag with an unconditional force (the escape hatch when
// tag-push is set to "force" or :force is passed).
func (r *Repo) PushTagForce(remote, tag string) error {
	_, err := r.run("push", "--force", remote, "refs/tags/"+tag)
	return err
}

func nonEmptyLines(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		if strings.TrimSpace(line) != "" {
			out = append(out, line)
		}
	}
	return out
}
