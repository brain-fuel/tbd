#!/usr/bin/env bash
# End-to-end smoke test for the v2 Cobra CLI and Go+ workflow core.
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
bin_dir="$(mktemp -d)"
work="$(mktemp -d)"
trap 'rm -rf "$bin_dir" "$work"' EXIT
bin="$bin_dir/tbd"
go build -o "$bin" "$here/cmd/tbd"

gitc() { git -c user.email=e2e@example.com -c user.name=e2e -c commit.gpgsign=false "$@"; }
origin="$work/origin.git"
repo="$work/repo"

echo "== repository and v2 config =="
gitc init -q --bare -b main "$origin"
gitc clone -q "$origin" "$repo"
cd "$repo"
git config user.email e2e@example.com
git config user.name e2e
git config commit.gpgsign false
git config tag.gpgsign false
echo seed > seed.txt
gitc add seed.txt
gitc commit -q -m seed
gitc push -q -u origin main
"$bin" init --yes
gitc add .tbd.yaml
gitc commit -q -m "configure tbd"
gitc push -q origin main

echo "== typed feature and atomic workflow state =="
"$bin" feature --id E2E-1 --desc "durable release"
echo work > feature.txt
"$bin" commit --message "feat: durable release" --no-edit
test -f .tbd/state.json
gitc merge-base --is-ancestor origin/main HEAD

echo "== observed deploy lease =="
"$bin" lease dev-deploy
gitc ls-remote --tags origin dev-deploy | grep -q dev-deploy

echo "== release candidate and durable completion =="
"$bin" release rc 1.2.3
gitc ls-remote --tags origin rc-1.2.3 | grep -q rc-1.2.3
"$bin" release complete 1.2.3
gitc ls-remote --tags origin v1.2.3 | grep -q v1.2.3
test -f RELEASE.json
test -f RELEASE.md
test -f .git/tbd-workflows/release-1.2.3.json
grep -q '"Done": true' .git/tbd-workflows/release-1.2.3.json

echo "== SemVer boundary =="
if "$bin" release complete not-a-version >invalid.out 2>&1; then
	echo "invalid version unexpectedly succeeded"
	exit 1
fi
grep -q 'invalid semantic version' invalid.out

echo "e2e v2 workflow passed"
