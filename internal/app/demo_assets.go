package app

const demoHTML = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<script>(function(){try{var t=localStorage.getItem("tbd-theme");document.documentElement.setAttribute("data-theme",t==="light"?"light":"dark")}catch(e){document.documentElement.setAttribute("data-theme","dark")}})();</script>
<title>tbd demo</title>
<style>
:root{color-scheme:dark;--bg:#02102e;--panel:#06214f;--panel2:#0a2a63;--line:#173a72;--text:#e9eef7;--muted:#9fb2d6;--green:#30f03d;--yellow:#ffd347;--red:#ff6969;--cyan:#25d8ff}
:root[data-theme="light"]{color-scheme:light;--bg:#ffffff;--panel:#f5f8fd;--panel2:#e9f0fa;--line:#d3ddf0;--text:#0a1f4d;--muted:#51617f}
*{box-sizing:border-box}
html,body{height:100%}
body{margin:0;height:100dvh;overflow:hidden;display:grid;grid-template-rows:auto minmax(0,1fr);background:var(--bg);color:var(--text);font:14px Inter,ui-sans-serif,system-ui,-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif;letter-spacing:0}
button,input{font:inherit}
button{min-height:34px;border:1px solid var(--line);border-radius:6px;background:var(--panel2);color:var(--text);padding:0 12px;cursor:pointer}
button:hover{border-color:var(--muted);background:var(--line)}
button.primary{background:var(--green);border-color:var(--green);color:var(--bg);font-weight:700}
.topbar{min-height:64px;display:grid;grid-template-columns:minmax(0,1fr) auto;align-items:center;gap:16px;padding:10px 16px;background:var(--panel);border-bottom:1px solid var(--line);z-index:10}
.title{min-width:0}
.title strong{display:block;font-size:16px}
.title span{display:block;color:var(--muted);font:12px ui-monospace,SFMono-Regular,Menlo,monospace;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
.controls{display:flex;align-items:center;gap:8px;flex-wrap:wrap;justify-content:flex-end}
.speed,.zoom{display:flex;align-items:center;gap:6px;color:var(--muted)}
.speed input,.zoom input{width:110px}
.app{min-height:0;display:grid;grid-template-rows:auto minmax(0,1fr);overflow:hidden}
.story{border-bottom:1px solid var(--line);padding:12px 16px;background:var(--bg);display:grid;grid-template-columns:auto minmax(0,1fr);gap:14px;align-items:start}
.counter{font:700 22px ui-monospace,SFMono-Regular,Menlo,monospace;color:var(--yellow)}
.story h1{font-size:16px;margin:0 0 4px}
.story p{margin:0;color:var(--text);line-height:1.35}
.agents{min-height:0;display:grid;grid-template-columns:repeat(2,minmax(0,1fr));grid-template-rows:repeat(2,minmax(0,1fr));overflow:hidden}
.agent{min-width:0;min-height:0;display:grid;grid-template-rows:auto minmax(0,1fr) minmax(86px,15vh);border-right:1px solid var(--line);border-bottom:1px solid var(--line);background:var(--bg)}
.agent.active{box-shadow:inset 0 0 0 2px var(--yellow);animation:agentPulse .7s ease}
.agent.active .graph-svg{animation:graphPop .55s ease}
.agent:nth-child(2n){border-right:0}
.agent:nth-last-child(-n+2){border-bottom:0}
.agent-head{padding:10px 12px;border-bottom:1px solid var(--line);background:var(--panel)}
.agent-head strong{font-size:15px}
.agent-head span{display:block;color:var(--muted);font:12px ui-monospace,SFMono-Regular,Menlo,monospace;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
.graph{min-height:0;overflow:auto;padding:10px}
.log{min-height:0;overflow:auto;border-top:1px solid var(--line);background:var(--bg);padding:8px;display:grid;align-content:start;gap:8px}
.entry{border:1px solid var(--line);border-radius:6px;background:var(--panel);padding:7px}
.entry.failed{border-color:#7d3036;background:#1c1218}
.entry.fresh{border-color:var(--yellow);animation:logIn .45s ease}
.entry b{display:block;font:12px ui-monospace,SFMono-Regular,Menlo,monospace;color:var(--cyan);margin-bottom:4px}
.entry.failed b{color:var(--red)}
.entry pre{margin:0;white-space:pre-wrap;color:var(--text);font:11px/1.35 ui-monospace,SFMono-Regular,Menlo,monospace}
.status{color:var(--muted);font:12px ui-monospace,SFMono-Regular,Menlo,monospace}
.graph-svg{display:block;transform-origin:0 0}
.commit-node{cursor:pointer}
.commit-node:hover circle{stroke:var(--yellow);stroke-width:4}
.ref-label text{font:700 12px ui-monospace,SFMono-Regular,Menlo,monospace;fill:var(--panel)}
.commit-subject{font:13px ui-sans-serif,system-ui,sans-serif;fill:var(--muted)}
.commit-sha{font:700 12px ui-monospace,SFMono-Regular,Menlo,monospace;fill:var(--panel)}
.edge{fill:none;stroke:var(--line);stroke-width:3;stroke-linecap:round}.edge-arrow{fill:var(--line)}
@keyframes agentPulse{0%{box-shadow:inset 0 0 0 2px rgba(255,211,71,.2)}35%{box-shadow:inset 0 0 0 2px var(--yellow),0 0 0 4px rgba(255,211,71,.12)}100%{box-shadow:inset 0 0 0 2px var(--yellow)}}
@keyframes graphPop{0%{opacity:.65;transform:translateY(8px)}100%{opacity:1;transform:translateY(0)}}
@keyframes logIn{0%{opacity:.25;transform:translateY(-8px)}100%{opacity:1;transform:translateY(0)}}
@media (max-width:820px){.agents{grid-template-columns:1fr;grid-template-rows:none;overflow:auto}.agent{min-height:520px;border-right:0;border-bottom:1px solid var(--line)}.agent:nth-child(2n){border-right:0}.agent:nth-last-child(-n+2){border-bottom:1px solid var(--line)}body{overflow:auto}.app{overflow:visible}.topbar{position:sticky;top:0}}
#theme-toggle{position:fixed;top:10px;right:10px;z-index:100;background:var(--panel);color:var(--text);border:1px solid var(--line);border-radius:8px;padding:6px 9px;cursor:pointer;font-size:1rem;line-height:1}#theme-toggle .moon{display:none}:root[data-theme="light"] #theme-toggle .sun{display:none}:root[data-theme="light"] #theme-toggle .moon{display:inline}
</style>
</head>
<body>
<button id="theme-toggle" type="button" title="Toggle light / dark"><span class="sun">☀</span><span class="moon">☾</span></button>
<header class="topbar">
  <div class="title"><strong>tbd demo</strong><span id="workspace">loading</span></div>
  <div class="controls">
    <button id="step" class="primary" type="button">Step</button>
    <button id="play" type="button">Play</button>
    <button id="reset" type="button">Reset</button>
    <label class="zoom">Zoom <input id="zoom" type="range" min="35" max="110" step="5" value="60"></label>
    <label class="speed">Speed <input id="speed" type="range" min="600" max="4000" step="200" value="1600"></label>
    <span id="status" class="status">loading</span>
  </div>
</header>
<main class="app">
  <section class="story">
    <div id="counter" class="counter">0/0</div>
    <div><h1 id="story-title">Loading</h1><p id="story-detail"></p></div>
  </section>
  <section id="agents" class="agents"></section>
</main>
<script src="/assets/wasm_exec.js"></script>
<script src="/assets/demo.js"></script>
</body>
</html>`

const demoJS = `(function(){
"use strict";
var state = null;
var playing = false;
var timer = null;
var zoom = 0.60;
function el(id){ return document.getElementById(id); }
function setStatus(v){ el("status").textContent = v; }
function boot(){
  bind();
  loadWasm().then(function(){ return refresh(); }).catch(function(err){ setStatus(String(err)); });
}
function bind(){
  el("step").addEventListener("click", step);
  el("play").addEventListener("click", togglePlay);
  el("reset").addEventListener("click", reset);
  el("theme-toggle").addEventListener("click", function(){var r=document.documentElement,c=r.getAttribute("data-theme")==="light"?"dark":"light";r.setAttribute("data-theme",c);try{localStorage.setItem("tbd-theme",c)}catch(e){}render();});
  el("zoom").addEventListener("input", function(){ zoom = (Number(el("zoom").value) || 60) / 100; render(); });
  el("speed").addEventListener("input", function(){ if (playing) schedule(); });
}
function loadWasm(){
  var go = new Go();
  return WebAssembly.instantiateStreaming(fetch("/assets/tbd_visual.wasm"), go.importObject).catch(function(){
    return fetch("/assets/tbd_visual.wasm").then(function(r){ return r.arrayBuffer(); }).then(function(bytes){ return WebAssembly.instantiate(bytes, go.importObject); });
  }).then(function(result){
    go.run(result.instance);
    if (!window.tbdVisual || !window.tbdVisual.renderTo) throw new Error("visual renderer did not initialize");
  });
}
function refresh(){
  setStatus("syncing");
  return fetch("/api/demo/state", {cache:"no-store"}).then(json).then(function(s){ state=s; render(); setStatus("ready"); });
}
function step(){
  setStatus("running");
  return fetch("/api/demo/step", {method:"POST"}).then(json).then(function(s){ state=s; render(); setStatus(s.done ? "done" : "ready"); if (s.done) stop(); });
}
function reset(){
  stop();
  setStatus("resetting");
  return fetch("/api/demo/reset", {method:"POST"}).then(json).then(function(s){ state=s; render(); setStatus("ready"); });
}
function togglePlay(){
  playing = !playing;
  el("play").textContent = playing ? "Pause" : "Play";
  if (playing) schedule(); else stop();
}
function schedule(){
  clearTimeout(timer);
  if (!playing) return;
  timer = setTimeout(function(){ step().then(function(){ if (playing) schedule(); }); }, Number(el("speed").value) || 1600);
}
function stop(){
  playing = false;
  el("play").textContent = "Play";
  clearTimeout(timer);
}
function render(){
  if (!state) return;
  el("zoom").value = Math.round(zoom * 100);
  document.querySelector(".title strong").textContent = "tbd demo: " + (state.scenario || "basic");
  el("workspace").textContent = state.dir;
  el("counter").textContent = state.step + "/" + state.total;
  var story = state.next || state.last || {title:"Ready", detail:"Step through the workflow simulation."};
  if (state.done) story = {title:"Demo complete", detail:"Reset to rebuild the remote and all four local clones from scratch."};
  el("story-title").textContent = story.title;
  el("story-detail").textContent = story.detail || "";
  var host = el("agents");
  var seen = {};
  var active = {};
  (state.activeActors || []).forEach(function(id){ active[id] = true; });
  (state.agents || []).forEach(function(agent){
    seen[agent.id] = true;
    var card = document.getElementById("agent-" + agent.id);
    if (!card) {
      card = document.createElement("article");
      card.id = "agent-" + agent.id;
      card.className = "agent";
      card.innerHTML = '<div class="agent-head"><strong></strong><span></span></div><div class="graph" id="graph-' + agent.id + '"></div><div class="log" id="log-' + agent.id + '"></div>';
      host.appendChild(card);
    }
    card.classList.toggle("active", !!active[agent.id]);
    card.querySelector("strong").textContent = agent.name;
    card.querySelector("span").textContent = agent.path;
    window.tbdVisual.renderTo("graph-" + agent.id, JSON.stringify(agent.graph), JSON.stringify({zoom:zoom, filters:{branch:true, remote:true, tag:true, deploy:true, release:true, lock:true}}));
    renderLog(agent);
  });
  Array.prototype.slice.call(host.children).forEach(function(child){
    var id = child.id.replace("agent-", "");
    if (!seen[id]) child.remove();
  });
}
function renderLog(agent){
  var host = el("log-" + agent.id);
  host.innerHTML = "";
  if (!agent.logs || agent.logs.length === 0) {
    var empty = document.createElement("div");
    empty.className = "status";
    empty.textContent = "No commands yet.";
    host.appendChild(empty);
    return;
  }
  agent.logs.slice().reverse().forEach(function(log){
    var entry = document.createElement("div");
    entry.className = "entry" + (log.failed ? " failed" : "") + (log.step === state.step ? " fresh" : "");
    var b = document.createElement("b");
    b.textContent = "#" + log.step + " " + log.command;
    var pre = document.createElement("pre");
    pre.textContent = log.output || (log.failed ? "failed" : "ok");
    entry.appendChild(b);
    entry.appendChild(pre);
    host.appendChild(entry);
  });
}
function json(r){ if (!r.ok) return r.text().then(function(t){ throw new Error(t || r.statusText); }); return r.json(); }
window.tbdVisualHost = { selectCommit:function(){} };
document.addEventListener("DOMContentLoaded", boot);
})();`
