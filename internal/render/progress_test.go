package render

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestStepperNonTTYTelegraphs(t *testing.T) {
	var buf bytes.Buffer
	ran := false
	err := Stepper{W: &buf, TTY: false}.Run("fetching origin", func() error {
		ran = true
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !ran {
		t.Fatal("fn should run")
	}
	if !strings.Contains(buf.String(), "fetching origin") {
		t.Fatalf("label not telegraphed: %q", buf.String())
	}
	if !strings.Contains(buf.String(), "\n") {
		t.Fatal("non-tty telegraph should be a full line")
	}
}

func TestStepperTTYErasesAndPropagates(t *testing.T) {
	var buf bytes.Buffer
	err := Stepper{W: &buf, TTY: true}.Run("pushing", func() error {
		return errors.New("boom")
	})
	if err == nil || err.Error() != "boom" {
		t.Fatalf("error should propagate, got %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "pushing") {
		t.Fatalf("label not shown: %q", out)
	}
	if !strings.Contains(out, "\x1b[K") {
		t.Fatalf("spinner line should be erased: %q", out)
	}
}

func TestStepperNilWriterStillRuns(t *testing.T) {
	ran := false
	if err := (Stepper{}).Run("x", func() error { ran = true; return nil }); err != nil {
		t.Fatal(err)
	}
	if !ran {
		t.Fatal("fn should run even with no writer")
	}
}
