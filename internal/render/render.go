// Package render provides terminal output helpers: optional ANSI color, simple
// aligned tables, and an ASCII commit-graph used to visualize rebases. Color is
// disabled when color-mode:none is passed or when output is not a terminal.
package render

import (
	"fmt"
	"strings"
)

// Colors holds whether ANSI styling is enabled.
type Colors struct{ enabled bool }

// NewColors decides whether to colorize. colorMode of "none" forces plain
// output; "always" forces color; otherwise color follows isTTY.
func NewColors(colorMode string, isTTY bool) Colors {
	switch colorMode {
	case "none":
		return Colors{enabled: false}
	case "always":
		return Colors{enabled: true}
	}
	return Colors{enabled: isTTY}
}

// Enabled reports whether ANSI styling is on.
func (c Colors) Enabled() bool { return c.enabled }

func (c Colors) wrap(code, s string) string {
	if !c.enabled {
		return s
	}
	return "\x1b[" + code + "m" + s + "\x1b[0m"
}

func (c Colors) Red(s string) string     { return c.wrap("31", s) }
func (c Colors) Green(s string) string   { return c.wrap("32", s) }
func (c Colors) Yellow(s string) string  { return c.wrap("33", s) }
func (c Colors) Cyan(s string) string    { return c.wrap("36", s) }
func (c Colors) Magenta(s string) string { return c.wrap("35", s) }
func (c Colors) Bold(s string) string    { return c.wrap("1", s) }
func (c Colors) Dim(s string) string     { return c.wrap("2", s) }

// Table renders rows of cells into a left-aligned, space-padded table.
type Table struct {
	header []string
	rows   [][]string
}

// NewTable creates a table with the given header.
func NewTable(header ...string) *Table { return &Table{header: header} }

// Add appends a row.
func (t *Table) Add(cells ...string) { t.rows = append(t.rows, cells) }

// String renders the table. Width is computed from visible (un-styled) length,
// so cells may carry ANSI color and still align.
func (t *Table) String() string {
	cols := len(t.header)
	for _, r := range t.rows {
		if len(r) > cols {
			cols = len(r)
		}
	}
	width := make([]int, cols)
	consider := func(r []string) {
		for i, c := range r {
			if n := visibleLen(c); n > width[i] {
				width[i] = n
			}
		}
	}
	if len(t.header) > 0 {
		consider(t.header)
	}
	for _, r := range t.rows {
		consider(r)
	}

	var b strings.Builder
	writeRow := func(r []string) {
		for i := 0; i < cols; i++ {
			cell := ""
			if i < len(r) {
				cell = r[i]
			}
			pad := width[i] - visibleLen(cell)
			b.WriteString(cell)
			if i < cols-1 {
				b.WriteString(strings.Repeat(" ", pad+2))
			}
		}
		b.WriteByte('\n')
	}
	if len(t.header) > 0 {
		writeRow(t.header)
	}
	for _, r := range t.rows {
		writeRow(r)
	}
	return b.String()
}

// visibleLen returns the display width of s, ignoring ANSI escape sequences.
func visibleLen(s string) int {
	n, inEsc := 0, false
	for _, r := range s {
		switch {
		case r == '\x1b':
			inEsc = true
		case inEsc && r == 'm':
			inEsc = false
		case inEsc:
			// skip
		default:
			n++
		}
	}
	return n
}

// Plain is a convenience for fmt.Sprintf used by callers.
func Plain(format string, a ...any) string { return fmt.Sprintf(format, a...) }
