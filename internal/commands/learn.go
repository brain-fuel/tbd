package commands

import (
	"fmt"
	"io"
	"sort"

	"goforge.dev/tbd/internal/cli"
	"goforge.dev/tbd/internal/render"
)

func init() {
	cli.Register(&cli.Command{
		Name:    "learn",
		Summary: "Guided walkthrough of the whole workflow (try: tbd learn)",
		Usage: "tbd learn [chapter]\n\n" +
			"Prints a narrated tour of tbd: setup, the single-commit feature loop,\n" +
			"publishing, conflict recovery, the two-developer deploy lease, releases,\n" +
			"and finishing. Pass a chapter to jump to one; \"tbd learn topics\" lists them.\n" +
			"It only prints (runs no git), so it is safe to read anywhere.",
		Run: runLearn,
	})
}

// lesson is a tiny printer for the walkthrough.
type lesson struct {
	c render.Colors
	w io.Writer
}

func (l lesson) h(s string)   { fmt.Fprintf(l.w, "\n%s\n", l.c.Bold(l.c.Cyan("== "+s+" =="))) }
func (l lesson) p(s string)   { fmt.Fprintf(l.w, "%s\n", s) }
func (l lesson) cmd(s string) { fmt.Fprintf(l.w, "    %s %s\n", l.c.Dim("$"), l.c.Green(s)) }
func (l lesson) out(s string) { fmt.Fprintf(l.w, "      %s\n", l.c.Dim(s)) }
func (l lesson) who(s string) { fmt.Fprintf(l.w, "%s\n", l.c.Bold(l.c.Magenta(s))) }
func (l lesson) tip(s string) { fmt.Fprintf(l.w, "  %s %s\n", l.c.Yellow("tip"), s) }
func (l lesson) blank()       { fmt.Fprintln(l.w) }

type chapter struct {
	key   string
	title string
	body  func(lesson)
}

func chapters() []chapter {
	return []chapter{
		{"setup", "1. Setup", chSetup},
		{"feature", "2. Start a feature, one commit at a time", chFeature},
		{"push", "3. Publish for review", chPush},
		{"conflict", "4. When a rebase conflicts", chConflict},
		{"lease", "5. The deploy lease: you and a teammate", chLease},
		{"release", "6. Cut a release", chRelease},
		{"finish", "7. Finish, guard, status", chFinish},
	}
}

func runLearn(c *cli.Context) error {
	l := lesson{c: c.Colors(), w: c.Stdout}
	all := chapters()

	switch sel := c.Args.Pos(0); sel {
	case "":
		l.p(l.c.Bold("tbd learn") + " - a tour of trunk-based development with tbd")
		l.p(l.c.Dim("the one rule: trunk head must be an ancestor of everything you do"))
		for _, ch := range all {
			l.h(ch.title)
			ch.body(l)
		}
		l.blank()
		l.p(l.c.Dim("jump to one chapter with: ") + l.c.Green("tbd learn lease") +
			l.c.Dim("   list them with: ") + l.c.Green("tbd learn topics"))
		return nil
	case "topics":
		l.p(l.c.Bold("chapters:"))
		for _, ch := range all {
			fmt.Fprintf(l.w, "  %-10s %s\n", l.c.Green(ch.key), ch.title)
		}
		return nil
	default:
		for _, ch := range all {
			if ch.key == sel {
				l.h(ch.title)
				ch.body(l)
				return nil
			}
		}
		keys := make([]string, 0, len(all))
		for _, ch := range all {
			keys = append(keys, ch.key)
		}
		sort.Strings(keys)
		return fmt.Errorf("unknown chapter %q (try: %v, or \"tbd learn topics\")", sel, keys)
	}
}

func chSetup(l lesson) {
	l.p("Point tbd at a repo. Pick your trunk and the deploy tags your CD watches.")
	l.cmd("tbd init trunk:develop lease-tags:deploy-now,uat-deploy")
	l.out("✓ wrote .tbd.yaml")
	l.p("That writes .tbd.yaml. See the resolved settings any time:")
	l.cmd("tbd config list")
	l.tip("no trunk yet? " + l.c.Green("tbd init :create-trunk") + " makes it from HEAD.")
}

func chFeature(l lesson) {
	l.p("Branch off the latest trunk, then work in ONE commit that tbd keeps")
	l.p("rebased on trunk for you.")
	l.cmd("tbd feature start payments")
	l.p("Edit code, then:")
	l.cmd("tbd commit message:\"add payment intent\"")
	l.p("Edit more. Commit again - it AMENDS the same commit and re-rebases:")
	l.cmd("tbd commit")
	l.p("Every tbd commit does the same three things, always:")
	l.p("  1. stage everything and collapse the feature to exactly one commit")
	l.p("  2. fetch the trunk")
	l.p("  3. rebase that commit onto the latest trunk head")
	l.p("Reword the message any time - pass a new one, or open your editor:")
	l.cmd("tbd commit message:\"clearer subject\"")
	l.cmd("tbd commit :edit")
	l.tip("a message is needed only for the first commit; later ones keep it,")
	l.p("      unless you pass message:/m: or use :edit to change it.")
}

func chPush(l lesson) {
	l.p("commit stays local. To open a PR or run CI, publish the branch:")
	l.cmd("tbd feature push")
	l.p("Because commit rewrites history each time, this force-pushes with a lease")
	l.p("(compare-and-swap): it updates YOUR branch but never clobbers a teammate's")
	l.p("push to it. Push again after every amend - the lease keeps it safe.")
	l.tip("push is for review; finish (chapter 7) is for integrating into trunk.")
}

func chConflict(l lesson) {
	l.p("If trunk moved and the rebase in step 3 conflicts, your commit is already")
	l.p("made - nothing is lost. The rebase just stops on the conflicted file.")
	l.cmd("tbd commit")
	l.out("✗ rebase of feature/payments hit a conflict")
	l.p("Fix the file, stage it, and resume - no editor, same message:")
	l.cmd("git add path/to/file")
	l.cmd("tbd continue")
	l.p("Or back the whole rebase out and stay where you were:")
	l.cmd("tbd commit :abort-on-conflict")
	l.tip("tbd continue also resolves conflicts from sync, finish, and push.")
}

func chLease(l lesson) {
	l.p("A lease is the deploy tag your CD pipeline watches - here, deploy-now for a")
	l.p("shared staging environment. Only one commit is deployed at a time, and tbd")
	l.p("decides the move from where the tag points now relative to YOUR branch.")
	l.blank()
	l.p(l.c.Bold("Cast:") + " you are on feature/payments; Dana is on feature/search.")
	l.blank()

	l.who("You - first deploy")
	l.p("deploy-now is unset, so the first lease bootstraps it to trunk head:")
	l.cmd("tbd lease deploy-now")
	l.out("initializing deploy-now at trunk head")
	l.p("Run it again. The tag sits on trunk (an ancestor of your branch), so it")
	l.p("ADVANCES to your latest commit - CD now deploys your payments work:")
	l.cmd("tbd lease deploy-now")
	l.out("advancing deploy-now to your latest commit")
	l.blank()

	l.who("Dana - takes the slot")
	l.p("Dana wants staging for her search branch. To her, deploy-now is on someone")
	l.p("else's branch (yours), so tbd TAKES it to her tip - CD switches to search:")
	l.cmd("tbd lease deploy-now")
	l.out("taking deploy-now (currently on feature/payments)")
	l.p("If you both run it at the same instant, compare-and-swap picks one winner;")
	l.p("the loser is told who holds it now. No double deploy.")
	l.blank()

	l.who("You - take it back, even after an amend")
	l.p("You amend payments (tbd commit), then reclaim staging. deploy-now is now on")
	l.p("Dana's branch (foreign to you), so it is taken back to your new tip:")
	l.cmd("tbd commit")
	l.cmd("tbd lease deploy-now")
	l.out("taking deploy-now (currently on feature/search)")
	l.p("Note the amend did not confuse it: the old deployed commit was your earlier")
	l.p("one, recognized via your branch's reflog, never mistaken for a stranger's.")
	l.blank()

	l.p(l.c.Bold("Handy:"))
	l.cmd("tbd lease status            # where each deploy tag points, and who holds it")
	l.cmd("tbd lease deploy-now :no-advance   # don't move your own stale lease")
	l.cmd("tbd lease deploy-now :force        # override the compare-and-swap check")
	l.tip("what git really guarantees: one winner per race, not a lock held across")
	l.p("      the whole CD run. tbd does not pretend otherwise.")
	l.blank()
	l.p("This is the " + l.c.Bold("tag") + " strategy. With lease-strategy: ephemeral-branch a")
	l.p("deploy slot is instead a branch that exists only while leased: each lease")
	l.p("blows it away and remakes it at your tip. lease-strategy: none turns it off.")
}

func chRelease(l lesson) {
	l.p("Cut a release from a trunk commit - as a branch, a tag, or both:")
	l.cmd("tbd release cut 1.4.0 strategy:branch,tag")
	l.out("✓ created release branch release/1.4.0")
	l.out("✓ created release tag v1.4.0")
	l.p("A release can only come from a commit that is on trunk. List them:")
	l.cmd("tbd release list")
}

func chFinish(l lesson) {
	l.p("When the feature is ready, fold it into trunk. finish rebases once more,")
	l.p("fast-forwards trunk (never a merge commit), pushes trunk, and deletes the")
	l.p("branch:")
	l.cmd("tbd feature finish")
	l.p("Check the one rule any time - exits 0 if it holds, 1 if not (great in CI):")
	l.cmd("tbd guard")
	l.p("And see everything at a glance - trunk, your branch, leases, releases:")
	l.cmd("tbd status")
	l.blank()
	l.p(l.c.Green("That's the whole tool.") + " Branch, commit (one commit, always rebased),")
	l.p("push for review, lease to deploy, release, finish. The invariant holds the")
	l.p("whole way through.")
	l.blank()
	l.p("Adopting a \"normal\" multi-commit branch? " + l.c.Green("tbd rebase") + " squashes it to one")
	l.p("commit on trunk. To squash your work onto a different branch as a new branch:")
	l.cmd("tbd cherry-put onto:some-branch as:my-work")
	l.blank()
	l.p(l.c.Bold("Global options") + " (on every command): color is on for a terminal, off when")
	l.p("piped; force it with color-mode:none or color-mode:always, or set NO_COLOR.")
	l.p(":local skips the network; :no-fetch skips the pre-fetch.")
	l.p("Slow steps (fetch, push, rebase) are announced as they run, with a spinner")
	l.p("on a terminal, so you always see what tbd is doing.")
}
