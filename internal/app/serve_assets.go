package app

import "html/template"

var visualPage = template.Must(template.New("visual-page").Parse(visualHTML))

const visualHTML = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<script>(function(){try{var t=localStorage.getItem("tbd-theme");document.documentElement.setAttribute("data-theme",t==="light"?"light":"dark")}catch(e){document.documentElement.setAttribute("data-theme","dark")}})();</script>
<title>tbd visualizer</title>
<style>
:root{color-scheme:dark;--bg:#02102e;--panel:#06214f;--panel-2:#0a2a63;--line:#173a72;--text:#e9eef7;--muted:#9fb2d6;--green:#30f03d;--cyan:#25d8ff;--yellow:#ffd347;--pink:#ff5ec4;--red:#ff6969;--blue:#7278ff}
:root[data-theme="light"]{color-scheme:light;--bg:#ffffff;--panel:#f5f8fd;--panel-2:#e9f0fa;--line:#d3ddf0;--text:#0a1f4d;--muted:#51617f}
*{box-sizing:border-box}
html,body{height:100%}
body{
  margin:0;
  height:100dvh;
  overflow:hidden;
  display:grid;
  grid-template-rows:auto minmax(0,1fr);
  background:var(--bg);
  color:var(--text);
  font-family:Inter,ui-sans-serif,system-ui,-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif;
  font-size:14px;
  letter-spacing:0;
}
button,input,textarea{font:inherit}
button{
  border:1px solid var(--line);
  background:var(--panel-2);
  color:var(--text);
  min-height:34px;
  padding:0 12px;
  border-radius:6px;
  cursor:pointer;
}
button:hover{border-color:var(--muted);background:var(--line)}
button.primary{background:var(--green);border-color:var(--green);color:var(--bg);font-weight:700}
button.icon{width:34px;padding:0}
.topbar{
  min-height:58px;
  display:grid;
  grid-template-columns:minmax(0,1fr) auto;
  align-items:center;
  gap:16px;
  padding:0 16px;
  background:var(--panel);
  border-bottom:1px solid var(--line);
  position:sticky;
  top:0;
  z-index:20;
}
.brand{display:flex;align-items:baseline;gap:12px;min-width:0}
.brand strong{font-size:16px}
.repo{color:var(--muted);font-family:ui-monospace,SFMono-Regular,Menlo,monospace;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
.toolbar{display:flex;align-items:center;gap:10px;flex-wrap:wrap;justify-content:flex-end}
.field{display:flex;align-items:center;gap:7px;color:var(--muted)}
.field input[type="number"]{width:82px}
.field input[type="range"]{width:104px}
input[type="number"],textarea{
  border:1px solid var(--line);
  background:var(--bg);
  color:var(--text);
  border-radius:6px;
  padding:8px;
}
.app{
  display:grid;
  grid-template-columns:minmax(0,1fr) minmax(340px,420px);
  min-height:0;
  overflow:hidden;
}
.workspace{min-width:0;min-height:0;display:grid;grid-template-rows:minmax(0,1fr) auto;background:var(--bg)}
.stage-wrap{min-height:0;overflow:auto;position:relative}
#graph-stage{min-width:100%;min-height:100%;padding:18px}
.loading{color:var(--muted);font-family:ui-monospace,SFMono-Regular,Menlo,monospace;padding:24px}
.raw-wrap{border-top:1px solid var(--line);background:var(--bg);max-height:210px;overflow:auto}
#raw-graph{margin:0;padding:12px 16px;color:var(--text);font:12px/1.35 ui-monospace,SFMono-Regular,Menlo,monospace;white-space:pre}
.side{
  min-width:0;
  min-height:0;
  border-left:1px solid var(--line);
  background:var(--panel);
  display:grid;
  grid-template-rows:auto auto minmax(0,1fr) auto;
  overflow:hidden;
}
.panel{border-bottom:1px solid var(--line);padding:14px}
.panel-title{font-size:12px;text-transform:uppercase;color:var(--muted);font-weight:700;margin-bottom:10px}
.status-grid{display:grid;grid-template-columns:1fr 1fr;gap:8px}
.metric{background:var(--bg);border:1px solid var(--line);border-radius:6px;padding:8px}
.metric b{display:block;color:var(--muted);font-size:11px;margin-bottom:3px}
.metric span{font-family:ui-monospace,SFMono-Regular,Menlo,monospace}
.filters{display:flex;flex-wrap:wrap;gap:8px}
.filters label{display:flex;align-items:center;gap:6px;background:var(--bg);border:1px solid var(--line);border-radius:6px;padding:6px 8px;color:var(--text)}
.console{min-height:0;display:grid;grid-template-rows:auto minmax(76px,120px) auto;background:var(--bg)}
.console form{display:grid;gap:8px}
textarea{width:100%;height:60px;resize:none;font-family:ui-monospace,SFMono-Regular,Menlo,monospace}
.command-row{display:flex;gap:8px;align-items:center}
.command-row button.primary{min-width:72px}
.quick{display:flex;gap:8px;flex-wrap:wrap;margin-top:10px}
.scroll-panel{min-height:0;overflow:auto}
#details,#workflow,#history{font-size:13px;color:var(--text)}
.selection-title{font-family:ui-monospace,SFMono-Regular,Menlo,monospace;color:var(--yellow);margin-bottom:8px}
.kv{display:grid;grid-template-columns:82px minmax(0,1fr);gap:5px 8px;margin:0}
.kv dt{color:var(--muted)}
.kv dd{margin:0;min-width:0;overflow-wrap:anywhere}
.list{display:grid;gap:8px}
.list-item{border:1px solid var(--line);background:var(--bg);border-radius:6px;padding:8px}
.list-item b{display:block;margin-bottom:2px}
.list-item small{color:var(--muted)}
#out{
  margin:0;
  min-height:0;
  height:100%;
  max-height:none;
  overflow:auto;
  white-space:pre-wrap;
  color:var(--text);
  font:12px/1.4 ui-monospace,SFMono-Regular,Menlo,monospace;
  background:var(--bg);
  border-top:1px solid var(--line);
  padding:12px 14px;
}
#sync-state{color:var(--muted);font-family:ui-monospace,SFMono-Regular,Menlo,monospace}
.notice{color:var(--muted);font-size:12px;padding:10px 14px;border-top:1px solid var(--line)}
.graph-svg{display:block;transform-origin:0 0;transition:transform 120ms ease}
.commit-node{cursor:pointer}
.commit-node:hover circle{stroke:var(--yellow);stroke-width:4}
.ref-label text{font:700 12px ui-monospace,SFMono-Regular,Menlo,monospace;fill:var(--panel)}
.commit-subject{font:13px ui-sans-serif,system-ui,sans-serif;fill:var(--muted)}
.commit-sha{font:700 12px ui-monospace,SFMono-Regular,Menlo,monospace;fill:var(--panel)}
.edge{fill:none;stroke:var(--line);stroke-width:3;stroke-linecap:round}
.edge-arrow{fill:var(--line)}
@media (max-width: 980px){
  .topbar{height:auto;grid-template-columns:1fr;padding:12px}
  .toolbar{justify-content:flex-start}
  .app{grid-template-columns:1fr;grid-template-rows:minmax(220px,1fr) minmax(380px,45vh)}
  .side{border-left:0;border-top:1px solid var(--line)}
  .raw-wrap{display:none}
}
@media (max-height: 760px){
  .panel{padding:10px 12px}
  textarea{height:48px}
  .console{grid-template-rows:auto minmax(58px,82px) auto}
  .quick{gap:6px;margin-top:8px}
  .notice{padding:7px 12px}
}
#theme-toggle .moon{display:none}#theme-toggle .sun{display:inline}:root[data-theme="light"] #theme-toggle .sun{display:none}:root[data-theme="light"] #theme-toggle .moon{display:inline}
</style>
</head>
<body>
<header class="topbar">
  <div class="brand">
    <strong>tbd visualizer</strong>
    <span class="repo">{{.Root}}</span>
  </div>
  <div class="toolbar">
    <button id="refresh" type="button">Refresh</button>
    <button id="live" type="button" aria-pressed="true">Live</button>
    <label class="field">Limit <input id="limit" type="number" min="20" max="2000" step="20" value="240"></label>
    <label class="field">Zoom <input id="zoom" type="range" min="70" max="150" value="100"></label>
    <span id="sync-state">loading</span>
    <button id="theme-toggle" type="button" title="Toggle light / dark"><span class="sun">☀</span><span class="moon">☾</span></button>
  </div>
</header>
<main class="app">
  <section class="workspace">
    <div class="stage-wrap"><div id="graph-stage"><div class="loading">loading wasm</div></div></div>
    <div class="raw-wrap"><pre id="raw-graph"></pre></div>
  </section>
  <aside class="side">
    <section class="panel">
      <div class="panel-title">Repository</div>
      <div class="status-grid">
        <div class="metric"><b>branch</b><span id="branch-name">-</span></div>
        <div class="metric"><b>head</b><span id="head-sha">-</span></div>
        <div class="metric"><b>commits</b><span id="commit-count">-</span></div>
        <div class="metric"><b>refs</b><span id="ref-count">-</span></div>
      </div>
    </section>
    <section class="panel">
      <div class="panel-title">Refs</div>
      <div class="filters">
        <label><input type="checkbox" data-filter="branch" checked>Branches</label>
        <label><input type="checkbox" data-filter="remote" checked>Remotes</label>
        <label><input type="checkbox" data-filter="tag" checked>Tags</label>
        <label><input type="checkbox" data-filter="deploy" checked>Deploy</label>
        <label><input type="checkbox" data-filter="release" checked>Release</label>
        <label><input type="checkbox" data-filter="lock" checked>Locks</label>
      </div>
    </section>
    <section class="panel scroll-panel">
      <div class="panel-title">Selection</div>
      <div id="details">No commit selected.</div>
      <div class="panel-title" style="margin-top:16px">Workflow</div>
      <div id="workflow" class="list"></div>
      <div class="panel-title" style="margin-top:16px">History</div>
      <div id="history" class="list"></div>
    </section>
    <section class="console">
      <div class="panel">
        <form id="command-form">
          <textarea id="cmd" name="cmd" spellcheck="false">tbd status</textarea>
          <div class="command-row">
            <button class="primary" type="submit">Run</button>
            <button id="clear-output" type="button">Clear</button>
          </div>
        </form>
        <div class="quick">
          <button type="button" data-cmd="tbd status">status</button>
          <button type="button" data-cmd="tbd guard">guard</button>
          <button type="button" data-cmd="tbd graph">graph</button>
          <button type="button" data-cmd="git status --short">git status</button>
        </div>
      </div>
      <pre id="out"></pre>
      <div class="notice">LearnGitBranching visual influence is used under MIT license.</div>
    </section>
  </aside>
</main>
<script src="/assets/wasm_exec.js"></script>
<script src="/assets/app.js"></script>
</body>
</html>`

const visualAppJS = `(function(){
"use strict";

var state = {
  live: true,
  graph: null,
  limit: 240,
  zoom: 1,
  filters: {branch:true, remote:true, tag:true, deploy:true, release:true, lock:true}
};
var refreshTimer = null;

function el(id){ return document.getElementById(id); }

function setSync(text){ el("sync-state").textContent = text; }

function boot(){
  bindControls();
  loadWasm().then(function(){
    setSync("ready");
    refreshGraph();
    refreshTimer = window.setInterval(function(){
      if (state.live) refreshGraph();
    }, 4000);
  }).catch(function(err){
    setSync("wasm failed");
    el("graph-stage").innerHTML = "";
    var div = document.createElement("div");
    div.className = "loading";
    div.textContent = String(err);
    el("graph-stage").appendChild(div);
  });
}

function bindControls(){
  el("refresh").addEventListener("click", refreshGraph);
  el("theme-toggle").addEventListener("click", function(){var r=document.documentElement,c=r.getAttribute("data-theme")==="light"?"dark":"light";r.setAttribute("data-theme",c);try{localStorage.setItem("tbd-theme",c)}catch(e){}renderGraph();});
  el("live").addEventListener("click", function(){
    state.live = !state.live;
    el("live").setAttribute("aria-pressed", String(state.live));
    el("live").textContent = state.live ? "Live" : "Paused";
  });
  el("limit").addEventListener("change", function(){
    state.limit = Number(el("limit").value) || 240;
    refreshGraph();
  });
  el("zoom").addEventListener("input", function(){
    state.zoom = (Number(el("zoom").value) || 100) / 100;
    renderGraph();
  });
  document.querySelectorAll("[data-filter]").forEach(function(input){
    input.addEventListener("change", function(){
      state.filters[input.getAttribute("data-filter")] = input.checked;
      renderGraph();
    });
  });
  document.querySelectorAll("[data-cmd]").forEach(function(button){
    button.addEventListener("click", function(){
      runCommand(button.getAttribute("data-cmd"));
    });
  });
  el("clear-output").addEventListener("click", function(){ el("out").textContent = ""; });
  el("command-form").addEventListener("submit", function(ev){
    ev.preventDefault();
    runCommand(el("cmd").value);
  });
  renderHistory();
}

function loadWasm(){
  if (typeof Go === "undefined") {
    return Promise.reject(new Error("wasm_exec.js did not load"));
  }
  var go = new Go();
  var instantiate = WebAssembly.instantiateStreaming ?
    WebAssembly.instantiateStreaming(fetch("/assets/tbd_visual.wasm"), go.importObject) :
    fetch("/assets/tbd_visual.wasm").then(function(r){ return r.arrayBuffer(); }).then(function(bytes){
      return WebAssembly.instantiate(bytes, go.importObject);
    });
  return instantiate.catch(function(){
    return fetch("/assets/tbd_visual.wasm").then(function(r){ return r.arrayBuffer(); }).then(function(bytes){
      return WebAssembly.instantiate(bytes, go.importObject);
    });
  }).then(function(result){
    go.run(result.instance);
    if (!window.tbdVisual || !window.tbdVisual.render) {
      throw new Error("visual renderer did not initialize");
    }
  });
}

function refreshGraph(){
  setSync("syncing");
  return fetch("/api/graph?limit=" + encodeURIComponent(state.limit), {cache:"no-store"}).then(function(r){
    if (!r.ok) return r.text().then(function(text){ throw new Error(text || r.statusText); });
    return r.json();
  }).then(function(graph){
    state.graph = graph;
    renderGraph();
    updateSummary(graph);
    setSync("updated");
  }).catch(function(err){
    setSync("error");
    el("out").textContent = String(err);
  });
}

function renderGraph(){
  if (!state.graph || !window.tbdVisual) return;
  var options = {filters: state.filters, zoom: state.zoom};
  window.tbdVisual.render(JSON.stringify(state.graph), JSON.stringify(options));
  el("raw-graph").textContent = state.graph.raw || "";
}

function updateSummary(graph){
  var status = graph.status || {};
  el("branch-name").textContent = status.detached ? "detached" : (status.branch || "-");
  el("head-sha").textContent = shortSha(status.head);
  el("commit-count").textContent = String((graph.commits || []).length);
  el("ref-count").textContent = String((graph.refs || []).length);
  renderWorkflow(graph.workflow || {});
}

function renderWorkflow(workflow){
  var host = el("workflow");
  host.innerHTML = "";
  var rows = [];
  (workflow.items || []).slice(0, 5).forEach(function(item){
    rows.push({title:(item.id ? item.id + " " : "") + item.desc, meta:item.kind + " | " + item.status + " | " + shortSha(item.commit)});
  });
  (workflow.groups || []).slice(0, 3).forEach(function(group){
    rows.push({title:group.name, meta:group.kind + " | " + (group.item_ids || []).join(", ")});
  });
  (workflow.uat || []).slice(0, 3).forEach(function(uat){
    rows.push({title:uat.semver || uat.candidate_ref, meta:(uat.valid ? "valid" : "stale") + " | " + shortSha(uat.commit)});
  });
  if (rows.length === 0) {
    host.textContent = "No workflow state.";
    return;
  }
  rows.forEach(function(row){
    var item = document.createElement("div");
    item.className = "list-item";
    var b = document.createElement("b");
    b.textContent = row.title || "-";
    var small = document.createElement("small");
    small.textContent = row.meta || "";
    item.appendChild(b);
    item.appendChild(small);
    host.appendChild(item);
  });
}

function runCommand(command){
  command = (command || "").trim();
  if (!command) return;
  el("cmd").value = command;
  setSync("running");
  var fd = new FormData();
  fd.set("cmd", command);
  return fetch("/run", {method:"POST", body:fd}).then(function(r){
    return r.text().then(function(text){
      if (!r.ok) throw new Error(text || r.statusText);
      return text;
    });
  }).then(function(text){
    el("out").textContent = text;
    remember(command);
    renderHistory();
    return refreshGraph();
  }).catch(function(err){
    el("out").textContent = String(err);
    setSync("error");
    refreshGraph();
  });
}

function remember(command){
  var history = loadHistory().filter(function(item){ return item !== command; });
  history.unshift(command);
  history = history.slice(0, 8);
  localStorage.setItem("tbd.visual.history", JSON.stringify(history));
}

function loadHistory(){
  try {
    var parsed = JSON.parse(localStorage.getItem("tbd.visual.history") || "[]");
    return Array.isArray(parsed) ? parsed : [];
  } catch (_err) {
    return [];
  }
}

function renderHistory(){
  var host = el("history");
  host.innerHTML = "";
  var history = loadHistory();
  if (history.length === 0) {
    host.textContent = "No commands yet.";
    return;
  }
  history.forEach(function(command){
    var button = document.createElement("button");
    button.type = "button";
    button.textContent = command;
    button.addEventListener("click", function(){ runCommand(command); });
    host.appendChild(button);
  });
}

function shortSha(sha){
  if (!sha) return "-";
  return sha.length > 12 ? sha.slice(0, 12) : sha;
}

window.tbdVisualHost = {
  selectCommit: function(payload){
    var data = {};
    try { data = JSON.parse(payload); } catch (_err) {}
    var host = el("details");
    host.innerHTML = "";
    var title = document.createElement("div");
    title.className = "selection-title";
    title.textContent = (data.short || "-") + " " + (data.subject || "");
    host.appendChild(title);
    var dl = document.createElement("dl");
    dl.className = "kv";
    addKV(dl, "sha", data.sha || "");
    addKV(dl, "author", data.author || "");
    addKV(dl, "time", data.time || "");
    addKV(dl, "parents", (data.parents || []).map(shortSha).join(", ") || "-");
    addKV(dl, "refs", (data.refs || []).join(", ") || "-");
    host.appendChild(dl);
  }
};

function addKV(dl, key, value){
  var dt = document.createElement("dt");
  var dd = document.createElement("dd");
  dt.textContent = key;
  dd.textContent = value;
  dl.appendChild(dt);
  dl.appendChild(dd);
}

document.addEventListener("DOMContentLoaded", boot);
})();`
