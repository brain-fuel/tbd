package render

import (
	"fmt"
	"io"
	"time"
)

var spinFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// Stepper telegraphs the procedure tbd is about to run and, on a terminal,
// animates a spinner for its duration. Progress is meant for stderr so stdout
// stays clean for piping.
type Stepper struct {
	W      io.Writer
	TTY    bool
	Colors Colors
}

// spinDelay is how long an operation must run before the spinner appears. Fast
// operations finish first and show nothing, so the spinner only shows up when it
// is actually needed.
const spinDelay = 150 * time.Millisecond

// Run announces label, runs fn, and returns fn's error. On a TTY it shows an
// animated spinner ONLY if fn runs longer than spinDelay, erasing it when done
// (the command prints its own result). Off a TTY it prints the label once, so
// the procedure is still telegraphed in logs and CI.
func (s Stepper) Run(label string, fn func() error) error {
	if s.W == nil {
		return fn()
	}
	if !s.TTY {
		fmt.Fprintf(s.W, "%s %s\n", s.Colors.Dim("..."), label)
		return fn()
	}

	quit := make(chan struct{})
	done := make(chan struct{})
	painted := false
	go func() {
		defer close(done)
		timer := time.NewTimer(spinDelay)
		defer timer.Stop()
		select {
		case <-quit:
			return // finished before the delay: never paint
		case <-timer.C:
		}
		painted = true
		t := time.NewTicker(90 * time.Millisecond)
		defer t.Stop()
		i := 0
		for {
			fmt.Fprintf(s.W, "\r%s %s", s.Colors.Cyan(spinFrames[i%len(spinFrames)]), label)
			select {
			case <-quit:
				return
			case <-t.C:
				i++
			}
		}
	}()

	err := fn()

	close(quit)
	<-done
	if painted {
		fmt.Fprint(s.W, "\r\033[K") // carriage return + clear to end of line
	}
	return err
}
