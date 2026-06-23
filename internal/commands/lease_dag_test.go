package commands

import (
	"strings"
	"testing"
)

func tagCommit(t *testing.T, dir, name string) string {
	t.Helper()
	r, err := openRepo(dir)
	if err != nil {
		t.Fatal(err)
	}
	return r.CommitOf(name)
}

func revOf(t *testing.T, dir, ref string) string {
	t.Helper()
	r, _ := openRepo(dir)
	sha, err := r.RevParse(ref)
	if err != nil {
		t.Fatalf("rev-parse %s: %v", ref, err)
	}
	return sha
}

// Unset lease bootstraps to TRUNK head, not the working branch tip.
func TestLeaseBootstrapToTrunkHead(t *testing.T) {
	dir := repoFixture(t)
	startFeatureBare(t, dir, "p")
	writeFile(t, dir, "a.txt", "a")
	if err := runCommit(mustCtx(dir, "commit", "message:work")); err != nil {
		t.Fatal(err)
	}
	// feature/p is now ahead of develop.
	if err := leaseAcquire(mustCtx(dir, "lease", "dev-deploy"), "dev-deploy"); err != nil {
		t.Fatal(err)
	}
	if got, want := tagCommit(t, dir, "dev-deploy"), revOf(t, dir, "develop"); got != want {
		t.Fatalf("bootstrap should target trunk head %s, got %s", want, got)
	}
	if tagCommit(t, dir, "dev-deploy") == revOf(t, dir, "feature/p") {
		t.Fatal("bootstrap should NOT target the feature tip")
	}
}

// A lease on an earlier commit of the working branch advances to its tip,
// including after an amend rewrites history (recognized via the reflog).
func TestLeaseAdvanceAfterAmend(t *testing.T) {
	dir := repoFixture(t)
	startFeatureBare(t, dir, "p")
	writeFile(t, dir, "a.txt", "a")
	if err := runCommit(mustCtx(dir, "commit", "message:work")); err != nil {
		t.Fatal(err)
	}
	c1 := revOf(t, dir, "feature/p")

	// Point the lease at c1 (current feature tip).
	if err := leaseAcquire(mustCtx(dir, "lease", "dev-deploy"), "dev-deploy"); err != nil {
		t.Fatal(err)
	}
	// Bootstrap put it on trunk head; lease again to advance onto c1.
	if err := leaseAcquire(mustCtx(dir, "lease", "dev-deploy"), "dev-deploy"); err != nil {
		t.Fatal(err)
	}
	if tagCommit(t, dir, "dev-deploy") != c1 {
		t.Fatalf("lease should have advanced to feature tip %s", c1)
	}

	// Amend: rewrites the feature commit, orphaning c1.
	writeFile(t, dir, "b.txt", "b")
	if err := runCommit(mustCtx(dir, "commit")); err != nil {
		t.Fatal(err)
	}
	c2 := revOf(t, dir, "feature/p")
	if c2 == c1 {
		t.Fatal("amend should have changed the feature tip")
	}
	// Lease was on c1 (now orphaned); reflog recognizes it as ours -> advance to c2.
	if err := leaseAcquire(mustCtx(dir, "lease", "dev-deploy"), "dev-deploy"); err != nil {
		t.Fatal(err)
	}
	if got := tagCommit(t, dir, "dev-deploy"); got != c2 {
		t.Fatalf("lease should advance to amended tip %s, got %s", c2, got)
	}
}

// :no-advance leaves a stale-mine lease where it is.
func TestLeaseNoAdvance(t *testing.T) {
	dir := repoFixture(t)
	startFeatureBare(t, dir, "p")
	writeFile(t, dir, "a.txt", "a")
	if err := runCommit(mustCtx(dir, "commit", "message:work")); err != nil {
		t.Fatal(err)
	}
	// Bootstrap onto trunk head.
	if err := leaseAcquire(mustCtx(dir, "lease", "dev-deploy"), "dev-deploy"); err != nil {
		t.Fatal(err)
	}
	before := tagCommit(t, dir, "dev-deploy")
	// stale-mine (trunk head is an ancestor of feature/p) but :no-advance.
	if err := leaseAcquire(mustCtx(dir, "lease", "dev-deploy", ":no-advance"), "dev-deploy"); err != nil {
		t.Fatal(err)
	}
	if after := tagCommit(t, dir, "dev-deploy"); after != before {
		t.Fatalf(":no-advance should not move the lease (%s -> %s)", before, after)
	}
}

// A lease sitting on another branch is taken to the working branch tip.
func TestLeaseTakeForeign(t *testing.T) {
	dir := repoFixture(t)
	// A foreign branch with its own commit.
	gitRun(t, dir, "switch", "-q", "-c", "feature/other", "develop")
	writeAndCommit(t, dir, "other.txt", "other work")
	other := revOf(t, dir, "feature/other")
	// Park the lease on the foreign commit.
	r, _ := openRepo(dir)
	if err := r.TagAnnotated("dev-deploy", other, "parked"); err != nil {
		t.Fatal(err)
	}

	// Our branch with our own commit.
	gitRun(t, dir, "switch", "-q", "develop")
	startFeatureBare(t, dir, "mine")
	writeFile(t, dir, "mine.txt", "mine")
	if err := runCommit(mustCtx(dir, "commit", "message:mine")); err != nil {
		t.Fatal(err)
	}
	mineHead := revOf(t, dir, "feature/mine")

	// Foreign -> take to our tip.
	if err := leaseAcquire(mustCtx(dir, "lease", "dev-deploy"), "dev-deploy"); err != nil {
		t.Fatal(err)
	}
	if got := tagCommit(t, dir, "dev-deploy"); got != mineHead {
		t.Fatalf("foreign lease should be taken to our tip %s, got %s", mineHead, got)
	}
}

// Leasing a commit already at the destination is a no-op.
func TestLeaseNoOpWhenCurrent(t *testing.T) {
	dir := repoFixture(t)
	startFeatureBare(t, dir, "p")
	writeFile(t, dir, "a.txt", "a")
	if err := runCommit(mustCtx(dir, "commit", "message:work")); err != nil {
		t.Fatal(err)
	}
	// Bootstrap, then advance to tip, then a third call is a no-op.
	for i := 0; i < 2; i++ {
		if err := leaseAcquire(mustCtx(dir, "lease", "dev-deploy"), "dev-deploy"); err != nil {
			t.Fatal(err)
		}
	}
	at := tagCommit(t, dir, "dev-deploy")
	ctx, out, _ := newCtx(dir, "lease", "dev-deploy")
	if err := runLease(ctx); err != nil {
		t.Fatal(err)
	}
	if tagCommit(t, dir, "dev-deploy") != at {
		t.Fatal("no-op lease should not move the tag")
	}
	if !strings.Contains(out.String(), "already deployed") {
		t.Fatalf("expected no-op message, got: %s", out.String())
	}
}
