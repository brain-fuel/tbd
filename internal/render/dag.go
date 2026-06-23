package render

import "strings"

// Commit is one node in a rebase visualization.
type Commit struct {
	Short   string // abbreviated sha
	Subject string // first line of the commit message
}

// RebasePlan describes the move a rebase performs: the feature commits currently
// hanging off Fork are replayed on top of the trunk commits that landed since.
//
//	Trunk   = commits on trunk after Fork, up to trunk head (newest first)
//	Feature = the feature's own commits after Fork (newest first)
//	Fork    = the common ancestor the feature forked from
type RebasePlan struct {
	Feature   string // feature branch name (for labels)
	Trunk     string // trunk branch name (for labels)
	Fork      Commit
	TrunkLine []Commit
	FeatLine  []Commit
}

// Render draws a before/after ASCII graph of the rebase. The "before" picture
// shows the feature diverged from trunk; the "after" picture shows it replayed
// on top of trunk head. Colors are applied when enabled.
func (p RebasePlan) Render(c Colors) string {
	var b strings.Builder

	node := func(s string) string { return c.Cyan("●") + " " + s }
	trunkNode := func(cm Commit, label string) string {
		line := c.Cyan("●") + " " + c.Bold(cm.Short) + "  " + cm.Subject
		if label != "" {
			line += "  " + c.Dim(label)
		}
		return line
	}
	featNode := func(cm Commit, label, glyph string) string {
		line := glyph + " " + c.Yellow(cm.Short) + "  " + cm.Subject
		if label != "" {
			line += "  " + c.Dim(label)
		}
		return line
	}
	_ = node

	trunkLabel := "(trunk head: " + p.Trunk + ")"
	featLabel := "← " + p.Feature

	// --- before ---
	b.WriteString(c.Bold("before") + "\n")
	for i, cm := range p.TrunkLine {
		label := ""
		if i == 0 {
			label = trunkLabel
		}
		b.WriteString("  " + trunkNode(cm, label) + "\n")
	}
	if len(p.TrunkLine) > 0 {
		b.WriteString("  " + c.Cyan("│") + "\n")
	}
	for i, cm := range p.FeatLine {
		label := ""
		if i == 0 {
			label = featLabel
		}
		b.WriteString("  " + c.Cyan("│") + " " + featNode(cm, label, c.Yellow("○")) + "\n")
	}
	if len(p.FeatLine) > 0 {
		b.WriteString("  " + c.Cyan("├─╯") + "\n")
	}
	b.WriteString("  " + c.Dim("◇") + " " + c.Dim(p.Fork.Short) + "  " + c.Dim(p.Fork.Subject+"  (fork point)") + "\n")

	// --- after ---
	b.WriteString("\n" + c.Bold("after") + "\n")
	for i, cm := range p.FeatLine {
		label := ""
		if i == 0 {
			label = featLabel + " (replayed)"
		}
		b.WriteString("  " + featNode(cm, label, c.Green("●")) + "\n")
	}
	for i, cm := range p.TrunkLine {
		label := ""
		if i == 0 {
			label = trunkLabel
		}
		b.WriteString("  " + trunkNode(cm, label) + "\n")
	}
	if len(p.TrunkLine) > 0 {
		b.WriteString("  " + c.Cyan("│") + "\n")
	}
	b.WriteString("  " + c.Dim("◇") + " " + c.Dim(p.Fork.Short) + "  " + c.Dim(p.Fork.Subject) + "\n")

	return b.String()
}
