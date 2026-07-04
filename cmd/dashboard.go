package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/nuelScript/skiff/internal/docker"
	"github.com/nuelScript/skiff/internal/proxy"
	"github.com/nuelScript/skiff/internal/registry"
	"github.com/nuelScript/skiff/internal/ui"
	"github.com/spf13/cobra"
)

func newDashboardCmd() *cobra.Command {
	var addr string
	cmd := &cobra.Command{
		Use:   "dashboard",
		Short: "Serve a local web dashboard of your apps",
		RunE: func(cmd *cobra.Command, args []string) error {
			mux := http.NewServeMux()
			mux.HandleFunc("/api/apps", handleAppsAPI)
			mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/" {
					http.NotFound(w, r)
					return
				}
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				fmt.Fprint(w, dashboardHTML)
			})

			ui.Banner(version)
			ui.Field("dashboard", "http://localhost"+addr)
			ui.Note("live view of your apps  (ctrl-c to stop)")
			fmt.Println()
			return http.ListenAndServe(addr, mux)
		},
	}
	cmd.Flags().StringVar(&addr, "addr", ":4000", "address to listen on")
	return cmd
}

type appStatus struct {
	Name     string `json:"name"`
	Target   string `json:"target"`
	State    string `json:"state"`
	Health   string `json:"health"`
	URL      string `json:"url"`
	HostPort int    `json:"hostPort"`
}

func handleAppsAPI(w http.ResponseWriter, r *http.Request) {
	apps, err := registry.List()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	out := make([]appStatus, 0, len(apps))
	for _, a := range apps {
		state := docker.For(a.Host).State(a.Container)

		probeHost := "127.0.0.1"
		url := proxy.URL(a.Name)
		target := "local"
		if a.Host != "" {
			probeHost = docker.SSHHostname(a.Host)
			url = fmt.Sprintf("http://%s:%d", probeHost, a.HostPort)
			target = a.Host
		}

		health := "—"
		if state == "running" {
			if probe(probeHost, a.HostPort) {
				health = "healthy"
			} else {
				health = "unreachable"
			}
		}

		out = append(out, appStatus{
			Name:     a.Name,
			Target:   target,
			State:    state,
			Health:   health,
			URL:      url,
			HostPort: a.HostPort,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

const dashboardHTML = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Skiff</title>
<style>
  :root { --bg:#0b0b0f; --card:#15151c; --line:#26262f; --text:#e5e5ea; --muted:#8a8a99; --accent:#7C3AED; --green:#16A34A; --red:#DC2626; }
  * { box-sizing:border-box; }
  body { margin:0; background:var(--bg); color:var(--text); font:14px/1.5 ui-monospace,SFMono-Regular,Menlo,monospace; }
  header { display:flex; align-items:baseline; gap:12px; padding:24px 28px; border-bottom:1px solid var(--line); }
  header h1 { margin:0; font-size:18px; color:var(--accent); letter-spacing:.5px; }
  header .count { color:var(--muted); font-size:13px; }
  main { padding:20px 28px; display:grid; gap:12px; grid-template-columns:repeat(auto-fill,minmax(320px,1fr)); }
  .card { background:var(--card); border:1px solid var(--line); border-radius:10px; padding:16px 18px; }
  .top { display:flex; justify-content:space-between; align-items:center; margin-bottom:12px; }
  .name { font-size:15px; font-weight:600; }
  .badge { font-size:11px; padding:2px 9px; border-radius:999px; border:1px solid var(--line); color:var(--muted); text-transform:uppercase; letter-spacing:.5px; }
  .badge.running { color:var(--green); border-color:var(--green); }
  .badge.exited, .badge.missing { color:var(--red); border-color:var(--red); }
  .row { display:flex; justify-content:space-between; color:var(--muted); font-size:12.5px; padding:2px 0; }
  .row b { color:var(--text); font-weight:500; }
  a { color:var(--accent); text-decoration:none; }
  a:hover { text-decoration:underline; }
  .empty { color:var(--muted); padding:48px; text-align:center; grid-column:1/-1; }
</style>
</head>
<body>
<header><h1>Skiff</h1><span class="count" id="count"></span></header>
<main id="apps"></main>
<script>
function esc(s){ return String(s).replace(/[&<>"]/g, function(c){ return {'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;'}[c]; }); }
async function load(){
  try {
    var apps = await (await fetch('/api/apps')).json();
    document.getElementById('count').textContent = apps.length ? apps.length + (apps.length>1?' apps':' app') : '';
    var main = document.getElementById('apps');
    if(!apps.length){ main.innerHTML = '<div class="empty">No apps deployed yet &mdash; run <b>skiff deploy</b>.</div>'; return; }
    main.innerHTML = apps.map(function(a){
      var host = esc(a.url.replace(/^https?:\/\//,''));
      return '<div class="card"><div class="top"><span class="name">'+esc(a.name)+'</span>'
        + '<span class="badge '+esc(a.state)+'">'+esc(a.state)+'</span></div>'
        + '<div class="row"><span>health</span><b>'+esc(a.health)+'</b></div>'
        + '<div class="row"><span>target</span><b>'+esc(a.target)+'</b></div>'
        + '<div class="row"><span>url</span><a href="'+esc(a.url)+'" target="_blank">'+host+'</a></div>'
        + '</div>';
    }).join('');
  } catch(e){}
}
load(); setInterval(load, 3000);
</script>
</body>
</html>`
