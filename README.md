# tbd - trunk-based development over git

`tbd` is a small, opinionated wrapper over git's DAG. It exists to enforce one
invariant on every mutating operation:

> **The head of the trunk must be an ancestor of whatever you operate on or produce.**

That single rule (`git merge-base --is-ancestor`) is what keeps trunk-based
development safe: you never integrate, release, or deploy work that has silently
diverged from trunk. When your branch *has* diverged, `tbd` rebases it onto the
latest trunk for you and **shows you the move** as a before/after graph - nothing
happens silently.

## Install

```sh
go build -o tbd ./cmd/tbd
```

## Quick start

```sh
tbd learn                      # narrated tour of the whole workflow
tbd init                       # write .tbd.yaml (defaults below)
tbd feature start login        # branch feature/login off the latest trunk
# ...commit work...
tbd feature finish             # rebase onto trunk, fast-forward trunk, push, clean up
tbd release cut 1.0.0          # cut a release from a trunk commit
tbd lease dev-deploy           # move the deploy tag to your work (one taker wins)
tbd status                     # dashboard of trunk, branches, leases, releases
tbd guard                      # exit 0/1: does the invariant hold? (for CI)
```

Arguments use colon syntax: `key:value` for named args, `:flag` for booleans
(e.g. `tbd feature finish :no-push`, `tbd release cut 1.0.0 strategy:branch,tag`).

## Configuration - `.tbd.yaml`

```yaml
trunk-name: develop
feature-prefix: feature/
release-strategy: branch          # "branch" | "tag" | [branch, tag]
release-branch-prefix: release/
release-tag-template: v{version}
lease-strategy: tag               # none | tag | ephemeral-branch
lease-tags: [dev-deploy, uat1-deploy, uat2-deploy]   # used when lease-strategy: tag
lease-branches: [deploy-now]      # used when lease-strategy: ephemeral-branch
remote: origin
auto-rebase: true                 # false = refuse on divergence instead of rebasing
tag-push: with-lease              # "with-lease" (CAS) | "force"
```

## Commands

| Command | What it does | Invariant enforced |
|---|---|---|
| `learn` | Guided walkthrough of the whole workflow (`tbd learn topics` to jump) | - |
| `init` | Write `.tbd.yaml`; `:create-trunk` makes the trunk if missing | - |
| `status` | Trunk, current branch, features, leases, releases | read-only |
| `commit` | Collapse the feature to ONE commit, fetch trunk, rebase onto it | single commit, always rebased |
| `continue` | Resume a tbd rebase after resolving conflicts (`:abort` to back out) | - |
| `feature start NAME` | Branch `feature/NAME` from trunk head | start point is trunk head |
| `feature sync [BR]` | Rebase a feature onto the latest trunk (the explicit fixer) | trunk head ⊑ feature after |
| `feature push [BR]` | Publish the feature branch (force-with-lease, for PR/CI) | rebased onto trunk before publishing |
| `feature finish [BR]` | Rebase (auto), fast-forward trunk, push, delete branch | trunk head ⊑ feature; trunk only fast-forwards |
| `feature list` | Feature branches with ahead/behind + status | read-only |
| `release cut VERSION` | Branch and/or tag per `release-strategy` | `from` must be on trunk |
| `release list` | Release points + on-trunk marker | read-only |
| `lease NAME` | Move deploy tag `NAME` per the DAG rules below (CAS) | single winner per race |
| `lease status` | Each deploy tag: position + holder | read-only |
| `guard` / `check` | Report the invariant; exit 0 if it holds, 1 otherwise | read-only |
| `config list` / `config get KEY` | Show resolved configuration | read-only |
| `version` | Print the tbd version | - |

### Divergence and auto-rebase

When trunk has moved ahead of your feature, `feature finish` (and `feature sync`)
rebase it onto the latest trunk and print the move:

```
Rebasing feature/widget onto develop:

before
  ● 2324c61  trunk advances  (trunk head: develop)
  │
  │ ○ b07c5ca  widget work  ← feature/widget
  ├─╯
  ◇ 01ebddf  login work  (fork point)

after
  ● b07c5ca  widget work  ← feature/widget (replayed)
  ● 2324c61  trunk advances  (trunk head: develop)
  │
  ◇ 01ebddf  login work
```

Set `auto-rebase: false` to refuse instead, with a hint to run `tbd feature sync`.

### Single-commit features: `tbd commit`

`tbd commit` enforces a one-commit-per-feature discipline. Every invocation, no
exceptions, does the same three things:

1. Stages all changes and collapses the feature to exactly **one** commit
   (creates it, amends it, or squashes several into one).
2. Fetches the trunk.
3. Rebases that single commit onto the latest trunk head.

```sh
tbd feature start login
# ...edit...
tbd commit message:"add login form"   # first commit (message required)
# ...edit more...
tbd commit                            # amends the same commit, re-rebases onto trunk
```

The result is invariant: after any `tbd commit`, the feature is one commit
sitting directly on top of trunk. A message is required only for the first
commit; later ones keep it unless you pass a new `message:`/`m:`, or use `:edit`
to open your editor on the message (reword on the first commit, an amend, or a
squash; it still collapses-to-one and rebases).

If the rebase in step 3 conflicts, your commit is already made (work is never
lost); the rebase stops with the file left in conflict. Fix it, `git add` the
file, and run `tbd continue` (which resumes without opening an editor), or
`tbd commit :abort-on-conflict` to back the rebase out and stay as you were.
The same `tbd continue` resolves conflicts from `feature sync`, `finish`, and
`push` too.

`commit` keeps everything local. To publish the branch for a pull request or CI,
use `tbd feature push` - it rebases onto trunk, then force-with-lease pushes
(force because `commit` rewrites history; the lease never clobbers a teammate's
push). `tbd feature finish` is different: it folds the feature into trunk and
deletes the branch, so reach for `push` when you want the branch reviewed first
and `finish` when you are ready to integrate.

### Leases (deploy slots), decided at the DAG level

A *lease* is the deploy tag your CD pipeline watches (e.g. `dev-deploy`,
`uat-deploy`). `tbd lease <name>` moves it to the commit you want deployed,
choosing the move from where the tag points now (T) relative to your working
branch (W):

| T (where the tag is now) | What `tbd lease` does |
|---|---|
| unset | bootstrap to **trunk head** |
| already at the destination | no-op |
| on W, or in W's reflog (your earlier or pre-amend commit) | **advance** to W's tip (`:no-advance` to leave it) |
| on someone else's branch | **take** it to W's tip |

The "on W's reflog" rule is what makes it survive `tbd commit`: after an amend or
rebase rewrites your commit, the old deployed commit is orphaned but still in
your branch's reflog, so it's recognized as yours and advanced rather than
mistaken for a stranger's. (Reflog is local and expires; an aged-out orphan
degrades to a "take", same destination.)

**What git actually guarantees.** There is no native lock. The one
mutual-exclusion primitive is compare-and-swap on a ref
(`git push --force-with-lease`), which tbd uses for every move: two people cannot
grab the same slot in the same race window, and the **holder** is recorded in the
annotated tag's tagger field. What a lease is **not**: a time-held lock across a
CD run. Git cannot enforce that; tbd does not pretend otherwise. (`:force` or
`tag-push: force` overrides the CAS check.)

#### Strategy: `ephemeral-branch`

Set `lease-strategy: ephemeral-branch` and list `lease-branches` instead of
`lease-tags`. The two strategies share no logic. Here a deploy slot is a
**branch** that exists ONLY while leased: every `tbd lease <name>` blows the
branch away and recreates it at your working branch's tip (CAS-guarded, `:force`
to override), so it never lingers between leases. There is no advance/no-op
gating; each lease is a fresh remake. Any tbd activity that fetches also mirrors
the remote lease-branches into local refs, so your view tracks reality. Run
`tbd lease <name>` from your feature branch, not from the lease branch itself.
`lease-strategy: none` disables leasing entirely.

## Development

```sh
go test ./...        # unit, data-driven, and property tests
bash scripts/e2e.sh  # full flow against a throwaway origin + two clones
```

Tests come in three flavors:

- **Data-driven / table tests** - e.g. `feature finish` preconditions
  (`TestFeatureFinishGuards`) and config parsing.
- **Property tests** (`pgregory.net/rapid`) over randomly generated git
  histories: the ahead/behind/diverged model matches the constructed shape, a
  rebase always restores the invariant, and any valid config survives a
  Save→Load round-trip.
- **End-to-end** - `scripts/e2e.sh` exercises the real flows, including the
  auto-rebase visualization, conflict resolution via `tbd continue`, and the
  DAG-gated lease (bootstrap, advance-through-amend, take a teammate's slot).

Git-dependent tests skip automatically when `git` is not on `PATH`.

Layout: `cmd/tbd` (entrypoint) · `internal/cli` (dispatch) · `internal/config`
· `internal/git` (the only place that shells out to git) · `internal/invariant`
(the guard) · `internal/render` (color + the rebase graph) · `internal/commands`.
