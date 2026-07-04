package app

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/spf13/cobra"

	"goforge.dev/tbd/v2/internal/v2/gitops"
)

func newSyncCommand(opts *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "sync",
		Short: "sync current repository state with remote refs and invalidate stale UAT",
		RunE: func(cmd *cobra.Command, args []string) error {
			e, err := loadEnv(cmd, opts)
			if err != nil {
				return err
			}
			if err := syncRemoteState(e); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "synced")
			return nil
		},
	}
}

func newGraphCommand(opts *rootOptions) *cobra.Command {
	var limit int
	cmd := &cobra.Command{
		Use:   "graph",
		Short: "render a terminal DAG horizon view",
		RunE: func(cmd *cobra.Command, args []string) error {
			e, err := loadEnv(cmd, opts)
			if err != nil {
				return err
			}
			g, err := e.Repo.DecoratedGraph(limit)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "trunk: %s\nremote: %s\ndeploy refs: %s\n\n%s\n",
				e.Config.TrunkName, e.Config.Remote, strings.Join(e.Config.Deploy.Refs, ", "), g)
			return nil
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 80, "maximum commits to render")
	return cmd
}

func newServeCommand(opts *rootOptions) *cobra.Command {
	var addr string
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "serve an in-browser DAG view and tbd command console",
		RunE: func(cmd *cobra.Command, args []string) error {
			e, err := loadEnv(cmd, opts)
			if err != nil {
				return err
			}
			interval, _ := time.ParseDuration(e.Config.Visualize.FetchInterval)
			if interval <= 0 {
				interval = 15 * time.Second
			}
			exe, _ := os.Executable()
			mux := newServeMux(e, exe)
			go func() {
				t := time.NewTicker(interval)
				defer t.Stop()
				for range t.C {
					_ = e.Fetch()
				}
			}()
			fmt.Fprintf(cmd.OutOrStdout(), "serving %s\n", addr)
			return http.ListenAndServe(addr, mux)
		},
	}
	cmd.Flags().StringVar(&addr, "addr", "127.0.0.1:8087", "listen address")
	return cmd
}

func newServeMux(e gitops.Env, exe string) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		_ = e.Fetch()
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = visualPage.Execute(w, map[string]any{
			"Root":   e.Root,
			"Trunk":  e.Config.TrunkName,
			"Deploy": strings.Join(e.Config.Deploy.Refs, ", "),
		})
	})
	mux.HandleFunc("/api/graph", func(w http.ResponseWriter, r *http.Request) {
		_ = e.Fetch()
		limit := graphLimit(r, 240)
		graph, err := buildVisualGraph(e, limit)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		_ = enc.Encode(graph)
	})
	mux.HandleFunc("/graph", func(w http.ResponseWriter, r *http.Request) {
		_ = e.Fetch()
		graph, _ := e.Repo.DecoratedGraph(graphLimit(r, 120))
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte(graph))
	})
	mux.HandleFunc("/assets/app.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write([]byte(visualAppJS))
	})
	mux.HandleFunc("/assets/wasm_exec.js", serveWasmExecJS)
	mux.HandleFunc("/assets/tbd_visual.wasm", serveVisualWASM)
	mux.HandleFunc("/run", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		out, err := runBrowserCommand(e, exe, r.FormValue("cmd"))
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			if strings.TrimSpace(out) != "" {
				fmt.Fprintf(w, "%s\n%v", out, err)
			} else {
				fmt.Fprint(w, err)
			}
			return
		}
		_, _ = w.Write([]byte(out))
	})
	return mux
}

func graphLimit(r *http.Request, fallback int) int {
	n, err := strconv.Atoi(r.URL.Query().Get("limit"))
	if err != nil || n <= 0 {
		return fallback
	}
	if n > 2000 {
		return 2000
	}
	return n
}

func runBrowserCommand(e gitops.Env, exe, raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	parts, err := splitCommandLine(raw)
	if err != nil {
		return "", err
	}
	if len(parts) == 0 {
		return "", nil
	}
	name := exe
	args := parts
	switch parts[0] {
	case "tbd":
		args = parts[1:]
	case "git":
		name = "git"
		args = parts[1:]
	}
	c := exec.Command(name, args...)
	c.Dir = e.Root
	out, err := c.CombinedOutput()
	if err != nil {
		return string(out), err
	}
	return string(out), nil
}

func splitCommandLine(s string) ([]string, error) {
	var out []string
	var b strings.Builder
	var quote rune
	escaped := false
	inArg := false
	flush := func() {
		if inArg {
			out = append(out, b.String())
			b.Reset()
			inArg = false
		}
	}
	for _, r := range s {
		if escaped {
			b.WriteRune(r)
			escaped = false
			inArg = true
			continue
		}
		if r == '\\' {
			escaped = true
			inArg = true
			continue
		}
		if quote != 0 {
			if r == quote {
				quote = 0
				continue
			}
			b.WriteRune(r)
			inArg = true
			continue
		}
		switch {
		case r == '\'' || r == '"':
			quote = r
			inArg = true
		case unicode.IsSpace(r):
			flush()
		default:
			b.WriteRune(r)
			inArg = true
		}
	}
	if escaped {
		b.WriteRune('\\')
	}
	if quote != 0 {
		return nil, fmt.Errorf("unterminated quote")
	}
	flush()
	return out, nil
}

var visualWASM struct {
	once sync.Once
	data []byte
	err  error
}

func serveWasmExecJS(w http.ResponseWriter, r *http.Request) {
	path := filepath.Join(runtime.GOROOT(), "lib", "wasm", "wasm_exec.js")
	data, err := os.ReadFile(path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(data)
}

func serveVisualWASM(w http.ResponseWriter, r *http.Request) {
	data, err := visualWASMBytes()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/wasm")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(data)
}

func visualWASMBytes() ([]byte, error) {
	visualWASM.once.Do(func() {
		visualWASM.data, visualWASM.err = buildVisualWASM()
	})
	return visualWASM.data, visualWASM.err
}

func buildVisualWASM() ([]byte, error) {
	tmp, err := os.CreateTemp("", "tbd-visual-*.wasm")
	if err != nil {
		return nil, err
	}
	path := tmp.Name()
	_ = tmp.Close()
	defer os.Remove(path)

	root, ok := moduleRoot()
	pkg := "goforge.dev/tbd/v2/internal/app/visualwasm"
	args := []string{"build", "-trimpath", "-o", path, pkg}
	cmd := exec.Command("go", args...)
	cmd.Env = append(os.Environ(), "GOOS=js", "GOARCH=wasm")
	if ok {
		cmd.Dir = root
		args[len(args)-1] = "./internal/app/visualwasm"
		cmd.Args = append([]string{"go"}, args...)
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("build visual wasm: %v\n%s", err, out)
	}
	return os.ReadFile(path)
}

func moduleRoot() (string, bool) {
	_, file, _, ok := runtime.Caller(0)
	if !ok || !filepath.IsAbs(file) {
		return "", false
	}
	for dir := filepath.Dir(file); ; dir = filepath.Dir(dir) {
		data, err := os.ReadFile(filepath.Join(dir, "go.mod"))
		if err == nil && strings.Contains(string(data), "module goforge.dev/tbd/v2") {
			return dir, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
	}
}
