package cli

import "goforge.dev/tbd/v2/internal/argv"

// Args and Parse are re-exported from the argv parser library so existing
// callers keep working while the parsing/validation logic lives in one place.
type Args = argv.Args

// Parse splits a raw argument vector into a command and its arguments.
func Parse(a []string) Args { return argv.Parse(a) }
