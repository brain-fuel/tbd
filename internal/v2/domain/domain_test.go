package domain

import "testing"

func TestNames(t *testing.T) {
	if WorkKindName(Feature{}) != "feature" || GroupKindName(Stack{}) != "stack" {
		t.Fatal("unexpected domain names")
	}
}
