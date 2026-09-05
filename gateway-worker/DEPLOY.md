# Mihani Gateway — Cloudflare Worker

Free-tier proxy that holds your upstream API keys server-side so they never
leave your machine or get embedded in the released binary. Clients authenticate
with a token; the gateway swaps it for the real upstream key before forwarding.

## What you need

- Wrangler CLI (you already have it)
- A Cloudflare account
- Your upstream keys (rotated — see `gateway-deploy.md` step 1)

## Deploy (one-time)

Run these **in order** inside `gateway-worker/`:

```powershell
# 1. Login (if not already)
wrangler login

# 2. Create the worker
wrangler deploy

# 3. After deploy, note your URL — it prints something like:
#    Published mihani-gw (XX ms)
#      http://mihani-gw.<YOUR-SUBDOMAIN>.workers.dev
```

**Important:** after deploy, set your secrets **in the Cloudflare Dashboard**:
Workers → `mihani-gw` → Settings → Environment Variables → add:

| Variable   | Value                                              |
|------------|----------------------------------------------------|
| `PRO_BASE` | `https://seekai.cc/v1`                             |
| `PRO_KEY`  | `<your-new-pro-key>`                               |
| `CLOUD_BASE` | `https://api.hcnsec.cn/v1`                       |
| `CLOUD_KEY` | `<your-new-cloud-key>`                           |
| `CLIENT_TOKENS` | `<your-client-token>` |

Then click **Save and Deploy**. The env vars are injected at runtime — never
commit them to the repo.

## Test locally (before deploying)

```powershell
cd D:\Mihani\Apps\MihaniCode\gateway-worker
# Set env vars first (or put them in wrangler.toml vars section):
$env:PRO_BASE = "https://seekai.cc/v1"
$env:PRO_KEY = "<your-new-pro-key>"
$env:CLOUD_BASE = "https://api.hcnsec.cn/v1"
$env:CLOUD_KEY = "<your-new-cloud-key>"
$env:CLIENT_TOKENS = "<your-client-token>"
wrangler dev
```

In another terminal, test:
```powershell
$TOKEN = "<your-client-token>"
# Health
Invoke-RestMethod http://127.0.0.1:8787/health
# Pro models
Invoke-RestMethod "http://127.0.0.1:8787/pro/models" -Headers @{ Authorization = "Bearer $TOKEN" } | Select-Object -ExpandProperty data | ForEach-Object { $_.id }
# Cloud models
Invoke-RestMethod "http://127.0.0.1:8787/cloud/models" -Headers @{ Authorization = "Bearer $TOKEN" } | Select-Object -ExpandProperty data | ForEach-Object { $_.id }
# Bad token should 401
Invoke-RestMethod "http://127.0.0.1:8787/pro/models" -Headers @{ Authorization = "Bearer wrong" }
```

## Point Mihani at it

Once deployed (URL like `https://mihani-gw.yourname.workers.dev`), on your
machine set two environment variables (System Properties → Environment Variables
→ User):

```
MIHANI_GATEWAY=https://mihani-gw.yourname.workers.dev
MIHANI_GATEWAY_TOKEN=<your-client-token>
```

Close any open Mihani windows and relaunch — the built-in providers will now
route through your worker.

## What this does (and doesn't) do

| Feature                        | Status        |
|--------------------------------|---------------|
| Proxies /pro/* and /cloud/*    | ✅            |
| Swaps client token → upstream key | ✅         |
| Streams SSE responses          | ✅            |
| Per-IP rate limit (30/min)     | ✅ (memory)   |
| Client auth (token match)      | ✅            |
| Per-user billing / accounting  | ❌ (future)   |
| Persistent rate limiting       | ❌ (KV needed) |
| Request logging                | ❌ (add later) |

The memory-based rate limiter resets when the worker restarts. For production
use with many users, migrate it to KV. For a personal / small-team deployment
this is fine.
