# Mihani Gateway — Cloudflare Worker

Single-file Worker that proxies `/pro/...` and `/cloud/...` to the upstream
providers while holding the real API keys server-side. Clients authenticate
with a token you issue; the upstream key is never exposed.

## Environment variables (set in Cloudflare Dashboard)

| Variable       | Required | Example                  |
|----------------|----------|--------------------------|
| `PRO_BASE`     | yes      | `https://your-pro-upstream/v1` |
| `PRO_KEY`      | yes      | `sk-...`                 |
| `CLOUD_BASE`   | yes      | `https://your-cloud-upstream/v1` |
| `CLOUD_KEY`    | yes      | `sk-...`                 |
| `CLIENT_TOKENS` | yes     | `tok_abc123,tok_def456`  |

## Local dev

```sh
wrangler dev
```

Then test:
```sh
TOKEN=<your-token>
curl http://127.0.0.1:8787/health
curl http://127.0.0.1:8787/pro/models -H "Authorization: Bearer $TOKEN"
curl http://127.0.0.1:8787/cloud/models -H "Authorization: Bearer $TOKEN"
```

## Deploy

```sh
wrangler deploy
```

The URL will be something like `https://mihani-gw.<YOUR_SUBDOMAIN>.workers.dev`.
Set `MIHANI_GATEWAY=https://mihani-gw.<YOUR_SUBDOMAIN>.workers.dev` and
`MIHANI_GATEWAY_TOKEN=<token>` on your machine (see README).
