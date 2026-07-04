package app

import (
	"sort"
	"strings"
	"time"

	"goforge.dev/tbd/v2/internal/git"
	"goforge.dev/tbd/v2/internal/v2/gitops"
	v2state "goforge.dev/tbd/v2/internal/v2/state"
)

type visualGraphSnapshot struct {
	Root        string              `json:"root"`
	GeneratedAt string              `json:"generatedAt"`
	Config      visualGraphConfig   `json:"config"`
	Status      visualGraphStatus   `json:"status"`
	Commits     []visualCommit      `json:"commits"`
	Refs        []visualRef         `json:"refs"`
	Edges       []visualEdge        `json:"edges"`
	Workflow    visualWorkflowState `json:"workflow"`
	Raw         string              `json:"raw"`
	Notice      string              `json:"notice"`
}

type visualGraphConfig struct {
	TrunkName       string   `json:"trunkName"`
	Remote          string   `json:"remote"`
	DeployStrategy  string   `json:"deployStrategy"`
	DeployRefs      []string `json:"deployRefs"`
	ReleaseStrategy string   `json:"releaseStrategy"`
	ReleasePrefix   string   `json:"releasePrefix"`
}

type visualGraphStatus struct {
	Branch        string `json:"branch"`
	Detached      bool   `json:"detached"`
	Head          string `json:"head"`
	Clean         bool   `json:"clean"`
	Rebase        bool   `json:"rebase"`
	CherryPick    bool   `json:"cherryPick"`
	RemoteEnabled bool   `json:"remoteEnabled"`
}

type visualCommit struct {
	SHA     string   `json:"sha"`
	Short   string   `json:"short"`
	Parents []string `json:"parents"`
	Author  string   `json:"author"`
	Email   string   `json:"email"`
	Time    string   `json:"time"`
	Unix    int64    `json:"unix"`
	Subject string   `json:"subject"`
}

type visualRef struct {
	Full    string `json:"full"`
	Name    string `json:"name"`
	Kind    string `json:"kind"`
	Role    string `json:"role,omitempty"`
	Target  string `json:"target"`
	Current bool   `json:"current,omitempty"`
	Remote  string `json:"remote,omitempty"`
}

type visualEdge struct {
	From    string `json:"from"`
	To      string `json:"to"`
	Shallow bool   `json:"shallow,omitempty"`
}

type visualWorkflowState struct {
	Items    []v2state.Item         `json:"items"`
	Groups   []v2state.Group        `json:"groups"`
	UAT      []v2state.UATState     `json:"uat"`
	Releases []v2state.ReleaseDraft `json:"releases"`
	Events   []v2state.ReleaseEvent `json:"events"`
}

func buildVisualGraph(e gitops.Env, limit int) (visualGraphSnapshot, error) {
	commits, err := e.Repo.GraphCommits(limit)
	if err != nil {
		return visualGraphSnapshot{}, err
	}
	refs, err := e.Repo.GraphRefs()
	if err != nil {
		return visualGraphSnapshot{}, err
	}
	raw, _ := e.Repo.DecoratedGraph(limit)

	head, _ := e.Repo.RevParse("HEAD")
	branch, berr := e.Repo.CurrentBranch()
	clean, _ := e.Repo.IsClean()
	commitSet := map[string]bool{}

	outCommits := make([]visualCommit, 0, len(commits))
	for _, cm := range commits {
		commitSet[cm.SHA] = true
		outCommits = append(outCommits, visualCommit{
			SHA:     cm.SHA,
			Short:   cm.Short,
			Parents: append([]string(nil), cm.Parents...),
			Author:  cm.Author,
			Email:   cm.Email,
			Time:    time.Unix(cm.Unix, 0).UTC().Format(time.RFC3339),
			Unix:    cm.Unix,
			Subject: cm.Subject,
		})
	}

	outRefs := make([]visualRef, 0, len(refs)+1)
	if head != "" {
		outRefs = append(outRefs, visualRef{
			Full:    "HEAD",
			Name:    "HEAD",
			Kind:    "head",
			Role:    "head",
			Target:  head,
			Current: true,
		})
	}
	for _, ref := range refs {
		v := classifyVisualRef(e, ref)
		if v.Target == "" {
			continue
		}
		if v.Kind == "branch" && v.Name == branch {
			v.Current = true
		}
		if berr != nil && head != "" && v.Target == head {
			v.Current = true
		}
		outRefs = append(outRefs, v)
	}
	sortVisualRefs(outRefs)

	var edges []visualEdge
	for _, cm := range commits {
		for _, parent := range cm.Parents {
			edges = append(edges, visualEdge{
				From:    cm.SHA,
				To:      parent,
				Shallow: !commitSet[parent],
			})
		}
	}

	return visualGraphSnapshot{
		Root:        e.Root,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Config: visualGraphConfig{
			TrunkName:       e.Config.TrunkName,
			Remote:          e.Config.Remote,
			DeployStrategy:  e.Config.Deploy.Strategy,
			DeployRefs:      append([]string(nil), e.Config.Deploy.Refs...),
			ReleaseStrategy: e.Config.Release.Strategy,
			ReleasePrefix:   e.Config.Release.BranchPrefix,
		},
		Status: visualGraphStatus{
			Branch:        branch,
			Detached:      berr != nil,
			Head:          head,
			Clean:         clean,
			Rebase:        e.Repo.RebaseInProgress(),
			CherryPick:    e.Repo.CherryPickInProgress(),
			RemoteEnabled: e.RemoteOK,
		},
		Commits:  outCommits,
		Refs:     outRefs,
		Edges:    edges,
		Workflow: loadVisualWorkflow(e.Root),
		Raw:      raw,
		Notice:   "Visual style inspired by LearnGitBranching, MIT licensed by Peter Cottle.",
	}, nil
}

func classifyVisualRef(e gitops.Env, ref git.GraphRef) visualRef {
	v := visualRef{
		Full:   ref.Full,
		Name:   ref.Short,
		Kind:   "ref",
		Target: ref.Target,
	}
	switch {
	case strings.HasPrefix(ref.Full, "refs/heads/"):
		v.Kind = "branch"
		v.Name = strings.TrimPrefix(ref.Full, "refs/heads/")
	case strings.HasPrefix(ref.Full, "refs/remotes/"):
		v.Kind = "remote"
		v.Name = strings.TrimPrefix(ref.Full, "refs/remotes/")
		if remote, _, ok := strings.Cut(v.Name, "/"); ok {
			v.Remote = remote
		}
	case strings.HasPrefix(ref.Full, "refs/tags/"):
		v.Kind = "tag"
		v.Name = strings.TrimPrefix(ref.Full, "refs/tags/")
	case ref.Full == "refs/stash":
		v.Kind = "stash"
		v.Name = "stash"
	case strings.HasPrefix(ref.Full, e.Config.Locks.RefPrefix):
		v.Kind = "lock"
		v.Role = "lock"
		v.Name = strings.TrimPrefix(ref.Full, e.Config.Locks.RefPrefix)
	case strings.HasPrefix(ref.Full, "refs/tbd/"):
		v.Kind = "tbd"
		v.Role = "state"
		v.Name = strings.TrimPrefix(ref.Full, "refs/tbd/")
	}

	if v.Kind == "branch" && v.Name == e.Config.TrunkName {
		v.Role = "trunk"
	}
	if v.Kind == "remote" && v.Name == e.Config.Remote+"/"+e.Config.TrunkName {
		v.Role = "trunk"
	}
	if isDeployRef(e, v) {
		v.Role = "deploy"
	}
	if v.Kind == "branch" && strings.HasPrefix(v.Name, e.Config.Release.BranchPrefix) {
		v.Role = "release"
	}
	if v.Kind == "tag" && (strings.HasPrefix(v.Name, "v") || strings.HasPrefix(v.Name, "rc-")) {
		v.Role = "release"
	}
	return v
}

func isDeployRef(e gitops.Env, ref visualRef) bool {
	for _, name := range e.Config.Deploy.Refs {
		if ref.Name == name {
			return true
		}
		if ref.Kind == "remote" && ref.Name == e.Config.Remote+"/"+name {
			return true
		}
	}
	return false
}

func sortVisualRefs(refs []visualRef) {
	kindRank := map[string]int{
		"head":   0,
		"branch": 1,
		"tag":    2,
		"remote": 3,
		"stash":  4,
		"lock":   5,
		"tbd":    6,
		"ref":    7,
	}
	roleRank := map[string]int{
		"head":    0,
		"trunk":   1,
		"deploy":  2,
		"release": 3,
		"lock":    4,
		"state":   5,
		"":        6,
	}
	sort.SliceStable(refs, func(i, j int) bool {
		if roleRank[refs[i].Role] != roleRank[refs[j].Role] {
			return roleRank[refs[i].Role] < roleRank[refs[j].Role]
		}
		if kindRank[refs[i].Kind] != kindRank[refs[j].Kind] {
			return kindRank[refs[i].Kind] < kindRank[refs[j].Kind]
		}
		return refs[i].Name < refs[j].Name
	})
}

func loadVisualWorkflow(root string) visualWorkflowState {
	st, _ := v2state.Load(root)
	book, _ := v2state.LoadRelease(root)
	items := make([]v2state.Item, 0, len(st.Items))
	for _, it := range st.Items {
		items = append(items, it)
	}
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].TouchedAt > items[j].TouchedAt
	})
	groups := make([]v2state.Group, 0, len(st.Groups))
	for _, g := range st.Groups {
		groups = append(groups, g)
	}
	sort.SliceStable(groups, func(i, j int) bool {
		return groups[i].UpdatedAt > groups[j].UpdatedAt
	})
	uat := make([]v2state.UATState, 0, len(st.UAT))
	for _, u := range st.UAT {
		uat = append(uat, u)
	}
	sort.SliceStable(uat, func(i, j int) bool {
		return uat[i].UpdatedAt > uat[j].UpdatedAt
	})
	return visualWorkflowState{
		Items:    items,
		Groups:   groups,
		UAT:      uat,
		Releases: book.Drafts,
		Events:   book.Events,
	}
}
