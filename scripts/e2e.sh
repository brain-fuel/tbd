#!/usr/bin/env bash
# End-to-end smoke test for tbd: stand up a bare "origin" plus two clones and
# exercise the feature, release, and lease flows including the auto-rebase
# visualization and a real compare-and-swap lease rejection.
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

echo "== lease take from c1 =="
"$bin" lease take dev-deploy
"$bin" lease status

echo "== lease CAS: c2 takes it, then stale c1 take is rejected =="
cd "$work/c2"
gitc pull -q --ff-only origin develop
cp "$work/c1/.tbd.yaml" .tbd.yaml 2>/dev/null || true
"$bin" lease status            # fetches tags so c2 has seen dev-deploy
"$bin" lease take dev-deploy   # c2 now holds it (remote moves)

cd "$work/c1"
# c1 is stale: it still thinks dev-deploy is where it left it. CAS must reject.
if "$bin" lease take dev-deploy 2>"$work/rej.txt"; then
  echo "FAIL: stale lease take should have been rejected"; cat "$work/rej.txt"; exit 1
fi
grep -q "taken by someone else" "$work/rej.txt" || { echo "FAIL: expected rejection message"; cat "$work/rej.txt"; exit 1; }
echo "lease correctly rejected the stale take"

echo "== status / version / config =="
"$bin" status
"$bin" version
"$bin" config list

echo "e2e OK"
