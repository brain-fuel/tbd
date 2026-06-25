# tbd bug reports

Bug reports filed against tbd, written per the OpenSC "How to write a good bug
report" guidance (clear title, summary, environment, exact steps to reproduce,
actual vs expected, root cause, fix, verification). All listed below are fixed
and verified with regression tests.

| #    | Title                                                                 | Severity | Status          |
|------|-----------------------------------------------------------------------|----------|-----------------|
| 0001 | rebase "after" graph shows stale pre-rebase SHAs for replayed commits | high     | fixed, verified |
| 0002 | `feature start` invalid name leaks raw git porcelain (after a fetch)  | low      | fixed, verified |
| 0003 | `release cut` invalid version leaks raw git porcelain mid-operation   | low/med  | fixed, verified |
| 0004 | `continue` silently drops `feature finish` after a conflict           | medium   | fixed, verified |

Verification baseline for every entry: `go vet ./...`, `go test ./...`, and
`scripts/e2e.sh` all pass on the fixed tree.
