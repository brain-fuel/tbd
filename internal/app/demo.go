package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"goforge.dev/tbd/v2/internal/v2/gitops"
	v2state "goforge.dev/tbd/v2/internal/v2/state"
)

const demoMarker = ".tbd-demo-owned"

func newDemoCommand(opts *rootOptions) *cobra.Command {
	var addr, dir string
	var noOpen bool
	cmd := &cobra.Command{
		Use:   "demo [basic|stack|collab]",
		Short: "run a browser simulation with four local repos and one remote",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			scenario := "basic"
			if len(args) == 1 {
				scenario = args[0]
			}
			if scenario != "basic" && scenario != "stack" && scenario != "collab" {
				return fmt.Errorf("unknown demo %q (use: basic, stack, collab)", scenario)
			}
			cwd, _ := os.Getwd()
			if !filepath.IsAbs(dir) {
				dir = filepath.Join(cwd, dir)
			}
			exe, err := os.Executable()
			if err != nil {
				return err
			}
			demo := newDemoRunner(dir, exe, scenario)
			if err := demo.Reset(); err != nil {
				return err
			}
			defer demo.Cleanup()

			mux := http.NewServeMux()
			demo.mount(mux)
			server := &http.Server{Addr: addr, Handler: mux}
			errc := make(chan error, 1)
			go func() {
				err := server.ListenAndServe()
				if err != nil && !errors.Is(err, http.ErrServerClosed) {
					errc <- err
				}
			}()

			url := "http://" + addr + "/"
			fmt.Fprintf(cmd.OutOrStdout(), "demo workspace: %s\n", dir)
			fmt.Fprintf(cmd.OutOrStdout(), "demo scenario: %s\n", scenario)
			fmt.Fprintf(cmd.OutOrStdout(), "demo server: %s\n", url)
			if !noOpen {
				go openBrowser(url)
			}

			sigc := make(chan os.Signal, 1)
			signal.Notify(sigc, os.Interrupt, syscall.SIGTERM)
			select {
			case sig := <-sigc:
				fmt.Fprintf(cmd.OutOrStdout(), "\nstopping demo after %s\n", sig)
			case err := <-errc:
				return err
			}
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_ = server.Shutdown(ctx)
			return nil
		},
	}
	cmd.Flags().StringVar(&addr, "addr", "127.0.0.1:8088", "listen address")
	cmd.Flags().StringVar(&dir, "dir", ".tbd-demo", "demo workspace directory")
	cmd.Flags().BoolVar(&noOpen, "no-open", false, "do not try to open the browser")
	return cmd
}

type demoRunner struct {
	mu       sync.Mutex
	dir      string
	exe      string
	origin   string
	scenario string
	agents   []demoAgent
	steps    []demoStep
	index    int
	logs     map[string][]demoLog
	active   []string
}

type demoAgent struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
	Repo  string `json:"repo"`
}

type demoStep struct {
	Title   string       `json:"title"`
	Detail  string       `json:"detail"`
	Actions []demoAction `json:"-"`
}

type demoStepSummary struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	Detail string `json:"detail"`
}

type demoAction struct {
	Actor        string
	Repo         string
	Label        string
	Kind         string
	Args         []string
	Path         string
	Content      string
	AllowFailure bool
}

type demoLog struct {
	Time    string `json:"time"`
	Step    int    `json:"step"`
	Command string `json:"command"`
	Output  string `json:"output"`
	Failed  bool   `json:"failed"`
}

type demoState struct {
	Dir          string           `json:"dir"`
	Origin       string           `json:"origin"`
	Scenario     string           `json:"scenario"`
	Step         int              `json:"step"`
	Total        int              `json:"total"`
	Done         bool             `json:"done"`
	Next         *demoStepSummary `json:"next,omitempty"`
	Last         *demoStepSummary `json:"last,omitempty"`
	Agents       []demoAgentView  `json:"agents"`
	ActiveActors []string         `json:"activeActors,omitempty"`
	Message      string           `json:"message,omitempty"`
}

type demoAgentView struct {
	ID    string              `json:"id"`
	Name  string              `json:"name"`
	Email string              `json:"email"`
	Path  string              `json:"path"`
	Graph visualGraphSnapshot `json:"graph"`
	Logs  []demoLog           `json:"logs"`
}

func newDemoRunner(dir, exe, scenario string) *demoRunner {
	return &demoRunner{
		dir:      dir,
		exe:      exe,
		origin:   filepath.Join(dir, "origin.git"),
		scenario: scenario,
		agents: []demoAgent{
			{ID: "ada", Name: "Ada", Email: "ada@example.com", Repo: filepath.Join(dir, "ada")},
			{ID: "ben", Name: "Ben", Email: "ben@example.com", Repo: filepath.Join(dir, "ben")},
			{ID: "cy", Name: "Cy", Email: "cy@example.com", Repo: filepath.Join(dir, "cy")},
			{ID: "dee", Name: "Dee", Email: "dee@example.com", Repo: filepath.Join(dir, "dee")},
		},
		steps: demoScript(scenario),
		logs:  map[string][]demoLog{},
	}
}

func (d *demoRunner) mount(mux *http.ServeMux) {
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, demoHTML)
	})
	mux.HandleFunc("/assets/wasm_exec.js", serveWasmExecJS)
	mux.HandleFunc("/assets/tbd_visual.wasm", serveVisualWASM)
	mux.HandleFunc("/assets/demo.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		_, _ = io.WriteString(w, demoJS)
	})
	mux.HandleFunc("/api/demo/state", func(w http.ResponseWriter, r *http.Request) {
		d.writeState(w, "")
	})
	mux.HandleFunc("/api/demo/step", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", http.StatusMethodNotAllowed)
			return
		}
		msg, err := d.Step()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		d.writeState(w, msg)
	})
	mux.HandleFunc("/api/demo/reset", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", http.StatusMethodNotAllowed)
			return
		}
		if err := d.Reset(); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		d.writeState(w, "reset demo workspace")
	})
}

func (d *demoRunner) Reset() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.removeOwnedDir(); err != nil {
		return err
	}
	if err := os.MkdirAll(d.dir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(d.dir, demoMarker), []byte(time.Now().Format(time.RFC3339)+"\n"), 0o644); err != nil {
		return err
	}
	d.index = 0
	d.logs = map[string][]demoLog{}
	d.active = nil
	if err := d.prepareRepos(); err != nil {
		return err
	}
	return nil
}

func (d *demoRunner) Cleanup() {
	d.mu.Lock()
	defer d.mu.Unlock()
	_ = d.removeOwnedDir()
}

func (d *demoRunner) removeOwnedDir() error {
	if _, err := os.Stat(d.dir); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if _, err := os.Stat(filepath.Join(d.dir, demoMarker)); err != nil {
		return fmt.Errorf("refusing to remove %s: missing %s marker", d.dir, demoMarker)
	}
	return os.RemoveAll(d.dir)
}

func (d *demoRunner) prepareRepos() error {
	if res := runExternal(d.dir, "git", "init", "-q", "--bare", "-b", "main", d.origin); res.err != nil {
		return res.err
	}
	seed := filepath.Join(d.dir, "seed")
	if res := runExternal(d.dir, "git", "clone", "-q", d.origin, seed); res.err != nil {
		return res.err
	}
	if err := configureGitUser(seed, "seed", "seed@example.com"); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(seed, "README.md"), []byte("tbd demo\n"), 0o644); err != nil {
		return err
	}
	if res := d.runTBD(seed, "init", "--yes", "--deploy-ref", "dev-deploy", "--deploy-ref", "qa-deploy", "--deploy-ref", "prod-deploy"); res.err != nil {
		return res.err
	}
	for _, args := range [][]string{
		{"add", "-A"},
		{"commit", "-q", "-m", "chore: seed demo"},
		{"push", "-q", "-u", "origin", "main"},
	} {
		if res := runExternal(seed, "git", args...); res.err != nil {
			return res.err
		}
	}
	for _, agent := range d.agents {
		if res := runExternal(d.dir, "git", "clone", "-q", d.origin, agent.Repo); res.err != nil {
			return res.err
		}
		if err := configureGitUser(agent.Repo, strings.ToLower(agent.Name), agent.Email); err != nil {
			return err
		}
	}
	return nil
}

func configureGitUser(dir, name, email string) error {
	for _, args := range [][]string{
		{"config", "user.name", name},
		{"config", "user.email", email},
		{"config", "commit.gpgsign", "false"},
		{"config", "tag.gpgsign", "false"},
	} {
		if res := runExternal(dir, "git", args...); res.err != nil {
			return res.err
		}
	}
	return nil
}

func (d *demoRunner) Step() (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.index >= len(d.steps) {
		return "demo complete", nil
	}
	step := d.steps[d.index]
	stepNo := d.index + 1
	active := map[string]bool{}
	for _, action := range step.Actions {
		log := d.runAction(stepNo, action)
		d.logs[action.Actor] = append(d.logs[action.Actor], log)
		if action.Actor != "" {
			active[action.Actor] = true
		}
		if log.Failed && !action.AllowFailure {
			return step.Title, fmt.Errorf("%s: %s\n%s", action.Actor, action.Label, log.Output)
		}
	}
	d.index++
	d.active = keys(active)
	return step.Title, nil
}

func (d *demoRunner) runAction(stepNo int, action demoAction) demoLog {
	start := time.Now()
	var res commandResult
	switch action.Kind {
	case "write":
		res = d.writeDemoFile(action, false)
	case "append":
		res = d.writeDemoFile(action, true)
	case "git":
		res = runExternal(d.repo(action.Repo), "git", action.Args...)
	case "resolve-state":
		res = resolveDemoStateConflict(d.repo(action.Repo))
	default:
		res = d.runTBD(d.repo(action.Repo), action.Args...)
	}
	out := strings.TrimSpace(res.output)
	if out == "" && res.err != nil {
		out = res.err.Error()
	}
	return demoLog{
		Time:    start.Format("15:04:05"),
		Step:    stepNo,
		Command: action.Label,
		Output:  out,
		Failed:  res.err != nil,
	}
}

func (d *demoRunner) writeDemoFile(action demoAction, appendMode bool) commandResult {
	path := filepath.Join(d.repo(action.Repo), action.Path)
	flag := os.O_CREATE | os.O_WRONLY
	if appendMode {
		flag |= os.O_APPEND
	} else {
		flag |= os.O_TRUNC
	}
	f, err := os.OpenFile(path, flag, 0o644)
	if err != nil {
		return commandResult{err: err, output: err.Error()}
	}
	defer f.Close()
	if _, err := f.WriteString(action.Content); err != nil {
		return commandResult{err: err, output: err.Error()}
	}
	return commandResult{output: "updated " + action.Path}
}

func (d *demoRunner) repo(id string) string {
	for _, agent := range d.agents {
		if agent.ID == id {
			return agent.Repo
		}
	}
	return d.dir
}

func (d *demoRunner) writeState(w http.ResponseWriter, message string) {
	state, err := d.State(message)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(state)
}

func (d *demoRunner) State(message string) (demoState, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	state := demoState{
		Dir:          d.dir,
		Origin:       d.origin,
		Scenario:     d.scenario,
		Step:         d.index,
		Total:        len(d.steps),
		Done:         d.index >= len(d.steps),
		ActiveActors: append([]string(nil), d.active...),
		Message:      message,
	}
	if d.index < len(d.steps) {
		s := d.steps[d.index]
		state.Next = &demoStepSummary{Number: d.index + 1, Title: s.Title, Detail: s.Detail}
	}
	if d.index > 0 {
		s := d.steps[d.index-1]
		state.Last = &demoStepSummary{Number: d.index, Title: s.Title, Detail: s.Detail}
	}
	for _, agent := range d.agents {
		graph, _ := d.graphFor(agent.Repo)
		state.Agents = append(state.Agents, demoAgentView{
			ID:    agent.ID,
			Name:  agent.Name,
			Email: agent.Email,
			Path:  agent.Repo,
			Graph: graph,
			Logs:  append([]demoLog(nil), d.logs[agent.ID]...),
		})
	}
	return state, nil
}

func (d *demoRunner) graphFor(repo string) (visualGraphSnapshot, error) {
	env, err := gitops.Load(repo, io.Discard, io.Discard, false)
	if err != nil {
		return visualGraphSnapshot{}, err
	}
	_ = env.Fetch()
	return buildVisualGraph(env, 80)
}

func (d *demoRunner) runTBD(dir string, args ...string) commandResult {
	return runExternal(dir, d.exe, args...)
}

type commandResult struct {
	output string
	err    error
}

func runExternal(dir, name string, args ...string) commandResult {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	return commandResult{output: out.String(), err: err}
}

func resolveDemoStateConflict(repo string) commandResult {
	unmerged := runExternal(repo, "git", "diff", "--name-only", "--diff-filter=U")
	if unmerged.err != nil {
		return unmerged
	}
	if !strings.Contains(unmerged.output, ".tbd/state.json") {
		return commandResult{output: "no .tbd/state.json conflict"}
	}
	ours, err := readConflictState(repo, "2")
	if err != nil {
		return commandResult{output: err.Error(), err: err}
	}
	theirs, err := readConflictState(repo, "3")
	if err != nil {
		return commandResult{output: err.Error(), err: err}
	}
	merged := mergeDemoState(ours, theirs)
	data, err := json.MarshalIndent(merged, "", "  ")
	if err != nil {
		return commandResult{output: err.Error(), err: err}
	}
	data = append(data, '\n')
	if err := os.WriteFile(filepath.Join(repo, ".tbd", "state.json"), data, 0o644); err != nil {
		return commandResult{output: err.Error(), err: err}
	}
	if res := runExternal(repo, "git", "add", ".tbd/state.json"); res.err != nil {
		return res
	}
	res := runGitEditorTrue(repo, "rebase", "--continue")
	if res.err != nil {
		return res
	}
	res.output = strings.TrimSpace("merged .tbd/state.json\n" + res.output)
	return res
}

func readConflictState(repo, stage string) (v2state.State, error) {
	res := runExternal(repo, "git", "show", ":"+stage+":.tbd/state.json")
	if res.err != nil {
		return v2state.State{}, res.err
	}
	st := v2state.New()
	if err := json.Unmarshal([]byte(res.output), &st); err != nil {
		return v2state.State{}, err
	}
	return st, nil
}

func mergeDemoState(ours, theirs v2state.State) v2state.State {
	out := v2state.New()
	out.UpdatedAt = gitNowMax(ours.UpdatedAt, theirs.UpdatedAt)
	for k, v := range ours.Items {
		out.Items[k] = v
	}
	for k, v := range theirs.Items {
		out.Items[k] = v
	}
	for k, v := range ours.Groups {
		out.Groups[k] = v
	}
	for k, v := range theirs.Groups {
		out.Groups[k] = v
	}
	for k, v := range ours.UAT {
		out.UAT[k] = v
	}
	for k, v := range theirs.UAT {
		out.UAT[k] = v
	}
	return out
}

func gitNowMax(a, b string) string {
	if a > b {
		return a
	}
	return b
}

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func runGitEditorTrue(dir string, args ...string) commandResult {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_EDITOR=true")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	return commandResult{output: out.String(), err: err}
}

type demoIssue struct {
	ID   string
	Desc string
}

type demoWorkflow struct {
	Actor   string
	Kind    string
	Label   string
	File    string
	Initial string
	Polish  string
	Version string
	Issues  []demoIssue
}

func demoScript(scenario string) []demoStep {
	workflows := demoWorkflows(scenario)
	steps := []demoStep{
		{
			Title:   demoStartTitle(scenario),
			Detail:  demoStartDetail(scenario),
			Actions: startWorkflowActions(workflows),
		},
		{
			Title:   "Everyone makes work and amends it",
			Detail:  "Each local repo edits files, creates a tbd commit, then amends the deployable candidate with another change.",
			Actions: workCommitActions(workflows),
		},
	}
	for i, wf := range workflows {
		steps = append(steps, deployWorkflowSteps(wf, i == 0, scenario)...)
	}
	steps = append(steps, demoStep{
		Title:  "Everyone syncs and inspects",
		Detail: "Each clone fetches the shared remote state; the graphs show local branches, remote trunk, release tags, and deploy refs.",
		Actions: []demoAction{
			tbdAction("ada", "sync"),
			tbdAction("ada", "graph", "--limit", "16"),
			tbdAction("ben", "sync"),
			gitAction("ben", "status", "--short"),
			tbdAction("cy", "sync"),
			gitAction("cy", "status", "--short"),
			tbdAction("dee", "sync"),
			gitAction("dee", "status", "--short"),
		},
	})
	return steps
}

func demoWorkflows(scenario string) []demoWorkflow {
	workflows := []demoWorkflow{
		{
			Actor:   "ada",
			Kind:    "feature",
			Label:   "checkout feature",
			File:    "checkout.txt",
			Initial: "checkout buttons v1\ncheckout validation v1\n",
			Polish:  "checkout copy polish\n",
			Version: "1.1.0",
			Issues:  []demoIssue{{ID: "SHOP-101", Desc: "Checkout buttons"}},
		},
	}
	switch scenario {
	case "stack":
		workflows = append(workflows,
			stackWorkflow("ben", "billing stack", "billing.txt", "1.2.0", []demoIssue{
				{ID: "BILL-201", Desc: "Invoice header"},
				{ID: "BILL-202", Desc: "Invoice totals"},
				{ID: "BILL-203", Desc: "Invoice export"},
			}),
			stackWorkflow("cy", "search stack", "search.txt", "1.3.0", []demoIssue{
				{ID: "SEARCH-301", Desc: "Search API"},
				{ID: "SEARCH-302", Desc: "Search UI"},
				{ID: "SEARCH-303", Desc: "Search telemetry"},
			}),
			stackWorkflow("dee", "ops stack", "ops.txt", "1.4.0", []demoIssue{
				{ID: "OPS-401", Desc: "Audit notifications"},
				{ID: "OPS-402", Desc: "Audit digest"},
				{ID: "OPS-403", Desc: "Audit escalation"},
			}),
		)
	case "collab":
		workflows = append(workflows,
			collabWorkflow("ben", "billing collab", "billing.txt", "1.2.0", []demoIssue{
				{ID: "BILL-201", Desc: "Invoice header"},
				{ID: "BILL-202", Desc: "Invoice totals"},
				{ID: "BILL-203", Desc: "Invoice export"},
			}),
			collabWorkflow("cy", "search collab", "search.txt", "1.3.0", []demoIssue{
				{ID: "SEARCH-301", Desc: "Search API"},
				{ID: "SEARCH-302", Desc: "Search UI"},
				{ID: "SEARCH-303", Desc: "Search telemetry"},
			}),
			collabWorkflow("dee", "ops collab", "ops.txt", "1.4.0", []demoIssue{
				{ID: "OPS-401", Desc: "Audit notifications"},
				{ID: "OPS-402", Desc: "Audit digest"},
				{ID: "OPS-403", Desc: "Audit escalation"},
			}),
		)
	default:
		workflows = append(workflows,
			featureWorkflow("ben", "billing feature", "billing.txt", "1.2.0", demoIssue{ID: "BILL-201", Desc: "Invoice header"}),
			featureWorkflow("cy", "search feature", "search.txt", "1.3.0", demoIssue{ID: "SEARCH-301", Desc: "Search API"}),
			featureWorkflow("dee", "ops feature", "ops.txt", "1.4.0", demoIssue{ID: "OPS-401", Desc: "Audit notifications"}),
		)
	}
	return workflows
}

func featureWorkflow(actor, label, file, version string, issue demoIssue) demoWorkflow {
	return demoWorkflow{
		Actor:   actor,
		Kind:    "feature",
		Label:   label,
		File:    file,
		Initial: issue.Desc + " v1\n",
		Polish:  issue.Desc + " polish\n",
		Version: version,
		Issues:  []demoIssue{issue},
	}
}

func stackWorkflow(actor, label, file, version string, issues []demoIssue) demoWorkflow {
	return aggregateWorkflow(actor, "stack", label, file, version, issues)
}

func collabWorkflow(actor, label, file, version string, issues []demoIssue) demoWorkflow {
	return aggregateWorkflow(actor, "collab", label, file, version, issues)
}

func aggregateWorkflow(actor, kind, label, file, version string, issues []demoIssue) demoWorkflow {
	initial := strings.Builder{}
	polish := strings.Builder{}
	for _, issue := range issues {
		initial.WriteString(issue.ID + " " + issue.Desc + " v1\n")
		polish.WriteString(issue.ID + " " + issue.Desc + " polish\n")
	}
	return demoWorkflow{
		Actor:   actor,
		Kind:    kind,
		Label:   label,
		File:    file,
		Initial: initial.String(),
		Polish:  polish.String(),
		Version: version,
		Issues:  issues,
	}
}

func demoStartTitle(scenario string) string {
	switch scenario {
	case "stack":
		return "One feature and three stack workflows start"
	case "collab":
		return "One feature and three collab workflows start"
	default:
		return "Four independent feature workflows start"
	}
}

func demoStartDetail(scenario string) string {
	switch scenario {
	case "stack":
		return "Ada starts a standalone feature while Ben, Cy, and Dee each start a three-item ordered stack."
	case "collab":
		return "Ada starts a standalone feature while Ben, Cy, and Dee each start a three-item collab aggregate."
	default:
		return "Ada, Ben, Cy, and Dee each start a standalone feature branch from the shared trunk."
	}
}

func startWorkflowActions(workflows []demoWorkflow) []demoAction {
	actions := make([]demoAction, 0, len(workflows))
	for _, wf := range workflows {
		args := []string{wf.Kind}
		for _, issue := range wf.Issues {
			args = append(args, "--id", issue.ID, "--desc", issue.Desc)
		}
		actions = append(actions, tbdAction(wf.Actor, args...))
	}
	return actions
}

func workCommitActions(workflows []demoWorkflow) []demoAction {
	actions := []demoAction{}
	for _, wf := range workflows {
		actions = append(actions,
			writeAction(wf.Actor, wf.File, wf.Initial),
			tbdAction(wf.Actor, "commit", "--no-edit"),
			appendAction(wf.Actor, wf.File, wf.Polish),
			tbdAction(wf.Actor, "commit", "--no-edit"),
		)
	}
	return actions
}

func deployWorkflowSteps(wf demoWorkflow, first bool, scenario string) []demoStep {
	if first {
		return []demoStep{
			{
				Title:  "Ada leases dev and the mutex rules animate",
				Detail: "Ada borrows dev-deploy. Dee's plain lease is refused while Ada holds it; Dee then explicitly steals, relinquishes back to trunk, and Ada leases dev again.",
				Actions: []demoAction{
					tbdAction("ada", "lease", "dev-deploy"),
					allowFailure(tbdAction("dee", "lease", "dev-deploy")),
					tbdAction("dee", "steal", "dev-deploy"),
					tbdAction("dee", "relinquish", "dev-deploy"),
					tbdAction("ada", "lease", "dev-deploy"),
				},
			},
			{
				Title:  "Ada promotes dev to QA",
				Detail: "Ada relinquishes dev back to trunk, leases qa-deploy, and records an RC for QA.",
				Actions: []demoAction{
					tbdAction("ada", "relinquish", "dev-deploy"),
					tbdAction("ada", "lease", "qa-deploy"),
					tbdAction("ada", "release", "rc", wf.Version),
				},
			},
			{
				Title:  "Ada promotes QA to prod",
				Detail: "QA returns to trunk, prod-deploy points at Ada's candidate, and release complete fast-forwards trunk.",
				Actions: []demoAction{
					tbdAction("ada", "relinquish", "qa-deploy"),
					tbdAction("ada", "lease", "prod-deploy"),
					tbdAction("ada", "release", "complete", wf.Version),
				},
			},
		}
	}

	name := strings.ToUpper(wf.Actor[:1]) + wf.Actor[1:]
	shape := wf.Kind
	if scenario == "basic" {
		shape = "feature"
	}
	return []demoStep{
		{
			Title:  name + " rebases and leases dev",
			Detail: fmt.Sprintf("%s catches up to the latest trunk release, resolves shared metadata when needed, then borrows dev-deploy.", wf.Label),
			Actions: []demoAction{
				allowFailure(tbdAction(wf.Actor, "commit", "--no-edit")),
				resolveStateAction(wf.Actor),
				tbdAction(wf.Actor, "commit", "--no-edit"),
				tbdAction(wf.Actor, "lease", "dev-deploy"),
			},
		},
		{
			Title:  name + " promotes " + shape + " through QA",
			Detail: "The candidate moves from dev to QA and records an RC before production.",
			Actions: []demoAction{
				tbdAction(wf.Actor, "relinquish", "dev-deploy"),
				tbdAction(wf.Actor, "lease", "qa-deploy"),
				tbdAction(wf.Actor, "release", "rc", wf.Version),
			},
		},
		{
			Title:  name + " completes " + shape + " in prod",
			Detail: "The prod deploy ref points at the candidate, then the production release advances trunk.",
			Actions: []demoAction{
				tbdAction(wf.Actor, "relinquish", "qa-deploy"),
				tbdAction(wf.Actor, "lease", "prod-deploy"),
				tbdAction(wf.Actor, "release", "complete", wf.Version),
			},
		},
	}
}

func tbdAction(actor string, args ...string) demoAction {
	return demoAction{Actor: actor, Repo: actor, Kind: "tbd", Args: args, Label: "tbd " + strings.Join(args, " ")}
}

func writeAction(actor, path, content string) demoAction {
	return demoAction{Actor: actor, Repo: actor, Kind: "write", Path: path, Content: content, Label: "edit " + path}
}

func appendAction(actor, path, content string) demoAction {
	return demoAction{Actor: actor, Repo: actor, Kind: "append", Path: path, Content: content, Label: "edit " + path}
}

func gitAction(actor string, args ...string) demoAction {
	return demoAction{Actor: actor, Repo: actor, Kind: "git", Args: args, Label: "git " + strings.Join(args, " ")}
}

func resolveStateAction(actor string) demoAction {
	return demoAction{Actor: actor, Repo: actor, Kind: "resolve-state", Label: "merge .tbd/state.json; git rebase --continue"}
}

func allowFailure(action demoAction) demoAction {
	action.AllowFailure = true
	return action
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		if os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
			return
		}
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}
