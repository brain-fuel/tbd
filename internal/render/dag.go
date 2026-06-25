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
//
// Render uses the plan's FeatLine for both halves, so the "after" SHAs are a
// projection (the pre-rebase commits). Callers that want the "after" graph to
// show the real post-rebase SHAs should render the halves separately
// (RenderBefore, then re-read the new SHAs into the plan, then RenderAfter).
func (p RebasePlan) Render(c Colors) string {
	return p.RenderBefore(c) + "\n" + p.RenderAfter(c)
}

// RenderBefore draws the "before" half: the feature diverged from trunk.
func (p RebasePlan) RenderBefore(c Colors) string {
	var b strings.Builder
	trunkLabel := "(trunk head: " + p.Trunk + ")"
	featLabel := "← " + p.Feature

	b.WriteString(c.Bold("before") + "\n")
	for i, cm := range p.TrunkLine {
		label := ""
		if i == 0 {
			label = trunkLabel
		}
		b.WriteString("  " + trunkNode(c, cm, label) + "\n")
	}
	if len(p.TrunkLine) > 0 {
		b.WriteString("  " + c.Cyan("│") + "\n")
	}
	for i, cm := range p.FeatLine {
		label := ""
		if i == 0 {
			label = featLabel
		}
		b.WriteString("  " + c.Cyan("│") + " " + featNode(c, cm, label, c.Yellow("○")) + "\n")
	}
	if len(p.FeatLine) > 0 {
		b.WriteString("  " + c.Cyan("├─╯") + "\n")
	}
	b.WriteString("  " + c.Dim("◇") + " " + c.Dim(p.Fork.Short) + "  " + c.Dim(p.Fork.Subject+"  (fork point)") + "\n")
	return b.String()
}

// RenderAfter draws the "after" half: the feature replayed on top of trunk head.
// It draws the replayed nodes from FeatLine, so callers should set FeatLine to
// the real post-rebase commits before calling it (see visualizeRebase).
func (p RebasePlan) RenderAfter(c Colors) string {
	var b strings.Builder
	trunkLabel := "(trunk head: " + p.Trunk + ")"
	featLabel := "← " + p.Feature

	b.WriteString(c.Bold("after") + "\n")
	for i, cm := range p.FeatLine {
		label := ""
		if i == 0 {
			label = featLabel + " (replayed)"
		}
		b.WriteString("  " + featNode(c, cm, label, c.Green("●")) + "\n")
	}
	for i, cm := range p.TrunkLine {
		label := ""
		if i == 0 {
			label = trunkLabel
		}
		b.WriteString("  " + trunkNode(c, cm, label) + "\n")
	}
	if len(p.TrunkLine) > 0 {
		b.WriteString("  " + c.Cyan("│") + "\n")
	}
	b.WriteString("  " + c.Dim("◇") + " " + c.Dim(p.Fork.Short) + "  " + c.Dim(p.Fork.Subject) + "\n")
	return b.String()
}

func trunkNode(c Colors, cm Commit, label string) string {
	line := c.Cyan("●") + " " + c.Bold(cm.Short) + "  " + cm.Subject
	if label != "" {
		line += "  " + c.Dim(label)
	}
	return line
}

func featNode(c Colors, cm Commit, label, glyph string) string {
	line := glyph + " " + c.Yellow(cm.Short) + "  " + cm.Subject
	if label != "" {
		line += "  " + c.Dim(label)
	}
	return line
}
