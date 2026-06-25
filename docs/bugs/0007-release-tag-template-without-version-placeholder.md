# Bug 0007: a `release-tag-template` without `{version}` silently allows only one release, then blocks every future one

- **Component:** `internal/commands` (`runRelease`)
- **Affected command:** `tbd release cut VERSION` (tag strategy)
- **Version:** tbd 1.11.3 (commit 4adee34)
- **Severity:** low — misconfiguration trap; no data loss, but every release after the first fails with a confusing "tag already exists"
- **Status:** FIXED, verified (see Verification)

## Summary

The tag name is built by substituting `{version}` into `release-tag-template`.
If the template contains no `{version}` (e.g. a typo, or `RELEASE`), every
release resolves to the *same* tag. The first `release cut` succeeds; every
later one fails with `release tag "RELEASE" already exists`, which does not hint
that the template is the problem. tbd never validates that a tag-strategy
template actually varies per version.

## Environment

- OS: Linux 6.18 (WSL2); system git
- tbd built with `go build -o tbd ./cmd/tbd` at commit 4adee34 (v1.11.3)

## Steps to reproduce

```sh
W=$(mktemp -d); cd "$W"; git init -q -b develop r; cd r
git config user.name dev; git config user.email dev@x
echo s > s; git add .; git commit -qm seed
printf 'trunk-name: develop\nrelease-strategy: tag\nrelease-tag-template: RELEASE\n' > .tbd.yaml
git add -A; git commit -qm cfg
tbd release cut 1.0.0 :local
tbd release cut 2.0.0 :local
```

## Actual result (before fix)

```
✓ created release tag RELEASE
...
tbd release: release tag "RELEASE" already exists
```

The second (and every future) release is blocked, with no indication that the
template is missing `{version}`.

## Expected result

`tbd release cut` rejects a tag-strategy template that lacks `{version}` up
front, naming the real problem:

```
tbd release: release-tag-template "RELEASE" has no {version} placeholder; every release would collide on the tag "RELEASE"
```

## Root cause

`runRelease` substituted `{version}` into the template and used the result
without checking that the substitution actually occurred (i.e. that the template
contained the placeholder).

## Fix

In `runRelease`, when the strategy includes `tag`, reject a template that does
not contain `{version}` before doing any work. (Only enforced for the tag
strategy; a branch-only release never consults the tag template.)

## Verification

- Manual: the reproduction now fails on the FIRST `release cut` with the clear
  message above; a template containing `{version}` still cuts normally.
- Automated: `TestReleaseRejectsTemplateWithoutVersion` and the existing
  `TestReleaseCutValidVersion`.
- `go vet ./...`, full `go test ./...`, and `scripts/e2e.sh` all pass.
