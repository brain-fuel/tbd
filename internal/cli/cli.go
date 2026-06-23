// Package cli provides tbd's command dispatcher: a small registry that commands
// join from their init() functions, plus the Context handed to each handler.
package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"sort"

	"goforge.dev/tbd/internal/render"
)

// ExitError lets a command set the process exit code without the dispatcher
// printing an error message (the command has already reported its own output).
type ExitError struct{ Code int }

func (e ExitError) Error() string { return fmt.Sprintf("exit status %d", e.Code) }

// Context carries everything a command handler needs: the parsed arguments,
// output streams, and the directory from which tbd was invoked.
type Context struct {
	Args   Args
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
	Dir    string
	IsTTY  bool
}

// Colors returns a colorizer honoring the color-mode argument and TTY status.
func (c *Context) Colors() render.Colors {
	return render.NewColors(c.Args.GetOr("color-mode", ""), c.IsTTY)
}

// Command is a registered tbd subcommand.
type Command struct {
	Name    string
	Summary string
	Usage   string
	Run     func(*Context) error
}

var registry = map[string]*Command{}

// Register adds a command to the dispatch table. Commands call this from init().
func Register(c *Command) {
	if _, dup := registry[c.Name]; dup {
		panic("cli: duplicate command " + c.Name)
	}
	registry[c.Name] = c
}

// Lookup returns a registered command by name.
func Lookup(name string) (*Command, bool) {
	c, ok := registry[name]
	return c, ok
}

// Commands returns all registered commands sorted by name.
func Commands() []*Command {
	out := make([]*Command, 0, len(registry))
	for _, c := range registry {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Run parses argv, builds a Context, and dispatches to the matching command.
// It returns a process exit code.
func Run(argv []string) int {
	args := Parse(argv)
	dir, _ := os.Getwd()
	ctx := &Context{Args: args, Stdin: os.Stdin, Stdout: os.Stdout, Stderr: os.Stderr, Dir: dir, IsTTY: isTerminal(os.Stdout)}

	switch args.Command {
	case "", "help":
		return runHelp(ctx)
	}

	cmd, ok := registry[args.Command]
	if !ok {
		fmt.Fprintf(ctx.Stderr, "tbd: unknown command %q (try \"tbd help\")\n", args.Command)
		return 2
	}
	if err := cmd.Run(ctx); err != nil {
		var ee ExitError
		if errors.As(err, &ee) {
			return ee.Code
		}
		fmt.Fprintf(ctx.Stderr, "tbd %s: %v\n", cmd.Name, err)
		return 1
	}
	return 0
}

// isTerminal reports whether w is a character device (a terminal).
func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// runHelp prints top-level usage or, when a command name is given as the first
// positional ("tbd help feature"), that command's usage.
func runHelp(ctx *Context) int {
	if topic := ctx.Args.Pos(0); topic != "" {
		if cmd, ok := registry[topic]; ok {
			fmt.Fprintf(ctx.Stdout, "%s — %s\n\n%s\n", cmd.Name, cmd.Summary, cmd.Usage)
			return 0
		}
		fmt.Fprintf(ctx.Stderr, "tbd: no help for unknown command %q\n", topic)
		return 2
	}
	fmt.Fprintln(ctx.Stdout, "tbd — trunk-based development over git's DAG")
	fmt.Fprintln(ctx.Stdout, "\nUsage:\n  tbd <command> [positional...] key:value :flag\n\nCommands:")
	for _, c := range Commands() {
		fmt.Fprintf(ctx.Stdout, "  %-10s %s\n", c.Name, c.Summary)
	}
	fmt.Fprintln(ctx.Stdout, "\nRun \"tbd help <command>\" for command details.")
	return 0
}
