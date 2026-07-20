# tbd

`tbd` is a workflow-aware Git CLI for trunk-based delivery. Version 2 uses
Cobra flags only; the old colon argument syntax is intentionally gone.

The core rule is still simple: deployable work must be rebased onto the current
configured trunk before it moves through deploy or release refs.

## v2.3.2 — Go+ Workflow Semantics

The semantic core is now authored in Go+. Closed enums define work kinds,
group kinds, item states, release events, push policy, UAT state, and lease
classification, while generated Go preserves the existing JSON wire format.

Release completion is a durable three-step workflow: record metadata, publish
the release tag through an observed compare-and-swap, then advance trunk. A
crash or rejected push resumes at the first incomplete step instead of
repeating completed effects. Configuration uses defaults plus path-aware
validation, release versions use strict SemVer, hooks retain structured process
failures, and state files use crash-safe atomic replacement.

Regenerate the checked-in Go facade after editing `.gp` sources:

```sh
go generate ./...
```

## Install

```sh
go build -o tbd ./cmd/tbd
```

## Quick Start

```sh
tbd init --yes
tbd feature --id JIRA-123 --desc "Add login"
# edit files
tbd commit
tbd lease dev-deploy
tbd release rc 1.2.3
tbd release complete 1.2.3
```

`tbd commit` stages all changes, keeps the branch to one commit, fast-forwards
the local trunk from the remote when possible, rebases onto trunk, and amends tbd
workflow metadata into that same commit.

For generic Git graph surgery:

```sh
tbd sr
tbd sr another-branch
tbd squash-rebase
```

## Defaults

`tbd init --yes` writes `.tbd.yaml` with these defaults:

```yaml
version: 2
trunk-name: main
remote: origin
auto-rebase: true
rerere: true
branches:
  feature-template: feature/{id}-{slug}
  fix-template: bugfix/{id-}{slug}
  collab-suffix: -collab
  stack-suffix: -stack
release:
  strategy: tag
  branch-prefix: release/
  tag-template: v{semver}
  rc-tag-template: rc-{semver}
  bad-tag-template: bad-{timestamp}
  default-revert-bump: patch
  delete-remote-rc-on-uat-reset: true
deploy:
  strategy: tag
  refs: [dev-deploy, test-deploy, prod-deploy]
push:
  branch: force-with-lease
  tag: force-with-lease
locks:
  ref-prefix: refs/tbd/locks/
  default-ttl: 3h
```

Use branch mode when a CD system only watches branches:

```sh
tbd init --yes \
  --release-strategy branch \
  --deploy-strategy branch \
  --deploy-ref deploy-dev \
  --deploy-ref deploy-uat \
  --deploy-ref prod-deploy
```

In branch release mode, deploy branches are mutable lease refs, but
`release/<semver>` branches are immutable. A successful release creates
`v<semver>` and fast-forwards the configured trunk branch to the release commit.

## Workflows

Feature:

```sh
tbd feature --id JIRA-123 --desc "Add login"
```

Fix:

```sh
tbd fix --desc "Patch production login"
tbd fix --id JIRA-456 --desc "Patch production login"
```

Collab creates one aggregate branch and tracks each feature in metadata and
release notes:

```sh
tbd collab --id JIRA-1 --desc "Feature one" --id JIRA-2 --desc "Feature two"
tbd collab add --id JIRA-3 --desc "Feature three"
```

Stack creates one branch with at most one commit per issue. Touching an item
moves it to the top of the stack metadata, and `tbd stack remove --id JIRA-2`
removes it and rebuilds from remaining recorded commits where possible.

```sh
tbd stack --id JIRA-1 --desc "Feature one" --id JIRA-2 --desc "Feature two"
tbd stack add --id JIRA-1 --desc "Feature one"
tbd stack remove --id JIRA-2
```

`git rerere.enabled=true` is set by default during init because stack/collab
rebases deliberately reuse conflict resolutions.

## Releases

Tag strategy:

```sh
tbd release rc 1.2.3
tbd release complete 1.2.3
```

`release rc` deletes/replaces `rc-<semver>` locally and remotely. The RC tag only
exists when the candidate is rebased onto the current trunk and marked good.

Branch strategy:

```sh
tbd release prepare 1.2.3
tbd release complete 1.2.3
```

`release prepare` creates immutable `release/<semver>`. `release complete` tags
`v<semver>` and fast-forwards trunk to that release commit. Numbers only go up;
bad commits are marked with `tbd tag <ref> bad`, which creates `bad-<timestamp>`.

Release metadata is written to `RELEASE.json` and rendered to `RELEASE.md`.
Draft notes can move with the same topology as the commits while a feature set is
still being worked.

## Reverts

```sh
tbd revert --ref <sha-or-tag-or-version> --why "remove feature"
tbd revert --all-past v1.2.3 --why "rollback to last good production"
tbd revert --ref JIRA-123 --minor --why "remove larger feature"
```

Before a production/RC tag exists, feature removal does not force a semver bump.
After an RC or production tag exists, revert metadata defaults to a patch bump
unless `--minor` or `--major` is passed.

## Deploy Leases

```sh
tbd lease dev-deploy
tbd steal dev-deploy
tbd relinquish dev-deploy
tbd lease deploy-uat --to HEAD
```

Deploy refs are tags or branches depending on `deploy.strategy`, but the
semantics are identical: the ref is moved by compare-and-swap where a remote is
available. `lease` borrows an unheld deploy mutex or advances your own held
mutex. It refuses to silently take a ref held by another branch. `steal` is the
explicit takeover verb. `relinquish` releases your held mutex by moving the
deploy ref back to the current trunk head. Do not work while checked out on
deploy branches; `tbd` refuses those operations.

## Locks

```sh
tbd lock acquire uat
tbd lock status uat
tbd lock acquire uat --steal
tbd lock release uat
```

Locks are stored under `refs/tbd/locks/<name>` as metadata commits and pushed
with CAS. The default TTL is three hours. Expired locks still require `--steal`
so taking ownership is explicit.

See [docs/locks.md](docs/locks.md) for raw Git recovery commands.

## Visualization

Terminal:

```sh
tbd graph
tbd graph --limit 200
```

Browser:

```sh
tbd serve --addr 127.0.0.1:8087
```

The server fetches on an interval, exposes `/api/graph` with commits, parent
edges, refs, and tbd workflow state, and serves a Go/WASM SVG visualizer inspired
by LearnGitBranching. The command console runs in the repository as a local
daemon; it accepts quoted arguments and can run `tbd ...` or explicit `git ...`
commands. The legacy `/graph` endpoint still returns the terminal DAG text. The
WASM renderer is compiled on first request with the local Go toolchain, so source
checkouts need `go` on `PATH` when serving the browser UI.

Demo simulation:

```sh
tbd demo
tbd demo stack
tbd demo collab
tbd demo --addr 127.0.0.1:8088 --dir .tbd-demo --no-open
```

The demo creates a marked workspace with one bare remote and four local clones
for Ada, Ben, Cy, and Dee. `tbd demo` runs the basic path with four independent
feature branches. `tbd demo stack` runs one standalone feature plus three
three-item stack workflows. `tbd demo collab` runs one standalone feature plus
three three-item collab workflows. The browser shows the four repos as a 2x2
desktop grid and only stacks them vertically on narrow screens. A toolbar zoom
control keeps the graphs readable without losing access to the controls, and
each step animates the repos and command logs it touched. Every demo uses real
`tbd` commands and moves deployable work through `dev-deploy`, then `qa-deploy`,
then `prod-deploy`, including amend/rebase behavior, lease refusal, explicit
steal/recovery, relinquish-to-trunk, RCs, and production releases. The demo
directory is deleted on normal shutdown and recreated fresh on the next run.

## Development

```sh
go test ./...
```

The legacy `internal/commands`, `internal/cli`, and colon parser packages remain
in the repository for regression coverage and reusable Git behavior. The binary
entrypoint is the Cobra V2 app in `internal/app`.
