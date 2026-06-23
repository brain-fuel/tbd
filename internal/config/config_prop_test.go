package config

import (
	"path/filepath"
	"strings"
	"testing"

	"pgregory.net/rapid"
)

// TestParseStrategyRoundTrip: for any nonempty subset of the valid strategies,
// ParseStrategy of its comma-joined form yields a set whose Has() agrees with
// membership, and validation passes.
func TestParseStrategyRoundTrip(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		subset := rapid.SliceOfNDistinct(
			rapid.SampledFrom([]string{"branch", "tag"}),
			1, 2,
			func(s string) string { return s },
		).Draw(rt, "subset")

		set, err := ParseStrategy(strings.Join(subset, ","))
		if err != nil {
			rt.Fatalf("ParseStrategy(%v): %v", subset, err)
		}
		want := map[string]bool{}
		for _, s := range subset {
			want[s] = true
		}
		for _, kind := range []string{"branch", "tag"} {
			if set.Has(kind) != want[kind] {
				rt.Fatalf("Has(%q) = %v, want %v", kind, set.Has(kind), want[kind])
			}
		}
	})
}

// TestParseStrategyRejectsInvalid: any token outside {branch,tag} is rejected.
func TestParseStrategyRejectsInvalid(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		bad := rapid.StringMatching(`[a-z]{1,6}`).Filter(func(s string) bool {
			return s != "branch" && s != "tag"
		}).Draw(rt, "bad")
		if _, err := ParseStrategy(bad); err == nil {
			rt.Fatalf("expected %q to be rejected", bad)
		}
	})
}

// genConfig draws an arbitrary valid Config.
func genConfig(rt *rapid.T) Config {
	ident := rapid.StringMatching(`[a-z][a-z0-9_-]{0,12}`)
	prefix := rapid.StringMatching(`[a-z][a-z0-9_-]{0,8}/`)
	strat := rapid.SliceOfNDistinct(
		rapid.SampledFrom([]string{"branch", "tag"}), 1, 2,
		func(s string) string { return s },
	).Draw(rt, "strategy")
	leases := rapid.SliceOfDistinct(
		ident, func(s string) string { return s },
	).Draw(rt, "leases")
	auto := rapid.Bool().Draw(rt, "auto")

	return Config{
		TrunkName:           ident.Draw(rt, "trunk"),
		FeaturePrefix:       prefix.Draw(rt, "featPrefix"),
		ReleaseStrategy:     StrategySet(strat),
		ReleaseBranchPrefix: prefix.Draw(rt, "relPrefix"),
		ReleaseTagTemplate:  "v{version}",
		LeaseStrategy:       "tag",
		LeaseTags:           leases,
		Remote:              ident.Draw(rt, "remote"),
		AutoRebase:          &auto,
		TagPush:             rapid.SampledFrom([]string{"with-lease", "force"}).Draw(rt, "tagPush"),
	}
}

// TestConfigSaveLoadRoundTrip: any valid Config survives Save→Load unchanged.
func TestConfigSaveLoadRoundTrip(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		orig := genConfig(rt)
		if err := orig.Validate(); err != nil {
			rt.Fatalf("generated config should be valid: %v", err)
		}
		dir := t.TempDir()
		path := filepath.Join(dir, FileName)
		if err := orig.Save(path); err != nil {
			rt.Fatalf("save: %v", err)
		}
		got, _, err := Load(dir)
		if err != nil {
			rt.Fatalf("load: %v", err)
		}
		if got.TrunkName != orig.TrunkName ||
			got.FeaturePrefix != orig.FeaturePrefix ||
			got.ReleaseBranchPrefix != orig.ReleaseBranchPrefix ||
			got.ReleaseTagTemplate != orig.ReleaseTagTemplate ||
			got.LeaseStrategy != orig.LeaseStrategy ||
			got.Remote != orig.Remote ||
			got.TagPush != orig.TagPush ||
			got.AutoRebaseEnabled() != orig.AutoRebaseEnabled() {
			rt.Fatalf("scalar mismatch:\n got %+v\nwant %+v", got, orig)
		}
		if !sameSet(got.ReleaseStrategy, orig.ReleaseStrategy) {
			rt.Fatalf("strategy mismatch: %v vs %v", got.ReleaseStrategy, orig.ReleaseStrategy)
		}
		if !sameSlice(got.LeaseTags, orig.LeaseTags) {
			rt.Fatalf("lease-tags mismatch: %v vs %v", got.LeaseTags, orig.LeaseTags)
		}
	})
}

func sameSet(a, b StrategySet) bool { return sameSlice([]string(a), []string(b)) }

func sameSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
