//go:build js && wasm

package main

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"syscall/js"
)

const svgNS = "http://www.w3.org/2000/svg"

var eventHandlers = map[string][]js.Func{}

type snapshot struct {
	Commits []commit `json:"commits"`
	Refs    []ref    `json:"refs"`
	Edges   []edge   `json:"edges"`
	Status  status   `json:"status"`
}

type commit struct {
	SHA     string   `json:"sha"`
	Short   string   `json:"short"`
	Parents []string `json:"parents"`
	Author  string   `json:"author"`
	Time    string   `json:"time"`
	Subject string   `json:"subject"`
}

type ref struct {
	Full    string `json:"full"`
	Name    string `json:"name"`
	Kind    string `json:"kind"`
	Role    string `json:"role"`
	Target  string `json:"target"`
	Current bool   `json:"current"`
	Remote  string `json:"remote"`
}

type edge struct {
	From    string `json:"from"`
	To      string `json:"to"`
	Shallow bool   `json:"shallow"`
}

type status struct {
	Branch   string `json:"branch"`
	Detached bool   `json:"detached"`
	Head     string `json:"head"`
	Clean    bool   `json:"clean"`
}

type renderOptions struct {
	Filters map[string]bool `json:"filters"`
	Zoom    float64         `json:"zoom"`
}

type point struct {
	X float64
	Y float64
}

type layout struct {
	Positions map[string]point
	Refs      map[string][]ref
	BySHA     map[string]commit
	Index     map[string]int
	Lanes     int
	Width     float64
	Height    float64
}

func main() {
	renderFn := js.FuncOf(func(this js.Value, args []js.Value) any {
		if len(args) < 1 {
			return nil
		}
		graphJSON := args[0].String()
		optionsJSON := "{}"
		if len(args) > 1 {
			optionsJSON = args[1].String()
		}
		if err := renderTo("graph-stage", graphJSON, optionsJSON); err != nil {
			showError(err)
		}
		return nil
	})
	renderToFn := js.FuncOf(func(this js.Value, args []js.Value) any {
		if len(args) < 2 {
			return nil
		}
		stageID := args[0].String()
		graphJSON := args[1].String()
		optionsJSON := "{}"
		if len(args) > 2 {
			optionsJSON = args[2].String()
		}
		if err := renderTo(stageID, graphJSON, optionsJSON); err != nil {
			showErrorTo(stageID, err)
		}
		return nil
	})
	obj := js.Global().Get("Object").New()
	obj.Set("render", renderFn)
	obj.Set("renderTo", renderToFn)
	js.Global().Set("tbdVisual", obj)
	select {}
}

func renderTo(stageID, graphJSON, optionsJSON string) error {
	var graph snapshot
	if err := json.Unmarshal([]byte(graphJSON), &graph); err != nil {
		return err
	}
	opts := renderOptions{Zoom: 1, Filters: map[string]bool{}}
	_ = json.Unmarshal([]byte(optionsJSON), &opts)
	if opts.Zoom <= 0 {
		opts.Zoom = 1
	}
	for _, handler := range eventHandlers[stageID] {
		handler.Release()
	}
	eventHandlers[stageID] = nil

	stage := js.Global().Get("document").Call("getElementById", stageID)
	if !stage.Truthy() {
		return fmt.Errorf("%s not found", stageID)
	}
	stage.Set("innerHTML", "")
	if len(graph.Commits) == 0 {
		div := js.Global().Get("document").Call("createElement", "div")
		div.Set("className", "loading")
		div.Set("textContent", "No commits to render.")
		stage.Call("appendChild", div)
		return nil
	}

	refs := visibleRefs(graph.Refs, opts)
	lay := computeLayout(graph.Commits, refs)
	displayWidth := lay.Width * opts.Zoom
	displayHeight := lay.Height * opts.Zoom
	svg := svgEl("svg")
	setAttrs(svg, map[string]string{
		"class":   "graph-svg",
		"width":   fmt.Sprintf("%.0f", displayWidth),
		"height":  fmt.Sprintf("%.0f", displayHeight),
		"viewBox": fmt.Sprintf("0 0 %.0f %.0f", lay.Width, lay.Height),
	})
	addDefs(svg)

	for _, ed := range graph.Edges {
		drawEdge(svg, lay, ed)
	}
	for _, cm := range graph.Commits {
		drawCommit(svg, lay, stageID, cm)
	}
	for _, cm := range graph.Commits {
		drawRefs(svg, lay, cm.SHA)
	}
	stage.Call("appendChild", svg)
	return nil
}

func visibleRefs(refs []ref, opts renderOptions) []ref {
	out := make([]ref, 0, len(refs))
	enabled := func(name string) bool {
		if opts.Filters == nil {
			return true
		}
		v, ok := opts.Filters[name]
		return !ok || v
	}
	for _, r := range refs {
		if r.Kind == "head" {
			out = append(out, r)
			continue
		}
		if r.Role == "deploy" && !enabled("deploy") {
			continue
		}
		if r.Role == "release" && !enabled("release") {
			continue
		}
		if r.Role == "lock" && !enabled("lock") {
			continue
		}
		switch r.Kind {
		case "branch":
			if !enabled("branch") {
				continue
			}
		case "remote":
			if !enabled("remote") {
				continue
			}
		case "tag":
			if !enabled("tag") {
				continue
			}
		}
		out = append(out, r)
	}
	return out
}

func computeLayout(commits []commit, refs []ref) layout {
	lay := layout{
		Positions: map[string]point{},
		Refs:      map[string][]ref{},
		BySHA:     map[string]commit{},
		Index:     map[string]int{},
	}
	for i, cm := range commits {
		lay.BySHA[cm.SHA] = cm
		lay.Index[cm.SHA] = i
	}
	for _, r := range refs {
		if _, ok := lay.BySHA[r.Target]; ok {
			lay.Refs[r.Target] = append(lay.Refs[r.Target], r)
		}
	}
	for sha := range lay.Refs {
		sortRefs(lay.Refs[sha])
	}

	laneBySHA := map[string]int{}
	nextLane := 0
	assign := func(sha string) int {
		if lane, ok := laneBySHA[sha]; ok {
			return lane
		}
		lane := nextLane
		nextLane++
		laneBySHA[sha] = lane
		return lane
	}
	seedRefs := append([]ref(nil), refs...)
	sortRefs(seedRefs)
	for _, r := range seedRefs {
		if _, ok := lay.BySHA[r.Target]; !ok {
			continue
		}
		if r.Role == "trunk" || r.Current || r.Role == "deploy" || r.Kind == "branch" {
			assign(r.Target)
		}
	}
	for _, cm := range commits {
		lane := assign(cm.SHA)
		for i, parent := range cm.Parents {
			if _, ok := lay.BySHA[parent]; !ok {
				continue
			}
			if _, exists := laneBySHA[parent]; exists {
				continue
			}
			if i == 0 {
				laneBySHA[parent] = lane
			} else {
				assign(parent)
			}
		}
	}

	const laneWidth = 132.0
	const rowHeight = 86.0
	const left = 74.0
	const top = 64.0
	for i, cm := range commits {
		lane := laneBySHA[cm.SHA]
		lay.Positions[cm.SHA] = point{
			X: left + float64(lane)*laneWidth,
			Y: top + float64(i)*rowHeight,
		}
	}
	lay.Lanes = maxInt(nextLane, 1)
	lay.Width = math.Max(920, left+float64(lay.Lanes)*laneWidth+560)
	lay.Height = math.Max(560, top+float64(len(commits))*rowHeight+90)
	return lay
}

func drawEdge(svg js.Value, lay layout, ed edge) {
	tail, ok := lay.Positions[ed.From]
	if !ok {
		return
	}
	head, ok := lay.Positions[ed.To]
	if !ok {
		head = point{X: tail.X, Y: tail.Y + 68}
	}
	const radius = 17.0
	start := point{X: tail.X, Y: tail.Y + radius + 1}
	end := point{X: head.X, Y: head.Y - radius - 2}
	d := fmt.Sprintf("M%.1f %.1f C%.1f %.1f %.1f %.1f %.1f %.1f",
		start.X, start.Y,
		start.X, start.Y+48,
		end.X, end.Y-48,
		end.X, end.Y,
	)
	path := svgEl("path")
	setAttrs(path, map[string]string{
		"class":      "edge",
		"d":          d,
		"marker-end": "url(#arrowhead)",
	})
	if ed.Shallow {
		path.Call("setAttribute", "stroke-dasharray", "6 7")
		path.Call("setAttribute", "opacity", "0.55")
	}
	svg.Call("appendChild", path)
}

func drawCommit(svg js.Value, lay layout, stageID string, cm commit) {
	pos := lay.Positions[cm.SHA]
	refs := lay.Refs[cm.SHA]
	g := svgEl("g")
	setAttrs(g, map[string]string{"class": "commit-node"})
	handler := js.FuncOf(func(this js.Value, args []js.Value) any {
		selectCommit(cm, refs)
		return nil
	})
	eventHandlers[stageID] = append(eventHandlers[stageID], handler)
	g.Call("addEventListener", "click", handler)

	circle := svgEl("circle")
	setAttrs(circle, map[string]string{
		"cx":           f(pos.X),
		"cy":           f(pos.Y),
		"r":            "17",
		"fill":         commitColor(refs),
		"stroke":       "#fff",
		"stroke-width": "2",
	})
	if hasRemoteOnly(refs) {
		circle.Call("setAttribute", "stroke-dasharray", "4 4")
	}
	sha := svgEl("text")
	setAttrs(sha, map[string]string{
		"class":             "commit-sha",
		"x":                 f(pos.X),
		"y":                 f(pos.Y + 1),
		"text-anchor":       "middle",
		"dominant-baseline": "middle",
		"font-size":         "9",
	})
	sha.Set("textContent", shortNode(cm.Short))
	subject := svgEl("text")
	setAttrs(subject, map[string]string{
		"class": "commit-subject",
		"x":     f(pos.X + 31),
		"y":     f(pos.Y + 5),
	})
	subject.Set("textContent", truncate(cm.Subject, 72))
	title := svgEl("title")
	title.Set("textContent", cm.Short+" "+cm.Subject)

	g.Call("appendChild", title)
	g.Call("appendChild", circle)
	g.Call("appendChild", sha)
	g.Call("appendChild", subject)
	svg.Call("appendChild", g)
}

func drawRefs(svg js.Value, lay layout, sha string) {
	refs := lay.Refs[sha]
	if len(refs) == 0 {
		return
	}
	pos := lay.Positions[sha]
	for i, r := range refs {
		label := truncateRef(r.Name)
		width := math.Max(54, float64(len(label))*7.2+18)
		height := 22.0
		x := pos.X + 42
		y := pos.Y - 38 + float64(i)*25
		if i > 3 {
			x = pos.X - 42 - width
			y = pos.Y - 38 + float64(i-4)*25
		}
		g := svgEl("g")
		setAttrs(g, map[string]string{"class": "ref-label"})
		pointer := svgEl("path")
		if x > pos.X {
			setAttrs(pointer, map[string]string{
				"d":    fmt.Sprintf("M%.1f %.1f L%.1f %.1f L%.1f %.1f Z", pos.X+20, pos.Y, x, y+height/2-6, x, y+height/2+6),
				"fill": refColor(r),
			})
		} else {
			setAttrs(pointer, map[string]string{
				"d":    fmt.Sprintf("M%.1f %.1f L%.1f %.1f L%.1f %.1f Z", pos.X-20, pos.Y, x+width, y+height/2-6, x+width, y+height/2+6),
				"fill": refColor(r),
			})
		}
		rect := svgEl("rect")
		setAttrs(rect, map[string]string{
			"x":            f(x),
			"y":            f(y),
			"width":        f(width),
			"height":       f(height),
			"rx":           "4",
			"fill":         refColor(r),
			"stroke":       "#fff",
			"stroke-width": "2",
		})
		if r.Kind == "remote" {
			rect.Call("setAttribute", "stroke-dasharray", "4 3")
		}
		text := svgEl("text")
		setAttrs(text, map[string]string{
			"x":                 f(x + 9),
			"y":                 f(y + height/2 + 1),
			"dominant-baseline": "middle",
		})
		text.Set("textContent", label)
		title := svgEl("title")
		title.Set("textContent", r.Full)
		g.Call("appendChild", title)
		g.Call("appendChild", pointer)
		g.Call("appendChild", rect)
		g.Call("appendChild", text)
		svg.Call("appendChild", g)
	}
}

func addDefs(svg js.Value) {
	defs := svgEl("defs")
	marker := svgEl("marker")
	setAttrs(marker, map[string]string{
		"id":           "arrowhead",
		"viewBox":      "0 0 10 10",
		"refX":         "5",
		"refY":         "5",
		"markerWidth":  "5",
		"markerHeight": "5",
		"orient":       "auto",
	})
	p := svgEl("path")
	setAttrs(p, map[string]string{"d": "M 0 0 L 10 5 L 0 10 z", "class": "edge-arrow"})
	marker.Call("appendChild", p)
	defs.Call("appendChild", marker)
	svg.Call("appendChild", defs)
}

func selectCommit(cm commit, refs []ref) {
	names := make([]string, 0, len(refs))
	for _, r := range refs {
		names = append(names, r.Name)
	}
	payload, _ := json.Marshal(map[string]any{
		"sha":     cm.SHA,
		"short":   cm.Short,
		"subject": cm.Subject,
		"author":  cm.Author,
		"time":    cm.Time,
		"parents": cm.Parents,
		"refs":    names,
	})
	host := js.Global().Get("tbdVisualHost")
	if host.Truthy() && host.Get("selectCommit").Truthy() {
		host.Call("selectCommit", string(payload))
	}
}

func showError(err error) {
	showErrorTo("graph-stage", err)
}

func showErrorTo(stageID string, err error) {
	stage := js.Global().Get("document").Call("getElementById", stageID)
	if !stage.Truthy() {
		return
	}
	stage.Set("innerHTML", "")
	div := js.Global().Get("document").Call("createElement", "div")
	div.Set("className", "loading")
	div.Set("textContent", err.Error())
	stage.Call("appendChild", div)
}

func sortRefs(refs []ref) {
	roleRank := map[string]int{"head": 0, "trunk": 1, "deploy": 2, "release": 3, "lock": 4, "state": 5, "": 6}
	kindRank := map[string]int{"head": 0, "branch": 1, "tag": 2, "remote": 3, "stash": 4, "lock": 5, "tbd": 6, "ref": 7}
	sort.SliceStable(refs, func(i, j int) bool {
		ri, rj := rank(roleRank, refs[i].Role), rank(roleRank, refs[j].Role)
		if ri != rj {
			return ri < rj
		}
		ki, kj := rank(kindRank, refs[i].Kind), rank(kindRank, refs[j].Kind)
		if ki != kj {
			return ki < kj
		}
		return refs[i].Name < refs[j].Name
	})
}

func commitColor(refs []ref) string {
	for _, r := range refs {
		if r.Kind == "head" {
			return "#30f03d"
		}
	}
	for _, r := range refs {
		if r.Role == "deploy" {
			return "#ff5ec4"
		}
		if r.Role == "release" {
			return "#ffd347"
		}
		if r.Role == "trunk" {
			return "#25d8ff"
		}
		if r.Kind == "branch" {
			return "#7278ff"
		}
		if r.Kind == "tag" {
			return "#dce7f0"
		}
	}
	return "#5cbcfc"
}

func refColor(r ref) string {
	if r.Kind == "head" {
		return "#7278ff"
	}
	switch r.Role {
	case "trunk":
		return "#30f03d"
	case "deploy":
		return "#ff5ec4"
	case "release":
		return "#ffd347"
	case "lock":
		return "#ff6969"
	}
	switch r.Kind {
	case "branch":
		return branchColor(r.Name)
	case "remote":
		return "#8fa3b8"
	case "tag":
		return "#dce7f0"
	default:
		return "#25d8ff"
	}
}

func branchColor(name string) string {
	palette := []string{"#0074d9", "#ff851b", "#2ecc40", "#b10dc9", "#39cccc", "#f012be", "#4682b4", "#20b2aa"}
	if name == "main" || name == "develop" {
		return "#30f03d"
	}
	h := 0
	for _, r := range name {
		h = h*31 + int(r)
	}
	if h < 0 {
		h = -h
	}
	return palette[h%len(palette)]
}

func hasRemoteOnly(refs []ref) bool {
	hasRemote := false
	for _, r := range refs {
		if r.Kind == "branch" || r.Kind == "tag" || r.Kind == "head" {
			return false
		}
		if r.Kind == "remote" {
			hasRemote = true
		}
	}
	return hasRemote
}

func svgEl(name string) js.Value {
	return js.Global().Get("document").Call("createElementNS", svgNS, name)
}

func setAttrs(v js.Value, attrs map[string]string) {
	for k, val := range attrs {
		v.Call("setAttribute", k, val)
	}
}

func f(v float64) string {
	return fmt.Sprintf("%.1f", v)
}

func rank(m map[string]int, key string) int {
	if v, ok := m[key]; ok {
		return v
	}
	return 99
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func shortNode(s string) string {
	if len(s) <= 4 {
		return s
	}
	return s[:4]
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n <= 3 {
		return s[:n]
	}
	return s[:n-3] + "..."
}

func truncateRef(s string) string {
	s = strings.TrimSpace(s)
	if len(s) <= 30 {
		return s
	}
	return s[:12] + "..." + s[len(s)-14:]
}
