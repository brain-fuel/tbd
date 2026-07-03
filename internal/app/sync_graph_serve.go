package app

import (
	"fmt"
	"html/template"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/spf13/cobra"
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
			mux := http.NewServeMux()
			mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
				_ = e.Fetch()
				graph, _ := e.Repo.DecoratedGraph(120)
				_ = page.Execute(w, map[string]any{
					"Root":   e.Root,
					"Trunk":  e.Config.TrunkName,
					"Deploy": strings.Join(e.Config.Deploy.Refs, ", "),
					"Graph":  graph,
				})
			})
			mux.HandleFunc("/graph", func(w http.ResponseWriter, r *http.Request) {
				_ = e.Fetch()
				graph, _ := e.Repo.DecoratedGraph(120)
				_, _ = w.Write([]byte(graph))
			})
			mux.HandleFunc("/run", func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					http.Error(w, "POST only", http.StatusMethodNotAllowed)
					return
				}
				raw := strings.TrimSpace(r.FormValue("cmd"))
				if raw == "" {
					return
				}
				parts := strings.Fields(raw)
				if len(parts) > 0 && parts[0] == "tbd" {
					parts = parts[1:]
				}
				exe, _ := os.Executable()
				c := exec.Command(exe, parts...)
				c.Dir = e.Root
				out, err := c.CombinedOutput()
				if err != nil {
					fmt.Fprintf(w, "%s\n%v", out, err)
					return
				}
				_, _ = w.Write(out)
			})
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

var page = template.Must(template.New("page").Parse(`<!doctype html>
<html>
<head>
<meta charset="utf-8">
<title>tbd graph</title>
<style>
body{font-family:ui-monospace,SFMono-Regular,Menlo,monospace;margin:0;background:#101214;color:#e6e6e6}
header{padding:14px 18px;background:#1d2228;border-bottom:1px solid #333}
main{display:grid;grid-template-columns:1fr 380px;gap:0;min-height:calc(100vh - 52px)}
pre{margin:0;padding:18px;white-space:pre-wrap;line-height:1.35}
aside{border-left:1px solid #333;background:#15191e;padding:16px}
input,button,textarea{font:inherit}
textarea{width:100%;height:92px;background:#0d0f12;color:#e6e6e6;border:1px solid #444;padding:8px}
button{margin-top:8px;background:#d8e86f;color:#111;border:0;padding:7px 10px;cursor:pointer}
#out{margin-top:16px;border-top:1px solid #333;padding-top:12px}
</style>
</head>
<body>
<header>{{.Root}} · trunk {{.Trunk}} · deploy {{.Deploy}}</header>
<main>
<pre id="graph">{{.Graph}}</pre>
<aside>
<form id="f">
<textarea name="cmd" placeholder="tbd sync"></textarea>
<button>Run</button>
</form>
<pre id="out"></pre>
</aside>
</main>
<script>
async function refresh(){ const r=await fetch('/graph'); graph.textContent=await r.text(); }
setInterval(refresh, 4000);
f.onsubmit=async e=>{e.preventDefault(); const fd=new FormData(f); const r=await fetch('/run',{method:'POST',body:fd}); out.textContent=await r.text(); await refresh();};
</script>
</body>
</html>`))
