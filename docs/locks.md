# tbd Locks With Raw Git

`tbd lock` stores locks as commits under refs like:

```sh
refs/tbd/locks/uat
```

The lock commit message is JSON metadata: owner, email, host, acquired time,
expiry time, and the repository commit that was current when the lock was taken.

## Inspect A Lock

```sh
git fetch origin 'refs/tbd/locks/*:refs/tbd/locks/*'
git log -1 --format=%B refs/tbd/locks/uat
```

## Release A Local Lock Ref

```sh
git update-ref -d refs/tbd/locks/uat
```

## Delete The Remote Lock

```sh
git push origin :refs/tbd/locks/uat
```

## Manually Steal A Lock

Create a metadata commit and compare-and-swap the remote ref:

```sh
empty_tree=4b825dc642cb6eb9a060e54bf8d69288fbee4904
old=$(git ls-remote origin refs/tbd/locks/uat | awk '{print $1}')
new=$(
  git commit-tree "$empty_tree" -m '{
    "name": "uat",
    "owner": "Your Name",
    "email": "you@example.com",
    "host": "manual",
    "commit": "'"$(git rev-parse HEAD)"'",
    "acquired": "'"$(date -u +%Y-%m-%dT%H:%M:%SZ)"'",
    "expires": "'"$(date -u -d "+3 hours" +%Y-%m-%dT%H:%M:%SZ)"'"
  }'
)
git update-ref refs/tbd/locks/uat "$new"
git push --atomic --force-with-lease=refs/tbd/locks/uat:"$old" \
  origin refs/tbd/locks/uat:refs/tbd/locks/uat
```

If another user moved the lock between `ls-remote` and `push`, Git rejects the
push. Fetch and inspect again before deciding whether to steal.

