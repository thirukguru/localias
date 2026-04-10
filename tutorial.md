# Localias Tutorial

> A step-by-step guide to using localias — the local reverse proxy that gives your dev servers stable, named URLs.

---

## Table of Contents

1. [What is Localias?](#what-is-localias)
2. [Installation](#installation)
3. [Quick Start (5 minutes)](#quick-start)
4. [Core Commands](#core-commands)
   - [localias run](#1-localias-run)
   - [localias alias](#2-localias-alias)
   - [localias list](#3-localias-list)
   - [localias proxy](#4-localias-proxy)
5. [Shorthand Syntax](#shorthand-syntax)
6. [Profiles (Multi-Service)](#profiles)
7. [HTTPS & Certificates](#https--certificates)
8. [Dashboard](#dashboard)
9. [Hosts File Sync](#hosts-file-sync)
10. [Health Checks](#health-checks)
11. [SSH Tunnels](#ssh-tunnels)
12. [MCP (AI Agent Integration)](#mcp-ai-agent-integration)
13. [Environment Variables](#environment-variables)
14. [Tips & Troubleshooting](#tips--troubleshooting)

---

## What is Localias?

When you run `npm run dev`, your app starts on a random port like `http://localhost:5173`.
Localias replaces that with a **stable, named URL**: `http://myapp.localhost:7777`.

**Before localias:**
```
App A → http://localhost:5173
App B → http://localhost:3000
App C → http://localhost:8080
```

**After localias:**
```
App A → http://myapp.localhost:7777
App B → http://api.localhost:7777
App C → http://admin.localhost:7777
```

---

## Installation

### Option 1: Install Script (easiest)
```bash
curl -fsSL https://raw.githubusercontent.com/thirukguru/localias/main/install.sh | bash
```

### Option 2: Go Install
```bash
go install github.com/thirukguru/localias@latest
```

### Option 3: Build from Source
```bash
git clone https://github.com/thirukguru/localias.git
cd localias
make build
sudo make install
```

### Verify Installation
```bash
localias --help
```

You should see:
```
Localias replaces port numbers with stable named .localhost URLs.
...
Available Commands:
  alias       Create a static route alias
  dashboard   Open the web dashboard in your browser
  ...
```

---

## Quick Start

### Step 1: Start the proxy daemon
```bash
localias proxy start
```
Output:
```
✓ Proxy daemon started on port 7777
```

### Step 2: Run your dev server through localias
```bash
cd ~/my-react-app
localias run -- npm run dev
```
Output:
```
→ http://my-react-app.localhost:7777
  Backend: 127.0.0.1:4000
```

### Step 3: Open in your browser
Open `http://my-react-app.localhost:7777` — that's it! 🎉

> **Note:** The URL Vite/Next.js prints (like `http://127.0.0.1:4000`) is the internal backend.
> Always use the `.localhost:7777` URL — that's the stable one.

---

## Core Commands

### 1. `localias run`

Runs any dev command and gives it a named URL. This is the command you'll use most.

**Basic usage:**
```bash
localias run -- npm run dev
```

**How it works:**
1. Infers your project name from `package.json`, `go.mod`, git, or directory name
2. Finds a free port (4000-4999 range)
3. Starts the proxy daemon (if not running)
4. Registers a route: `projectname.localhost:7777 → 127.0.0.1:<port>`
5. Runs your command with `PORT=<port>` injected
6. Cleans up the route when your command exits

**Examples:**

```bash
# React / Vite project
localias run -- npm run dev

# Next.js
localias run -- npx next dev

# Go server
localias run -- go run .

# Python Flask
localias run -- flask run

# Any command with arguments
localias run -- python -m http.server 8000

# With LAN sharing
localias run --share-lan -- npm run dev
```

**Flags:**
| Flag | Description |
|------|-------------|
| `--share-lan` | Share on LAN via mDNS |

**Injected environment variables:**
| Variable | Example | Description |
|----------|---------|-------------|
| `PORT` | `4000` | The assigned port |
| `HOST` | `127.0.0.1` | Bind address |
| `LOCALIAS_URL` | `http://myapp.localhost:7777` | The full proxy URL |
| `LOCALIAS_MCP_TOKEN` | `a3f2...` | Scoped MCP token for this route |
| `LOCALIAS_MCP_URL` | `http://mcp.localhost:7777` | MCP server URL |

**Framework auto-detection:**

Localias automatically injects the right flags for popular frameworks:

| Framework | What localias does |
|-----------|-------------------|
| Vite | Adds `--port 4000 --host 127.0.0.1` |
| Next.js | Adds `-p 4000 -H 127.0.0.1` |
| Nuxt | Adds `--port 4000 --host 127.0.0.1` |
| Astro | Adds `--port 4000 --host 127.0.0.1` |
| Angular | Adds `--port 4000 --host 127.0.0.1` |
| Remix | Adds `--port 4000` |
| Expo | Adds `--port 4000` |

---

### 2. `localias alias`

Creates a **static route** — for services that are already running (Docker containers, databases, external processes).

**Create an alias:**
```bash
# Map "redis" to port 6379
localias alias redis 6379

# Map "postgres" to port 5432
localias alias postgres 5432

# Map "api" to port 3000
localias alias api 3000
```

Now visit `http://redis.localhost:7777` to reach your Redis service.

**Force overwrite an existing alias:**
```bash
localias alias api 3001 --force
```

**Remove an alias:**
```bash
localias alias --remove redis
```

**Flags:**
| Flag | Description |
|------|-------------|
| `--force` | Overwrite existing route |
| `--remove` | Remove the alias instead of creating |

---

### 3. `localias list`

Shows all active routes.

**Basic list:**
```bash
localias list
```
Output:
```
NAME        URL                              BACKEND   TYPE
────        ───                              ───────   ────
myapp       http://myapp.localhost:7777       :4000     dynamic
api         http://api.localhost:7777         :3001     static
redis       http://redis.localhost:7777       :6379     static
```

**With health status:**
```bash
localias list --health
```
Output:
```
NAME        URL                              BACKEND   TYPE      HEALTH        LATENCY
────        ───                              ───────   ────      ──────        ───────
myapp       http://myapp.localhost:7777       :4000     dynamic   🟢 healthy    2.1ms
api         http://api.localhost:7777         :3001     static    🔴 unhealthy  -
redis       http://redis.localhost:7777       :6379     static    🟢 healthy    0.5ms
```

**JSON output (for scripts):**
```bash
localias list --json
```
```json
[
  {
    "name": "myapp",
    "url": "http://myapp.localhost:7777",
    "port": 4000,
    "type": "dynamic",
    "health": "healthy",
    "latency": "2.1ms"
  }
]
```

**Flags:**
| Flag | Description |
|------|-------------|
| `--health` | Run health checks and show status |
| `--json` | Output as JSON array |

---

### 4. `localias proxy`

Controls the background proxy daemon.

**Start the daemon:**
```bash
localias proxy start
```

**Start with HTTPS:**
```bash
# Auto-generates certificates
localias proxy start --https

# With custom certificate
localias proxy start --https --cert ./my.crt --key ./my.key
```

**Start in foreground (for debugging):**
```bash
localias proxy start --foreground
```

**Stop the daemon:**
```bash
localias proxy stop
```

**Flags:**
| Flag | Description |
|------|-------------|
| `--https` | Enable HTTPS with auto-generated certs |
| `--foreground` | Run in foreground (don't daemonize) |
| `--cert <path>` | Path to TLS certificate file |
| `--key <path>` | Path to TLS key file |

> **Tip:** You don't need to manually start the daemon. Running `localias run` or `localias alias` will auto-start it.

---

## Shorthand Syntax

Instead of `localias run`, you can use the shorthand to specify a name explicitly:

```bash
# Long form
localias run -- npm run dev       # name inferred from project

# Shorthand with explicit name
localias myapp -- npm run dev     # name is "myapp"
localias api -- go run ./api      # name is "api"
```

This is useful when you want a specific name regardless of what the project directory is called.

---

## Profiles

Profiles let you start **multiple services at once** from a `localias.yaml` file.

### Step 1: Create `localias.yaml`

```yaml
profiles:
  default:
    services:
      - name: web
        cmd: "npm run dev"
        dir: ./apps/web
      - name: api
        cmd: "go run ."
        dir: ./apps/api
      - name: worker
        cmd: "python worker.py"
        dir: ./services/worker

  frontend-only:
    services:
      - name: web
        cmd: "npm run dev"
        dir: ./apps/web
```

### Step 2: Start a profile

```bash
localias profile start
```
Output:
```
Starting profile "default" (3 services)

[web] → http://web.localhost:7777 (port 4000)
[api] → http://api.localhost:7777 (port 4001)
[worker] → http://worker.localhost:7777 (port 4002)
[web] ready in 1.2s
[api] listening on :4001
[worker] Worker started
```

Each service gets a **colored prefix** so you can tell logs apart.

### Step 3: Start a specific profile

```bash
localias profile start --profile frontend-only
```

### Step 4: List profiles

```bash
localias profile list
```
Output:
```
  default (3 services)
    - web: npm run dev
    - api: go run .
    - worker: python worker.py
  frontend-only (1 services)
    - web: npm run dev
```

### Step 5: Stop a profile

```bash
localias profile stop --profile default
```

Or just press **Ctrl-C** on the `profile start` command.

**Profile commands:**
| Command | Description |
|---------|-------------|
| `localias profile start` | Start default profile |
| `localias profile start --profile <name>` | Start named profile |
| `localias profile stop` | Stop default profile services |
| `localias profile stop --profile <name>` | Stop named profile services |
| `localias profile list` | List all available profiles |

---

## HTTPS & Certificates

### Auto-HTTPS (easiest)

```bash
localias proxy start --https
```

This automatically:
1. Generates a local CA (Certificate Authority)
2. Creates a leaf certificate for `*.localhost`
3. Starts the proxy with TLS

### Trust the certificate

To avoid browser "Not Secure" warnings:

```bash
sudo localias trust
```

This adds the localias CA to your system trust store. Supported on:
- **macOS** — Keychain
- **Debian/Ubuntu** — `update-ca-certificates`
- **Arch Linux** — `trust anchor`
- **Fedora/RHEL** — `update-ca-trust`

### HTTPS via environment variable

```bash
export LOCALIAS_HTTPS=1
localias proxy start    # HTTPS is auto-enabled
```

---

## Dashboard

Localias includes a built-in web dashboard.

### Open the dashboard
```bash
localias dashboard
```

This opens `http://localias.localhost:7777` in your browser.

### Dashboard tabs

| Tab | What it shows |
|-----|--------------|
| **Routes** | All active routes with health status (click to open) |
| **Traffic** | Live request log with method, path, status, latency |
| **Profiles** | Profile management CLI commands |
| **Settings** | Proxy configuration and useful commands |

---

## Hosts File Sync

Some tools don't resolve `.localhost` domains. You can sync routes to `/etc/hosts`:

### Manual sync
```bash
sudo localias hosts sync
```

This adds entries like:
```
# BEGIN localias managed block
127.0.0.1  myapp.localhost
127.0.0.1  api.localhost
# END localias managed block
```

### Remove entries
```bash
sudo localias hosts clean
```

### Auto-sync on every run

```bash
export LOCALIAS_SYNC_HOSTS=1
localias run -- npm run dev    # /etc/hosts updated automatically
```

---

## Health Checks

Localias continuously monitors backend health in the background.

### Check health via CLI
```bash
localias list --health
```

### How it works
- Sends HTTP GET to each backend every **10 seconds**
- Tracks **consecutive failures** (3 = unhealthy)
- Status shown in dashboard and `list --health`

---

## SSH Tunnels

Expose a local service to the internet via SSH reverse tunnel.

### Step 1: Set your relay server
```bash
export LOCALIAS_TUNNEL_HOST=relay.example.com
```

### Step 2: Open the tunnel
```bash
localias tunnel myapp
```
Output:
```
→ Opening tunnel for myapp via relay.example.com...
  Local: 127.0.0.1:4000
```

> **Note:** You need an SSH server that allows remote port forwarding. The route must already be registered via `localias run` or `localias alias`.

---

## MCP (AI Agent Integration)

Localias includes an MCP (Model Context Protocol) server so AI coding agents can discover your running services. It supports **scoped tokens** for per-route access control.

### Authentication

The MCP server supports two types of tokens:

**1. Admin Token** — full access to all routes and tools:
```bash
cat ~/.localias/mcp-token
```

**2. Scoped Tokens** — restricted to specific routes and capabilities:
```bash
# Create a token for specific routes
localias mcp token create --routes frontend,api --capabilities read,health

# Create a read-only token for all routes
localias mcp token create --routes '*' --capabilities read --label "monitoring agent"

# List all scoped tokens
localias mcp token list

# Revoke by prefix
localias mcp token revoke a3f2
```

**3. Ephemeral Tokens** — auto-issued and auto-revoked:

When you use `localias run`, an ephemeral token is automatically:
- Created — scoped to the launched route with `read` + `health` capabilities
- Injected — as `LOCALIAS_MCP_TOKEN` and `LOCALIAS_MCP_URL` in the child process environment
- Revoked — automatically when the process exits (tracked by PID)

This means each agent-launched process gets its own identity and can only see the routes it's authorized to touch.

### Token Capabilities

| Capability | What it allows |
|-----------|----------------|
| `read` | `list_routes` (filtered to allowed routes), `get_route` |
| `write` | `register_route` (only for allowed route names) |
| `health` | `health_check` (only for allowed routes) |
| `*` | Full admin access (admin token only) |

### Configure Cursor / Claude Desktop

Add this to your MCP settings:

```json
{
  "mcpServers": {
    "localias": {
      "url": "http://mcp.localhost:7777/mcp",
      "headers": {
        "Authorization": "Bearer <paste your token here>"
      }
    }
  }
}
```

### MCP Tools Available

| Tool | What it does |
|------|-------------|
| `list_routes` | List all active routes with health status (filtered by token scope) |
| `get_route` | Get details for a specific route + recent traffic |
| `register_route` | Register a new static alias (requires `write` capability) |
| `health_check` | Run an immediate health check |

### MCP Token Management

```bash
# Create a persistent token with specific routes and capabilities
localias mcp token create --routes frontend,api --capabilities read,health --label "CI agent"
# → ✓ Scoped MCP token created
# →   Token: a3f2b8c9d1e4f5a6b7c8d9e0f1a2b3c4...
# →   Routes: frontend, api
# →   Capabilities: read, health

# List all tokens (shows prefix, routes, caps, PID for ephemeral)
localias mcp token list
# → Scoped MCP tokens (3):
# →   a3f2b8c9…  routes=[frontend,api]  caps=[read,health]  CI agent
# →   b1c2d3e4…  routes=[myapp]         caps=[read,health]  ephemeral:myapp  PID: 12345 (ephemeral)
# →   c5d6e7f8…  routes=[*]             caps=[read]         monitoring agent

# Revoke tokens matching a prefix
localias mcp token revoke a3f2
# → ✓ Revoked 1 token(s) matching "a3f2"
```

### Test with curl

```bash
# Get your admin token
TOKEN=$(cat ~/.localias/mcp-token)

# List available tools
curl -s -X POST http://mcp.localhost:7777/mcp/message \\
  -H "Authorization: Bearer $TOKEN" \\
  -H "Content-Type: application/json" \\
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/list"}'

# Call list_routes
curl -s -X POST http://mcp.localhost:7777/mcp/message \\
  -H "Authorization: Bearer $TOKEN" \\
  -H "Content-Type: application/json" \\
  -d '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"list_routes","arguments":{}}}'

# Without token (rejected)
curl -s -X POST http://mcp.localhost:7777/mcp/message \\
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/list"}'
# → {"error":"Authorization header required..."}
```

### Security Notes

- Admin token file is `0600` (only your user can read it)
- MCP only listens on `localhost` — not exposed to the network
- Scoped tokens are stored in `~/.localias/mcp-tokens.json` (also `0600`)
- Ephemeral tokens auto-revoke when their PID exits
- To regenerate admin token: `rm ~/.localias/mcp-token && localias proxy stop && localias proxy start`

### Example: Agent Discovers and Uses Routes

This walkthrough shows how an AI agent (or any automated tool) can launch a service, discover it via MCP, and interact with it — all without hardcoding ports or URLs.

#### Scenario

An AI coding agent needs to:
1. Start a backend API server
2. Discover its URL automatically
3. Run health checks
4. Make requests to the discovered endpoint

#### Step 1: Agent launches the service

```bash
# The agent runs the backend through localias
localias run -- node server.js
```

Output:
```
→ http://my-api.localhost:7777
  Backend: 127.0.0.1:4000
```

Behind the scenes, `localias run` also:
- Creates an **ephemeral scoped token** for `my-api` with `read` + `health` capabilities
- Injects `LOCALIAS_MCP_TOKEN` and `LOCALIAS_MCP_URL` into the child process environment
- The token auto-revokes when the process exits

#### Step 2: Agent discovers the route via MCP

The child process (or a sibling agent reading the same env) uses the injected token:

```bash
# Read the injected credentials
TOKEN=$LOCALIAS_MCP_TOKEN
MCP_URL=$LOCALIAS_MCP_URL  # http://mcp.localhost:7777

# Discover routes visible to this token
curl -s -X POST "$MCP_URL/mcp/message" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "tools/call",
    "params": {
      "name": "list_routes",
      "arguments": {}
    }
  }'
```

Response (only shows routes this token is scoped to):
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "content": [{
      "type": "text",
      "text": "[{\"name\":\"my-api\",\"url\":\"http://my-api.localhost:7777\",\"backend_port\":4000,\"healthy\":true,\"latency\":\"2.1ms\"}]"
    }]
  }
}
```

> **Key point:** The agent only sees `my-api` — not other developers' routes. A different agent launched with a different `localias run` would only see its own route.

#### Step 3: Agent checks health before using the service

```bash
curl -s -X POST "$MCP_URL/mcp/message" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "id": 2,
    "method": "tools/call",
    "params": {
      "name": "health_check",
      "arguments": {"name": "my-api"}
    }
  }'
```

Response:
```json
{
  "result": {
    "content": [{
      "type": "text",
      "text": "{\"healthy\":true,\"latency\":\"1.8ms\",\"last_check\":\"10:45:02\"}"
    }]
  }
}
```

#### Step 4: Agent uses the discovered URL

Now the agent can make requests to `http://my-api.localhost:7777` — no hardcoded ports.

```bash
# The agent hits the discovered endpoint
curl http://my-api.localhost:7777/api/users
```

#### Full Python Agent Example

Here's a complete Python script showing an agent that discovers and uses localias routes:

```python
#!/usr/bin/env python3
"""Example: AI agent that discovers local services via localias MCP."""

import json
import os
import urllib.request

# Read credentials injected by `localias run`
MCP_TOKEN = os.environ.get("LOCALIAS_MCP_TOKEN")
MCP_URL = os.environ.get("LOCALIAS_MCP_URL", "http://mcp.localhost:7777")

def mcp_call(method, params=None):
    """Make a JSON-RPC call to the localias MCP server."""
    payload = {
        "jsonrpc": "2.0",
        "id": 1,
        "method": method,
    }
    if params:
        payload["params"] = params

    req = urllib.request.Request(
        f"{MCP_URL}/mcp/message",
        data=json.dumps(payload).encode(),
        headers={
            "Authorization": f"Bearer {MCP_TOKEN}",
            "Content-Type": "application/json",
        },
    )
    with urllib.request.urlopen(req) as resp:
        return json.loads(resp.read())

# 1. Discover what routes this agent can see
routes_resp = mcp_call("tools/call", {
    "name": "list_routes",
    "arguments": {},
})
routes = json.loads(routes_resp["result"]["content"][0]["text"])

print(f"Discovered {len(routes)} route(s):")
for route in routes:
    status = "✅" if route.get("healthy") else "❌"
    print(f"  {status} {route['name']} → {route['url']} (port {route['backend_port']})")

# 2. Check health of each route
for route in routes:
    health_resp = mcp_call("tools/call", {
        "name": "health_check",
        "arguments": {"name": route["name"]},
    })
    health = json.loads(health_resp["result"]["content"][0]["text"])
    print(f"\n  Health: {route['name']}")
    print(f"    Healthy: {health['healthy']}")
    print(f"    Latency: {health['latency']}")

# 3. Use the discovered URL
for route in routes:
    if route.get("healthy"):
        print(f"\n→ Making request to {route['url']}/api/status")
        try:
            with urllib.request.urlopen(f"{route['url']}/api/status") as resp:
                print(f"  Response: {resp.status} {resp.read().decode()[:200]}")
        except Exception as e:
            print(f"  Error: {e}")
```

Run it:
```bash
# Launch the service + agent together
localias run -- node server.js &

# Run the agent script (reads LOCALIAS_MCP_TOKEN from env)
python3 agent.py

# Output:
# Discovered 1 route(s):
#   ✅ my-api → http://my-api.localhost:7777 (port 4000)
#
#   Health: my-api
#     Healthy: True
#     Latency: 1.8ms
#
# → Making request to http://my-api.localhost:7777/api/status
#   Response: 200 {"status":"ok"}
```

#### Multi-Agent Isolation

When multiple agents run simultaneously, each only sees its own routes:

```bash
# Terminal 1 — Frontend agent
localias run -- npm run dev
# Gets token scoped to "frontend" only

# Terminal 2 — API agent
localias run -- go run ./api
# Gets token scoped to "api" only

# Terminal 3 — Admin with full access
TOKEN=$(cat ~/.localias/mcp-token)
# Admin token sees ALL routes
```

Each agent's `list_routes` call returns different results based on its token scope:
- Frontend agent sees: `[{name: "frontend", ...}]`
- API agent sees: `[{name: "api", ...}]`
- Admin sees: `[{name: "frontend", ...}, {name: "api", ...}]`

This is the key insight: **agents can only discover what they're authorized to touch.**

---

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `LOCALIAS_PORT` | `7777` | Proxy listening port |
| `LOCALIAS_STATE_DIR` | `~/.localias` | State directory (PID, socket, routes, certs) |
| `LOCALIAS_HTTPS` | `0` | Set to `1` to auto-enable HTTPS |
| `LOCALIAS_SYNC_HOSTS` | `0` | Set to `1` to auto-sync /etc/hosts |
| `LOCALIAS_TUNNEL_HOST` | _(none)_ | SSH relay server for tunnels |
| `LOCALIAS_APP_PORT` | _(auto)_ | Force a specific app port |
| `LOCALIAS=0` | _(enabled)_ | Set to `0` to disable localias (passthrough) |
| `LOCALIAS_MCP_TOKEN` | _(auto)_ | Scoped MCP token (injected by `localias run`) |
| `LOCALIAS_MCP_URL` | _(auto)_ | MCP endpoint URL (injected by `localias run`) |

**Usage example:**
```bash
# Custom port + auto-HTTPS + auto-hosts
export LOCALIAS_PORT=8888
export LOCALIAS_HTTPS=1
export LOCALIAS_SYNC_HOSTS=1

localias run -- npm run dev
# → https://myapp.localhost:8888
```

---

## Tips & Troubleshooting

### Disable localias temporarily
```bash
LOCALIAS=0 npm run dev    # Runs directly without proxy
```

### Change the proxy port
```bash
# Via flag
localias proxy start --port 8888

# Via environment
export LOCALIAS_PORT=8888
localias proxy start
```

### Check if the daemon is running
```bash
localias list
# If it shows routes → daemon is running
# If it says "connecting to daemon" → auto-starts
```

### View daemon logs
```bash
cat ~/.localias/localias.log
```

### Reset everything
```bash
localias proxy stop
rm -rf ~/.localias
```

### WebSocket / HMR not working?

Localias supports WebSocket proxying natively. If HMR (Hot Module Replacement) isn't working:

1. Make sure you're using the `.localhost` URL in the browser (not `127.0.0.1`)
2. Check that the framework detection is injecting the right port:
   ```bash
   localias run -- npm run dev
   # Should show: Backend: 127.0.0.1:4000
   ```

### Port already in use?

Localias auto-picks ports from the 4000-4999 range. If all ports are busy:
```bash
LOCALIAS_APP_PORT=9000 localias run -- npm run dev
```

### Multiple projects at once

Open separate terminals:
```bash
# Terminal 1
cd ~/project-a && localias run -- npm run dev
# → http://project-a.localhost:7777

# Terminal 2
cd ~/project-b && localias run -- go run .
# → http://project-b.localhost:7777

# Terminal 3
localias alias redis 6379
# → http://redis.localhost:7777
```

Or use profiles for a single-command start:
```bash
localias profile start
```

---

## Command Reference (Quick)

```bash
# Run with auto-inferred name
localias run -- <cmd>

# Run with explicit name
localias <name> -- <cmd>

# Static aliases
localias alias <name> <port>
localias alias --remove <name>

# List routes
localias list
localias list --health
localias list --json

# Proxy daemon
localias proxy start
localias proxy start --https
localias proxy stop

# Profiles
localias profile start [--profile <name>]
localias profile stop [--profile <name>]
localias profile list

# MCP tokens
localias mcp token create --routes <routes> --capabilities <caps>
localias mcp token list
localias mcp token revoke <prefix>

# Utilities
localias dashboard
localias trust
localias hosts sync
localias hosts clean
localias tunnel <name>
```

---

**Author:** Thiru K  
**Repo:** [github.com/thirukguru/localias](https://github.com/thirukguru/localias)
