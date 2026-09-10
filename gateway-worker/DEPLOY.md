# Mihani Gateway — Cloudflare Worker

Free-tier proxy that holds your upstream API keys server-side so they never
leave your machine or get embedded in the released binary. Clients authenticate
with a token; the gateway swaps it for the real upstream key before forwarding.

## What you need

- Wrangler CLI (`npm i -g wrangler`)
- A Cloudflare account
- Your upstream API keys

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

| Variable       | Value                  |
|----------------|------------------------|
| `PRO_BASE`     | `<your-pro-upstream>/v1` |
| `PRO_KEY`      | `<your-pro-key>`       |
| `CLOUD_BASE`   | `<your-cloud-upstream>/v1` |
| `CLOUD_KEY`    | `<your-cloud-key>`     |
| `CLIENT_TOKENS` | `<your-client-token>` |

Then click **Save and Deploy**. The env vars are injected at runtime — never
commit them to the repo.

## Point Mihani at it

Once deployed (URL like `https://mihani-gw.yourname.workers.dev`), set:

```
MIHANI_GATEWAY=https://mihani-gw.yourname.workers.dev
MIHANI_GATEWAY_TOKEN=<your-client-token>
```

Close any open Mihani windows and relaunch — the built-in providers will now
route through your worker.
