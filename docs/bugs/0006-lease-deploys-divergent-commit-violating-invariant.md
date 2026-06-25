# Bug 0006: `tbd lease` deploys a commit that has diverged from trunk, violating tbd's central invariant

- **Component:** `internal/commands` (`leaseTag`, `leaseEphemeral`)
- **Affected commands:** `tbd lease NAME`, `tbd lease take NAME`, `tbd lease NAME to:REF` (tag and ephemeral-branch strategies)
- **Version:** tbd 1.11.2 (commit c9ac5dd)
- **Severity:** high — the deploy operation breaks the one invariant tbd exists to enforce
- **Status:** FIXED, verified (see Verification)

## Summary

tbd's stated central rule (README, first paragraph):

> **The head of the trunk must be an ancestor of whatever you operate on or produce.**
> ... you never integrate, release, or **deploy** work that has silently diverged from trunk.

`tbd lease` is the deploy operation ("Claim a deploy slot"). But it moves the
deploy lease to the current working-branch tip (or to `to:REF`) **without ever
checking the trunk-ancestor invariant**. A branch that has genuinely diverged
from trunk (trunk head is not an ancestor of it) can be deployed. The status
command even has a category for it ("orphaned"), yet the move that creates that
state is unguarded.

## Environment

- OS: Linux 6.18 (WSL2); system git
- tbd built with `go build -o tbd ./cmd/tbd` at commit c9ac5dd (v1.11.2)

## Steps to reproduce

```sh
W=$(mktemp -d); cd "$W"; git init -q -b develop r; cd r
git config user.name dev; git config user.email dev@x
echo s > s; git add .; git commit -qm seed
tbd init >/dev/null; git add -A; git commit -qm cfg
tbd lease dev-deploy :local            # bootstrap at trunk head

git checkout -q -b rogue               # branch off trunk
echo evil >> s; git commit -qam "rogue commit"
git checkout -q develop                # advance trunk PAST the fork
echo trunkmove >> t.txt; git add t.txt; git commit -qm "trunk advances"
git checkout -q rogue                  # rogue is now divergent: trunk head is NOT its ancestor

git merge-base --is-ancestor develop rogue && echo "ancestor" || echo "DIVERGED"
tbd lease dev-deploy :local            # deploy the divergent commit
git merge-base --is-ancestor develop "$(git rev-parse dev-deploy^{commit})" \
  && echo "deploy target on trunk" || echo "VIOLATION: deployed divergent work"
```

## Actual result (before fix)

```
DIVERGED
advancing dev-deploy to your latest commit
✓ dev-deploy -> b5e8445
VIOLATION: deployed divergent work
```

`dev-deploy` was moved onto `rogue`, a commit trunk head is not an ancestor of.

## Expected result

`tbd lease` refuses to move a deploy slot onto a commit that has diverged from
trunk (trunk head is not its ancestor), the same guarantee tbd makes for
integrate and release. Example:

```
tbd lease: refusing to deploy dev-deploy onto work that has diverged from trunk develop
  (trunk head is not an ancestor of b5e8445); rebase onto trunk first (tbd feature sync), or pass :force
```

## Root cause

`leaseTag` computes `dest` (the working tip, or `to:REF`, or - on bootstrap -
trunk head) and hands it straight to `moveLeaseTag`; `leaseEphemeral` remakes
the lease branch at the working tip. Neither calls the invariant guard. The
bootstrap case happens to be safe (it uses trunk head), but advance / take /
`to:REF` / ephemeral are not.

## Fix

- Added `env.ensureOnTrunk(name, dest)`: unless `:force` is set, it verifies
  trunk head is an ancestor of `dest` and returns a clear refusal otherwise.
- `leaseTag` calls it on the resolved `dest` before moving the tag;
  `leaseEphemeral` calls it on the working tip before remaking the branch. The
  bootstrap-at-trunk-head path trivially passes. `:force` overrides, consistent
  with the flag's existing "I know what I am doing" use for lease pushes.

## Verification

- Manual: the reproduction now refuses with exit 1 and leaves `dev-deploy` on
  trunk head; `:force` still allows it; a normal on-trunk deploy still works.
- Automated: `TestLeaseRefusesDivergentDeploy` (refusal + lease unmoved),
  `TestLeaseForceDeploysDivergent`, and the existing lease tests; the refusal
  test was confirmed to fail before the fix and pass after.
- `go vet ./...`, full `go test ./...`, and `scripts/e2e.sh` all pass.
