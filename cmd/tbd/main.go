// Command tbd is a trunk-based development wrapper over git's DAG. Its central
// rule: before any mutating operation, the head of the trunk must be an ancestor
// of the ref being operated on or produced.
package main

import (
	"goforge.dev/tbd/internal/app"
)

func main() {
	app.Main()
}
