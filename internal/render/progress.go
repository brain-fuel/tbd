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

// Run announces label, runs fn, and returns fn's error. On a TTY it shows an
// animated spinner and erases it when done (the command prints its own result).
// Off a TTY it prints the label once, so the procedure is still telegraphed in
// logs and CI.
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
	go func() {
		t := time.NewTicker(90 * time.Millisecond)
		defer t.Stop()
		i := 0
		for {
			select {
			case <-quit:
				close(done)
				return
			case <-t.C:
				i++
				fmt.Fprintf(s.W, "\r%s %s", s.Colors.Cyan(spinFrames[i%len(spinFrames)]), label)
			}
		}
	}()
	fmt.Fprintf(s.W, "\r%s %s", s.Colors.Cyan(spinFrames[0]), label)

	err := fn()

	close(quit)
	<-done
	fmt.Fprint(s.W, "\r\033[K") // carriage return + clear to end of line
	return err
}
