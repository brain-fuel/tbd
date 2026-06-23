#!/usr/bin/env bash
# End-to-end smoke test for tbd: stand up a bare "origin" plus two clones and
# exercise the feature, release, and lease flows including the auto-rebase
# visualization, conflict resolution via tbd continue, and the DAG-gated lease
# (bootstrap, advance-through-amend, and taking a teammate's slot).
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
bin="$(mktemp -d)/tbd"
go build -o "$bin" "$here/cmd/tbd"

work="$(mktemp -d)"
origin="$work/origin.git"

# Hermetic git identity for every repo we create.
gitc() { git -c user.email=e2e@example.com -c user.name=e2e -c commit.gpgsign=false "$@"; }

echo "== set up bare origin + clone c1 =="
gitc init -q --bare -b develop "$origin"
gitc clone -q "$origin" "$work/c1"
cd "$work/c1"
# Repo-local identity so tbd's own git calls (annotated lease tags) work on a
# clean CI runner with no global git user. c1 and c2 are distinct people, which
# is the whole point of a lease.
git config user.email alice@e2e.example
git config user.name alice
git config commit.gpgsign false
git config tag.gpgsign false
echo "seed" > seed.txt
gitc add -A
gitc commit -q -m "seed"
gitc push -q -u origin develop

echo "== init =="
"$bin" init lease-tags:dev-deploy,uat1-deploy :force
gitc add .tbd.yaml
gitc commit -q -m "add tbd config"
gitc push -q origin develop

echo "== feature start / commit / guard / finish (clean ff) =="
"$bin" feature start login
echo "login" > login.txt
gitc add -A
gitc commit -q -m "login work"
"$bin" guard            # expect: invariant holds
"$bin" feature finish
gitc rev-parse origin/develop >/dev/null

echo "== force divergence: c1 starts a feature, THEN c2 advances trunk =="
# c1 forks the feature from the current trunk...
cd "$work/c1"
"$bin" feature start widget
echo "widget" > widget.txt
gitc add -A
gitc commit -q -m "widget work"

# ...then c2 lands a commit on trunk, so the feature is now behind.
gitc clone -q "$origin" "$work/c2"
cd "$work/c2"
gitc config user.email bob@e2e.example
gitc config user.name bob
gitc config commit.gpgsign false
gitc config tag.gpgsign false
echo "hotfix" > hotfix.txt
gitc add -A
gitc commit -q -m "trunk advances"
gitc push -q origin develop

cd "$work/c1"
# origin/develop is now ahead of where widget forked; finish should visibly rebase.
out="$("$bin" feature finish 2>&1)"
echo "$out"
echo "$out" | grep -q "Rebasing" || { echo "FAIL: expected rebase visualization"; exit 1; }
echo "$out" | grep -q "before" || { echo "FAIL: expected before/after DAG"; exit 1; }

echo "== tbd commit: one commit per feature, amend + rebase every time =="
"$bin" feature start patch
echo "patch a" > patch_a.txt
"$bin" commit message:"patch work"
echo "patch b" > patch_b.txt
"$bin" commit                       # amends, still a single commit
fork="$(gitc merge-base origin/develop feature/patch)"
n="$(gitc rev-list --count "$fork"..feature/patch)"
[ "$n" = "1" ] || { echo "FAIL: expected 1 commit on feature/patch, got $n"; exit 1; }
gitc merge-base --is-ancestor origin/develop feature/patch || { echo "FAIL: feature/patch not on trunk head"; exit 1; }
echo "feature/patch is exactly one commit on top of trunk"
# reword via :edit (drive the editor non-interactively)
GIT_EDITOR='printf "reworded patch\n" >' "$bin" commit :edit
gitc log -1 --format=%s feature/patch | grep -q "reworded patch" || { echo "FAIL: :edit did not reword"; exit 1; }
[ "$(gitc rev-list --count "$(gitc merge-base origin/develop feature/patch)"..feature/patch)" = "1" ] || { echo "FAIL: :edit changed commit count"; exit 1; }
echo "commit :edit reworded the message, still one commit"

echo "== feature push: publish the branch, then re-publish after an amend =="
"$bin" feature push
gitc ls-remote --heads origin feature/patch | grep -q feature/patch || { echo "FAIL: feature/patch not on origin"; exit 1; }
echo "patch c" > patch_c.txt
"$bin" commit                       # amends -> rewrites history
"$bin" feature push                 # force-with-lease must still succeed
remote_sha="$(gitc ls-remote origin refs/heads/feature/patch | awk '{print $1}')"
local_sha="$(gitc rev-parse feature/patch)"
[ "$remote_sha" = "$local_sha" ] || { echo "FAIL: origin feature/patch out of sync after amend"; exit 1; }
echo "feature/patch published and updated via force-with-lease"

"$bin" feature finish

echo "== release cut (branch + tag), verify on trunk =="
"$bin" release cut 1.0.0 strategy:branch,tag
gitc rev-parse origin/release/1.0.0 >/dev/null
gitc ls-remote --tags origin v1.0.0 | grep -q v1.0.0 || { echo "FAIL: release tag missing on origin"; exit 1; }
"$bin" release list

echo "== lease: bootstrap -> advance -> advance-through-amend =="
cd "$work/c1"
"$bin" feature start deployer
echo "deploy a" > deploy_a.txt
"$bin" commit message:"deploy work"
"$bin" lease dev-deploy                       # unset -> bootstrap to trunk head
boot="$(gitc rev-parse dev-deploy^{commit})"
[ "$boot" = "$(gitc rev-parse origin/develop)" ] || { echo "FAIL: bootstrap not on trunk head"; exit 1; }
"$bin" lease dev-deploy                       # on trunk (ancestor of W) -> advance to feature tip
[ "$(gitc rev-parse dev-deploy^{commit})" = "$(gitc rev-parse feature/deployer)" ] || { echo "FAIL: lease did not advance to feature tip"; exit 1; }
echo "deploy b" > deploy_b.txt
"$bin" commit                                 # amend -> rewrites tip, orphans old
"$bin" lease dev-deploy                       # recognizes old via reflog -> advance to amended tip
[ "$(gitc rev-parse dev-deploy^{commit})" = "$(gitc rev-parse feature/deployer)" ] || { echo "FAIL: lease did not advance through amend"; exit 1; }
echo "lease advanced through commit and amend"
"$bin" lease status

echo "== argument validation: unknown option is a helpful error =="
if "$bin" lease dev-deploy strategy:random 2>"$work/argerr.txt"; then
  echo "FAIL: strategy:random should be rejected"; exit 1
fi
grep -q "unknown option" "$work/argerr.txt" || { echo "FAIL: missing unknown-option message"; cat "$work/argerr.txt"; exit 1; }
grep -q "lease-strategy" "$work/argerr.txt" || { echo "FAIL: missing config hint"; cat "$work/argerr.txt"; exit 1; }
echo "unknown option rejected with guidance"

echo "== lease: take from someone else's branch =="
# c2 (a teammate) deploys its own feature, taking the slot.
cd "$work/c2"
gitc fetch -q origin
gitc switch -q develop
gitc reset --hard -q origin/develop
cp "$work/c1/.tbd.yaml" .tbd.yaml 2>/dev/null || true
"$bin" feature start mate
echo "mate work" > mate.txt
"$bin" commit message:"mate work"
"$bin" lease dev-deploy                       # foreign (on c1's branch) -> take to c2's tip
[ "$(gitc rev-parse dev-deploy^{commit})" = "$(gitc rev-parse feature/mate)" ] || { echo "FAIL: foreign take did not move to teammate tip"; exit 1; }
echo "teammate took the deploy lease to their branch"
cd "$work/c1"
"$bin" feature finish

echo "== ephemeral-branch lease (separate repo, distinct from the tag strategy) =="
origin2="$work/origin2.git"
gitc init -q --bare -b develop "$origin2"
gitc clone -q "$origin2" "$work/d1"
cd "$work/d1"
git config user.email d1@e2e.example
git config user.name d1
git config commit.gpgsign false
echo seed > s.txt; gitc add -A; gitc commit -q -m seed; gitc push -q -u origin develop
"$bin" init lease-strategy:ephemeral-branch lease-branches:deploy-now :force
gitc add .tbd.yaml; gitc commit -q -m cfg; gitc push -q origin develop
"$bin" feature start svc
echo "svc a" > svc_a.txt
"$bin" commit message:"svc work"
"$bin" lease deploy-now
tip="$(gitc rev-parse feature/svc)"
[ "$(gitc ls-remote origin refs/heads/deploy-now | awk '{print $1}')" = "$tip" ] || { echo "FAIL: ephemeral not remade at tip"; exit 1; }
if gitc ls-remote --tags origin deploy-now | grep -q deploy-now; then echo "FAIL: deploy-now must be a branch, not a tag"; exit 1; fi
echo "svc b" > svc_b.txt
"$bin" commit
tip2="$(gitc rev-parse feature/svc)"
"$bin" lease deploy-now
[ "$(gitc ls-remote origin refs/heads/deploy-now | awk '{print $1}')" = "$tip2" ] || { echo "FAIL: ephemeral not remade after amend"; exit 1; }
echo "ephemeral lease remade the branch at tip, again after amend, never a tag"
"$bin" lease status

echo "== conflict + tbd continue =="
cd "$work/c1"
"$bin" feature start conf
printf 'feature\n' > conflict.txt
"$bin" commit message:"conf work"
# A teammate adds the same file on trunk, so the next rebase must conflict.
cd "$work/c2"
gitc fetch -q origin
gitc switch -q develop
gitc reset --hard -q origin/develop
printf 'trunk\n' > conflict.txt
gitc add -A
gitc commit -q -m "trunk adds conflict.txt"
gitc push -q origin develop
cd "$work/c1"
set +e
"$bin" commit
rc=$?
set -e
[ "$rc" -ne 0 ] || { echo "FAIL: expected commit rebase to conflict"; exit 1; }
set +e
"$bin" continue
crc=$?
set -e
[ "$crc" -ne 0 ] || { echo "FAIL: continue should refuse with unresolved conflict"; exit 1; }
printf 'resolved\n' > conflict.txt
gitc add conflict.txt
"$bin" continue
fork="$(gitc merge-base origin/develop feature/conf)"
n="$(gitc rev-list --count "$fork"..feature/conf)"
[ "$n" = "1" ] || { echo "FAIL: expected 1 commit after continue, got $n"; exit 1; }
gitc merge-base --is-ancestor origin/develop feature/conf || { echo "FAIL: not on trunk after continue"; exit 1; }
echo "conflict resolved via tbd continue; single commit on trunk"
"$bin" feature finish

echo "== learn (walkthrough prints, runs no git) =="
"$bin" learn color-mode:none | grep -q "deploy-now" || { echo "FAIL: learn missing lease scenario"; exit 1; }
"$bin" learn topics | grep -q "lease" || { echo "FAIL: learn topics missing chapters"; exit 1; }
echo "learn walkthrough OK"

echo "== status / version / config =="
"$bin" status
"$bin" version
"$bin" config list

echo "e2e OK"
