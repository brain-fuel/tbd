// Package state persists tbd v2 workflow metadata in repository files.
package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"goforge.dev/goplus/std/fsatomic"
)

const (
	DirName         = ".tbd"
	StateFileName   = "state.json"
	ReleaseJSONName = "RELEASE.json"
	ReleaseMDName   = "RELEASE.md"
)

type State struct {
	Version   int                 `json:"version"`
	UpdatedAt string              `json:"updated_at"`
	Items     map[string]Item     `json:"items"`
	Groups    map[string]Group    `json:"groups"`
	UAT       map[string]UATState `json:"uat"`
}

type Item struct {
	ID        string `json:"id,omitempty"`
	Kind      string `json:"kind"`
	Desc      string `json:"desc"`
	Branch    string `json:"branch"`
	Commit    string `json:"commit,omitempty"`
	Status    string `json:"status"`
	TouchedAt string `json:"touched_at"`
}

type Group struct {
	Name      string   `json:"name"`
	Kind      string   `json:"kind"` // collab | stack
	Branch    string   `json:"branch"`
	ItemIDs   []string `json:"item_ids"`
	UpdatedAt string   `json:"updated_at"`
}

type UATState struct {
	Semver       string `json:"semver"`
	CandidateRef string `json:"candidate_ref"`
	Commit       string `json:"commit"`
	Valid        bool   `json:"valid"`
	Reason       string `json:"reason,omitempty"`
	UpdatedAt    string `json:"updated_at"`
}

type ReleaseBook struct {
	Version int            `json:"version"`
	Drafts  []ReleaseDraft `json:"drafts"`
	Events  []ReleaseEvent `json:"events"`
}

type ReleaseDraft struct {
	Semver    string        `json:"semver"`
	Branch    string        `json:"branch,omitempty"`
	Status    string        `json:"status"`
	Items     []ReleaseItem `json:"items"`
	Notes     string        `json:"notes"`
	UpdatedAt string        `json:"updated_at"`
}

type ReleaseItem struct {
	ID     string `json:"id,omitempty"`
	Kind   string `json:"kind"`
	Desc   string `json:"desc"`
	Commit string `json:"commit,omitempty"`
}

type ReleaseEvent struct {
	Time        string        `json:"time"`
	Type        string        `json:"type"`
	Semver      string        `json:"semver,omitempty"`
	Ref         string        `json:"ref,omitempty"`
	Commit      string        `json:"commit,omitempty"`
	Items       []ReleaseItem `json:"items,omitempty"`
	Explanation string        `json:"explanation,omitempty"`
}

func New() State {
	return State{
		Version: 2,
		Items:   map[string]Item{},
		Groups:  map[string]Group{},
		UAT:     map[string]UATState{},
	}
}

func Load(root string) (State, error) {
	path := filepath.Join(root, DirName, StateFileName)
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return New(), nil
	}
	if err != nil {
		return State{}, err
	}
	st := New()
	if err := json.Unmarshal(data, &st); err != nil {
		return State{}, fmt.Errorf("parse %s: %w", path, err)
	}
	if st.Items == nil {
		st.Items = map[string]Item{}
	}
	if st.Groups == nil {
		st.Groups = map[string]Group{}
	}
	if st.UAT == nil {
		st.UAT = map[string]UATState{}
	}
	return st, nil
}

func Save(root string, st State) error {
	st.Version = 2
	st.UpdatedAt = now()
	path := filepath.Join(root, DirName, StateFileName)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return fsatomic.WriteFile(path, data, 0o644)
}

func LoadRelease(root string) (ReleaseBook, error) {
	path := filepath.Join(root, ReleaseJSONName)
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return ReleaseBook{Version: 2}, nil
	}
	if err != nil {
		return ReleaseBook{}, err
	}
	book := ReleaseBook{Version: 2}
	if err := json.Unmarshal(data, &book); err != nil {
		return ReleaseBook{}, fmt.Errorf("parse %s: %w", path, err)
	}
	return book, nil
}

func SaveRelease(root string, book ReleaseBook) error {
	book.Version = 2
	data, err := json.MarshalIndent(book, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := fsatomic.WriteFile(filepath.Join(root, ReleaseJSONName), data, 0o644); err != nil {
		return err
	}
	return fsatomic.WriteFile(filepath.Join(root, ReleaseMDName), []byte(RenderMarkdown(book)), 0o644)
}

func RenderMarkdown(book ReleaseBook) string {
	var b strings.Builder
	b.WriteString("# Release Notes\n\n")
	if len(book.Drafts) > 0 {
		drafts := append([]ReleaseDraft(nil), book.Drafts...)
		sort.SliceStable(drafts, func(i, j int) bool { return drafts[i].UpdatedAt > drafts[j].UpdatedAt })
		for _, d := range drafts {
			fmt.Fprintf(&b, "## %s (%s)\n\n", empty(d.Semver, "unversioned"), empty(d.Status, "draft"))
			if strings.TrimSpace(d.Notes) != "" {
				b.WriteString(strings.TrimSpace(d.Notes))
				b.WriteString("\n\n")
			}
			for _, it := range d.Items {
				id := it.ID
				if id != "" {
					id += ": "
				}
				fmt.Fprintf(&b, "- %s%s\n", id, it.Desc)
			}
			b.WriteString("\n")
		}
	}
	if len(book.Events) > 0 {
		b.WriteString("## Event Log\n\n")
		for i := len(book.Events) - 1; i >= 0; i-- {
			ev := book.Events[i]
			fmt.Fprintf(&b, "- %s `%s`", ev.Time, ev.Type)
			if ev.Semver != "" {
				fmt.Fprintf(&b, " %s", ev.Semver)
			}
			if ev.Ref != "" {
				fmt.Fprintf(&b, " `%s`", ev.Ref)
			}
			if ev.Explanation != "" {
				fmt.Fprintf(&b, ": %s", ev.Explanation)
			}
			b.WriteString("\n")
		}
	}
	return b.String()
}

func UpsertDraft(book *ReleaseBook, draft ReleaseDraft) {
	draft.UpdatedAt = now()
	for i := range book.Drafts {
		if book.Drafts[i].Semver == draft.Semver && draft.Semver != "" {
			book.Drafts[i] = draft
			return
		}
	}
	book.Drafts = append(book.Drafts, draft)
}

func AppendEvent(book *ReleaseBook, ev ReleaseEvent) {
	if ev.Time == "" {
		ev.Time = now()
	}
	book.Events = append(book.Events, ev)
}

func ItemKey(kind, id, desc string) string {
	if id != "" {
		return kind + ":" + id
	}
	return kind + ":" + Slug(desc)
}

func Slug(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	dash := false
	for _, r := range s {
		ok := r >= 'a' && r <= 'z' || r >= '0' && r <= '9'
		if ok {
			b.WriteRune(r)
			dash = false
			continue
		}
		if !dash && b.Len() > 0 {
			b.WriteByte('-')
			dash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

func GroupName(ids []string, suffix string) string {
	return "feature/" + strings.Join(ids, "-") + suffix
}

func now() string { return time.Now().UTC().Format(time.RFC3339) }

func empty(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}
