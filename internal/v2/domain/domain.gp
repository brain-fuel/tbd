// Package domain contains tbd's closed workflow vocabulary. It is authored in
// Go+ so additions require exhaustive handling by semantic reducers.
package domain

type WorkKind enum { Feature(); Fix() }
type GroupKind enum { Collaboration(); Stack() }
type ItemStatus enum { Started(); Aggregated(kind GroupKind); Finished() }
type ReleaseEventKind enum { Candidate(); Released(); Invalidated() }
type RefStrategy enum { Tag(); Branch(prefix string) }
type PushPolicy enum { ForceWithLease(); Force() }
type UATStatus enum { Pending(); Valid(candidate string, commit string); Invalid(reason string) }
type LeaseState enum { Unset(); Current(commit string); Stale(previous string, destination string); Foreign(commit string) }

func WorkKindName(k WorkKind) string {
	return match k { case Feature(): "feature"; case Fix(): "fix" }
}

func GroupKindName(k GroupKind) string {
	return match k { case Collaboration(): "collab"; case Stack(): "stack" }
}

func ItemStatusName(s ItemStatus) string {
	return match s { case Started(): "started"; case Aggregated(k): GroupKindName(k); case Finished(): "finished" }
}

func ReleaseEventKindName(k ReleaseEventKind) string {
	return match k { case Candidate(): "rc"; case Released(): "release"; case Invalidated(): "invalidated" }
}
