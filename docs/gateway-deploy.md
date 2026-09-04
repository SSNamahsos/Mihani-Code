# Mihani Gateway — key rotation & deployment

## 1. Rotate the leaked keys (do this first)

The old upstream keys shipped inside released binaries (recoverable via the
public XOR mask) — treat them as **compromised**. Rotate both:

**Mihani Pro (seekai.cc)**
1. Log in to the seekai.cc dashboard → your API key page.
2. **Revoke/delete the old key** (the one that was embedded — the `uGO…` / any
   earlier value). Do not just add a new one; the old one must be disabled.
3. Create a **new** key and note its scope/allowlist of models.
4. This is the value that goes into `PRO_KEY`.

**Mihani Cloud (api.hcnsec.cn)**
1. In the hcnsec dashboard, **revoke the old key** (the embedded `LLu…` value).
2. Create a **new** key.
3. This is the value that goes into `CLOUD_KEY`.

The two keys provided and stored in `gateway/.env.local` are assumed to be the
**new, post-rotation** keys. **Confirm the old ones are actually revoked**, not
merely replaced — otherwise the leaked keys still work.

> Note: your local `~/.mihani/config.json` also contains the old `personal_key`
> values for `hsenc`/`testseek`. Update or remove those after rotation, and never
> commit that file (it is already git-ignored).

## 2. Deploy the gateway

The gateway is a single static Go binary, no database. Pick any host that gives
you a public HTTPS URL.

### Fly.io (recommended, has a secret manager)
```sh
# from the gateway/ directory
fly launch --name mihani-gw --internal-port 8080
# set secrets (never in the repo):
fly secrets set PRO_BASE=https://seekai.cc/v1 PRO_KEY=<new-pro-key>
fly secrets set CLOUD_BASE=https://api.hcnsec.cn/v1 CLOUD_KEY=<new-cloud-key>
fly secrets set CLIENT_TOKENS=<long-random-token>
fly deploy
# your URL is printed, e.g. https://mihani-gw.fly.dev
```
`/health` and Fly's health-check can both be `GET /health`.

### Render
- New **Web Service** → root dir `gateway` → build `go build -o mihani-gateway .`
  → start `./mihani-gateway`.
- Add environment variables in the dashboard: `PRO_BASE`, `PRO_KEY`,
  `CLOUD_BASE`, `CLOUD_KEY`, `CLIENT_TOKENS`, `PORT` (Render sets this).

### A VPS
```sh
go build -o mihani-gateway .
export PRO_BASE=... PRO_KEY=... CLOUD_BASE=... CLOUD_KEY=... CLIENT_TOKENS=...
./mihani-gateway
# put nginx/caddy in front for TLS, or run behind the platform's TLS.
```

### Verify
```sh
curl https://<host>/health                       # -> {"ok":true}
curl https://<host>/pro/models -H "Authorization: Bearer <token>"     # lists models
curl https://<host>/cloud/models -H "Authorization: Bearer <token>"   # lists models
curl https://<host>/pro/models -H "Authorization: Bearer wrong"       # -> 401
```

## 3. Point the client at the gateway (opt-in)

Until you're ready to make it the default, enable it per-user via env:
```sh
export MIHANI_GATEWAY=https://<host>
export MIHANI_GATEWAY_TOKEN=<the-client-token>
mihani
```
To make it the shipped default later, set it in `config.defaults()` / the
installed binary (a follow-up). Only do that once the gateway is up **and** you
have stopped embedding keys (step 4).

## 4. Stop shipping embedded keys

Once the gateway is the path:
- Stop setting the `BLOB` secret in `.github/workflows/release.yml` (or empty it)
  so release binaries no longer embed upstream keys.
- The `secrets.Secondary()`/`Primary()` embedded values then only back a
  deprecated path; eventually remove them so new binaries carry no keys.

Existing already-distributed binaries still contain the **old** keys — that is
why rotation (step 1) is required and is not optional.

## Remaining work (not done here)
- **Per-user tokens + billing:** map each `CLIENT_TOKEN` to a balance/credit in a
  store, enforce per-token spend at the gateway, and let the client self-report
  or the gateway meter usage. The shared local daily budget stays as a client-side
  guardrail.
- **Upstream identity in errors:** neutralize upstream host names in surfaced
  error text if you don't want them revealed.
- **Load/monitoring:** the rate limiter + concurrency cap are in; add metrics and
  alerting on 5xx/429 rates and upstream spend.
