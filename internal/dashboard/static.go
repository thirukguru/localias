// Package dashboard — embedded static HTML/CSS/JS for the dashboard SPA.
// Uses string constants instead of go:embed for simplicity.
//
// Author: Thiru K
// Module: github.com/thirukguru/localias/internal/dashboard
package dashboard

var indexHTML = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Localias Dashboard</title>
    <meta name="description" content="Localias local reverse proxy dashboard">
    <link rel="stylesheet" href="/style.css">
</head>
<body>
    <div id="app">
        <header>
            <div class="header-inner">
                <div class="brand">
                    <div class="logo">⚡</div>
                    <h1 class="brand-text">Localias</h1>
                </div>
                <span class="status-badge"><span class="status-dot"></span> Running</span>
            </div>
        </header>
        <nav>
            <div class="nav-inner" id="tabs">
                <button class="tab-btn active" data-tab="routes">Routes</button>
                <button class="tab-btn" data-tab="traffic">Traffic</button>
                <button class="tab-btn" data-tab="profiles">Profiles</button>
                <button class="tab-btn" data-tab="settings">Settings</button>
            </div>
        </nav>
        <main>
            <div id="tab-routes" class="tab-content">
                <div class="section-header">
                    <h2>Active Routes</h2>
                    <button onclick="loadRoutes()" class="btn-secondary">Refresh</button>
                </div>
                <div id="routes-table" class="card-list"></div>
                <div id="routes-empty" class="empty-state hidden">
                    <p class="empty-title">No routes registered</p>
                    <p class="empty-sub">Use localias run or localias alias to add routes</p>
                </div>
            </div>
            <div id="tab-traffic" class="tab-content hidden">
                <div class="section-header">
                    <h2>Request Traffic</h2>
                    <select id="traffic-filter" class="filter-select"><option value="">All routes</option></select>
                </div>
                <div class="table-wrap">
                    <table>
                        <thead><tr>
                            <th>Time</th><th>Route</th><th>Method</th><th>Path</th><th>Status</th><th>Latency</th>
                        </tr></thead>
                        <tbody id="traffic-body"></tbody>
                    </table>
                </div>
                <div id="traffic-empty" class="empty-state hidden"><p>No traffic yet</p></div>
            </div>
            <div id="tab-profiles" class="tab-content hidden">
                <h2>Profiles</h2>
                <div class="card">
                    <p class="card-desc">Manage service profiles from your <code>localias.yaml</code></p>
                    <div class="cmd-list">
                        <code>localias profile start --profile default</code>
                        <code>localias profile stop --profile default</code>
                        <code>localias profile list</code>
                    </div>
                </div>
            </div>
            <div id="tab-settings" class="tab-content hidden">
                <h2>Settings</h2>
                <div class="settings-grid">
                    <div class="card">
                        <h3>Proxy Configuration</h3>
                        <dl>
                            <div class="dl-row"><dt>Port</dt><dd class="mono accent">7777</dd></div>
                            <div class="dl-row"><dt>HTTPS</dt><dd class="mono">Disabled</dd></div>
                        </dl>
                    </div>
                    <div class="card">
                        <h3>CLI Commands</h3>
                        <div class="cmd-list">
                            <code>localias trust</code>
                            <code>localias hosts sync</code>
                        </div>
                    </div>
                </div>
            </div>
        </main>
    </div>
    <script src="/app.js"></script>
</body>
</html>`

var appJS = "// Localias Dashboard JS\n" +
	"(function(){\n" +
	"'use strict';\n" +
	"document.querySelectorAll('.tab-btn').forEach(function(btn){\n" +
	"  btn.addEventListener('click',function(){\n" +
	"    document.querySelectorAll('.tab-btn').forEach(function(b){b.classList.remove('active')});\n" +
	"    document.querySelectorAll('.tab-content').forEach(function(c){c.classList.add('hidden')});\n" +
	"    btn.classList.add('active');\n" +
	"    document.getElementById('tab-'+btn.dataset.tab).classList.remove('hidden');\n" +
	"    if(btn.dataset.tab==='traffic') loadTraffic();\n" +
	"    if(btn.dataset.tab==='routes') loadRoutes();\n" +
	"  });\n" +
	"});\n" +
	"window.loadRoutes=async function(){\n" +
	"  try{\n" +
	"    var res=await fetch('/api/routes');\n" +
	"    var routes=await res.json();\n" +
	"    var c=document.getElementById('routes-table');\n" +
	"    var e=document.getElementById('routes-empty');\n" +
	"    if(!routes||routes.length===0){c.innerHTML='';e.classList.remove('hidden');return;}\n" +
	"    e.classList.add('hidden');\n" +
	"    c.innerHTML=routes.map(function(r){\n" +
	"      var dot=r.healthy===true?'dot-green':r.healthy===false?'dot-red':'dot-gray';\n" +
	"      var badge=r.static?'static':'dynamic';\n" +
	"      return '<div class=\"route-card\" onclick=\"window.open(\\''+r.url+'\\',\\'_blank\\')\">' +\n" +
	"        '<div class=\"route-left\"><span class=\"dot '+dot+'\"></span>' +\n" +
	"        '<div><div class=\"route-name\">'+r.name+'</div>' +\n" +
	"        '<span class=\"route-url\">'+r.url+'</span></div></div>' +\n" +
	"        '<div class=\"route-right\"><span class=\"badge badge-'+(r.static?'static':'dynamic')+'\">'+badge+'</span>' +\n" +
	"        '<span class=\"mono route-port\">:'+r.port+'</span></div></div>';\n" +
	"    }).join('');\n" +
	"  }catch(ex){console.error('Failed:',ex);}\n" +
	"};\n" +
	"window.loadTraffic=async function(){\n" +
	"  try{\n" +
	"    var route=document.getElementById('traffic-filter').value;\n" +
	"    var url='/api/traffic'+(route?'?route='+route:'');\n" +
	"    var res=await fetch(url);\n" +
	"    var entries=await res.json();\n" +
	"    var body=document.getElementById('traffic-body');\n" +
	"    var empty=document.getElementById('traffic-empty');\n" +
	"    if(!entries||entries.length===0){body.innerHTML='';empty.classList.remove('hidden');return;}\n" +
	"    empty.classList.add('hidden');\n" +
	"    body.innerHTML=entries.reverse().map(function(e){\n" +
	"      var sc=e.status<300?'st-ok':e.status<400?'st-info':e.status<500?'st-warn':'st-err';\n" +
	"      var t=new Date(e.timestamp).toLocaleTimeString();\n" +
	"      var l=e.latency?(e.latency/1000000).toFixed(1)+'ms':'-';\n" +
	"      return '<tr><td class=\"text-muted\">'+t+'</td>'+" +
	"        '<td>'+e.route+'</td>'+" +
	"        '<td class=\"mono\">'+e.method+'</td>'+" +
	"        '<td class=\"mono text-muted path-cell\">'+e.path+'</td>'+" +
	"        '<td class=\"mono '+sc+'\">'+e.status+'</td>'+" +
	"        '<td class=\"text-muted\">'+l+'</td></tr>';\n" +
	"    }).join('');\n" +
	"  }catch(ex){console.error('Failed:',ex);}\n" +
	"};\n" +
	"try{var es=new EventSource('/api/traffic/stream');es.onmessage=function(ev){loadTraffic();};\n" +
	"es.onerror=function(){es.close();setTimeout(function(){location.reload();},5000);};}catch(e){}\n" +
	"document.getElementById('traffic-filter').addEventListener('change',loadTraffic);\n" +
	"loadRoutes();\n" +
	"setInterval(loadRoutes,10000);\n" +
	"})();\n"

var styleCSS = `/* Localias Dashboard — Self-contained CSS (no CDN) */
*,*::before,*::after{box-sizing:border-box;margin:0;padding:0}
:root{
  --bg:     #0a0a0f;
  --bg-s:   #12121a;
  --bg-card:#16161f;
  --border: #1e1e2e;
  --text:   #e2e2ef;
  --muted:  #6b6b80;
  --accent: #38bdf8;
  --accent2:#0ea5e9;
  --green:  #34d399;
  --red:    #f87171;
  --yellow: #fbbf24;
  --violet: #a78bfa;
  --radius: .75rem;
}
body{background:var(--bg);color:var(--text);font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,sans-serif;min-height:100vh;line-height:1.5;font-size:.9375rem}
#app{min-height:100vh;display:flex;flex-direction:column}
.hidden{display:none!important}

/* Header */
header{background:rgba(18,18,26,.9);backdrop-filter:blur(12px);border-bottom:1px solid var(--border);position:sticky;top:0;z-index:50}
.header-inner{max-width:72rem;margin:0 auto;padding:0 1.5rem;display:flex;align-items:center;justify-content:space-between;height:4rem}
.brand{display:flex;align-items:center;gap:.75rem}
.logo{width:2rem;height:2rem;background:linear-gradient(135deg,var(--accent),var(--accent2));border-radius:.5rem;display:flex;align-items:center;justify-content:center;font-size:1.1rem}
.brand-text{font-size:1.25rem;font-weight:700;background:linear-gradient(90deg,var(--accent),#93c5fd);-webkit-background-clip:text;-webkit-text-fill-color:transparent;background-clip:text}
.status-badge{display:inline-flex;align-items:center;gap:.4rem;padding:.25rem .75rem;border-radius:99px;font-size:.75rem;font-weight:500;background:rgba(52,211,153,.08);color:var(--green);border:1px solid rgba(52,211,153,.15)}
.status-dot{width:6px;height:6px;border-radius:50%;background:var(--green);animation:pulse 2s infinite}

/* Nav */
nav{background:rgba(18,18,26,.5);border-bottom:1px solid var(--border)}
.nav-inner{max-width:72rem;margin:0 auto;padding:.5rem 1.5rem;display:flex;gap:.25rem}
.tab-btn{display:inline-flex;align-items:center;padding:.5rem 1rem;border-radius:.5rem;font-size:.875rem;font-weight:500;color:var(--muted);cursor:pointer;background:transparent;border:none;transition:all .15s}
.tab-btn:hover{color:var(--text);background:rgba(255,255,255,.04)}
.tab-btn.active{color:var(--accent);background:rgba(56,189,248,.08)}

/* Main */
main{flex:1;max-width:72rem;width:100%;margin:0 auto;padding:1.5rem}
.tab-content{animation:fadeIn .2s ease}

/* Sections */
.section-header{display:flex;align-items:center;justify-content:space-between;margin-bottom:1.5rem}
h2{font-size:1.125rem;font-weight:600;color:var(--text)}
h3{font-size:.8125rem;font-weight:500;color:var(--muted);margin-bottom:1rem;text-transform:uppercase;letter-spacing:.05em}
.btn-secondary{padding:.375rem .75rem;border-radius:.5rem;background:var(--bg-card);color:var(--muted);font-size:.8125rem;border:1px solid var(--border);cursor:pointer;transition:all .15s}
.btn-secondary:hover{background:var(--bg-s);color:var(--text)}

/* Cards */
.card-list{display:flex;flex-direction:column;gap:.5rem}
.card{background:var(--bg-card);border:1px solid var(--border);border-radius:var(--radius);padding:1.5rem}
.card-desc{color:var(--muted);margin-bottom:1rem;font-size:.875rem}
.card code{display:block;background:var(--bg-s);padding:.5rem .75rem;border-radius:.5rem;font-size:.75rem;margin-bottom:.5rem;color:var(--text);font-family:'Fira Code',Consolas,monospace}

/* Route cards */
.route-card{display:flex;align-items:center;justify-content:space-between;padding:1rem 1.25rem;background:var(--bg-card);border:1px solid var(--border);border-radius:var(--radius);cursor:pointer;transition:all .15s}
.route-card:hover{border-color:rgba(56,189,248,.3);transform:translateY(-1px);box-shadow:0 4px 12px rgba(0,0,0,.3)}
.route-left{display:flex;align-items:center;gap:1rem}
.route-right{display:flex;align-items:center;gap:1rem}
.route-name{font-weight:500;color:var(--text);margin-bottom:.125rem}
.route-url{font-size:.8125rem;color:var(--accent)}
.route-port{font-size:.875rem;color:var(--muted)}
.dot{width:8px;height:8px;border-radius:50%;flex-shrink:0}
.dot-green{background:var(--green)}
.dot-red{background:var(--red)}
.dot-gray{background:var(--muted)}
.badge{padding:.125rem .5rem;border-radius:99px;font-size:.6875rem;font-weight:500}
.badge-static{background:rgba(167,139,250,.08);color:var(--violet);border:1px solid rgba(167,139,250,.15)}
.badge-dynamic{background:rgba(56,189,248,.08);color:var(--accent);border:1px solid rgba(56,189,248,.15)}

/* Table */
.table-wrap{border:1px solid var(--border);border-radius:var(--radius);overflow-x:auto}
table{width:100%;border-collapse:collapse;font-size:.8125rem}
thead{background:var(--bg-s)}
th{padding:.75rem 1rem;text-align:left;font-size:.6875rem;font-weight:500;color:var(--muted);text-transform:uppercase;letter-spacing:.05em}
td{padding:.625rem 1rem;border-top:1px solid var(--border)}
tr:hover{background:rgba(255,255,255,.02)}
.path-cell{max-width:15rem;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.mono{font-family:'Fira Code',Consolas,monospace}
.text-muted{color:var(--muted)}
.accent{color:var(--accent)}
.st-ok{color:var(--green)}
.st-info{color:var(--accent)}
.st-warn{color:var(--yellow)}
.st-err{color:var(--red)}

/* Empty state */
.empty-state{text-align:center;padding:4rem 0;color:var(--muted)}
.empty-title{font-size:1.125rem;font-weight:500;margin-bottom:.25rem}
.empty-sub{font-size:.875rem}

/* Settings */
.settings-grid{display:grid;gap:1.5rem;grid-template-columns:repeat(auto-fit,minmax(300px,1fr))}
dl{display:flex;flex-direction:column;gap:.75rem}
.dl-row{display:flex;justify-content:space-between}
dt{color:var(--muted)}
dd{font-family:'Fira Code',Consolas,monospace}

/* Filter */
.filter-select{background:var(--bg-card);border:1px solid var(--border);border-radius:.5rem;padding:.375rem .75rem;font-size:.8125rem;color:var(--text);cursor:pointer}

/* Animations */
@keyframes fadeIn{from{opacity:0;transform:translateY(4px)}to{opacity:1;transform:translateY(0)}}
@keyframes pulse{0%,100%{opacity:1}50%{opacity:.4}}

/* Scrollbar */
::-webkit-scrollbar{width:6px;height:6px}
::-webkit-scrollbar-track{background:transparent}
::-webkit-scrollbar-thumb{background:var(--border);border-radius:99px}
*{scrollbar-width:thin;scrollbar-color:var(--border) transparent}
:focus-visible{outline:2px solid var(--accent);outline-offset:2px}

/* Responsive */
@media(max-width:640px){
  .header-inner,.nav-inner,main{padding-left:1rem;padding-right:1rem}
  table{font-size:.75rem}
  th,td{padding:.5rem}
  .settings-grid{grid-template-columns:1fr}
}`
