# Mihani Gateway (key-protecting proxy)

This is the server-side proxy that lets the Mihani Code client use the built-in
providers **without shipping the upstream API keys in the binary.**

## Why it exists

The Mihani Code client is a desktop binary. Historically it embedded the
upstream API keys (for `seekai.cc` / "Mihani Pro" and `api.hcnsec.cn` / "Mihani
Cloud"), obfuscated with a reversible XOR whose mask was in the open-source
code. That means anyone with the released `.exe` can recover the keys, so they
were effectively public.

**No client-side obfuscation can stop a binary from leaking a secret.** The only
way to keep the keys out of the client is to not put them in the client at all:
the client talks to *your* gateway, and the gateway holds the keys.

```
            ┌──────────────────────────────────────────────┐
 client     │  YOUR GATEWAY (server)                        │        upstream
 ─────────▶ │  - checks client token (auth)                 │  ─────────▶  seekai.cc
 (no keys)  │  - rate-limits per IP + caps concurrency      │              (Mihani Pro)
            │  - swaps token for the REAL upstream key      │  ─────────▶  api.hcnsec.cn
            │  - streams the response back (SSE)            │              (Mihani Cloud)
            └──────────────────────────────────────────────┘
```

The client never sees `PRO_KEY` / `CLOUD_KEY`. It only ever presents a **client
token** that you issue.

## Endpoints

The gateway is an OpenAI-compatible pass-through. It exposes two provider
routes that mirror the client's two built-ins:

| Route              | Forwards to      | Client built-in |
|--------------------|------------------|-----------------|
| `/pro/...`         | `PRO_BASE`       | Mihani Pro      |
| `/cloud/...`       | `CLOUD_BASE`     | Mihani Cloud    |
| `GET /health`      | —                | liveness probe  |

The client's base URLs are normalized to end in `/v1`, so the gateway strips an
optional leading `/v1` before forwarding. A request the client makes to
`<gw>/pro/v1/chat/completions` is forwarded to `<PRO_BASE>/chat/completions`
with `Authorization: Bearer <PRO_KEY>`.

Request/response bodies are passed through **verbatim**, including SSE streaming
(`text/event-stream`), so the client's existing streaming agent loop is
unaffected. The client token is stripped and replaced by the upstream key; it is
never forwarded upstream.

## Authentication

- The client sends `Authorization: Bearer <CLIENT_TOKEN>`.
- The gateway compares it (constant-time) against `CLIENT_TOKENS`
  (comma-separated). Mismatch → `401`, and the upstream is not called.
- **`CLIENT_TOKENS` must be set in production.** If it is empty, the gateway
  runs with auth disabled and logs a loud warning — for local development only.
- Today there is one shared token. Per-user tokens (below) are the planned
  extension for real multi-user billing.

## Rate limiting & safety

- **Per-IP token bucket** (`BURST` burst, `RATE_PER_MIN` sustained). Excess →
  `429`. The client IP is the first `X-Forwarded-For` entry when present, else
  the TCP peer.
- **Global concurrency cap** (`MAX_CONCURRENT`): a semaphore bounds in-flight
  upstream requests so one client cannot exhaust your upstream quota or the
  server's memory.
- The HTTP client has a **10-minute** per-request timeout (LLM turns can be
  long) but **no** server-level read/write timeout, so streaming responses are
  not cut off.
- Hardening headers (`no-store`, `nosniff`, `no-referrer`, `DENY`) and a 30s
  read-header timeout.

## Configuration (environment)

| Var              | Required | Default | Meaning                                   |
|------------------|----------|---------|-------------------------------------------|
| `PRO_BASE`       | yes      | —       | Mihani Pro upstream base (…/v1)           |
| `PRO_KEY`        | yes      | —       | **secret** upstream key (never exposed)   |
| `CLOUD_BASE`     | yes      | —       | Mihani Cloud upstream base (…/v1)         |
| `CLOUD_KEY`      | yes      | —       | **secret** upstream key (never exposed)   |
| `CLIENT_TOKENS`  | prod yes | —       | comma-separated client tokens             |
| `PORT`           | no       | `:8080` | listen address                            |
| `RATE_PER_MIN`   | no       | `30`    | sustained req/min per IP                  |
| `BURST`          | no       | `8`     | per-IP burst                              |
| `MAX_CONCURRENT` | no       | `8`     | global in-flight upstream requests        |

The gateway reads config from the environment. For local dev it also loads
`.env.local` (or `.env`) from the working directory, but **existing env vars win**
so a real deployment's secrets are never shadowed.

Secrets are stored:
- **In production:** as platform secrets (Fly/Render/VPS) — never in a file that
  gets committed.
- **Locally:** in `.env.local`, which is **git-ignored** and never committed.
- `gateway/.env.example` (committed) shows the shape with placeholders only.

## Building & running

```sh
cd gateway
go build -o mihani-gateway .
go test ./...

# local (reads .env.local):
./mihani-gateway

# production (set env / platform secrets):
PRO_BASE=... PRO_KEY=... CLOUD_BASE=... CLOUD_KEY=... \
CLIENT_TOKENS=... PORT=8080 ./mihani-gateway
```

## How the client uses it (opt-in)

The client change is safe and off by default. When the environment variable
`MIHANI_GATEWAY` is set, `config.Load` re-points the two built-ins at the
gateway and uses a client token:

| Env                    | Effect                                                              |
|------------------------|---------------------------------------------------------------------|
| `MIHANI_GATEWAY`       | e.g. `https://gw.example.com`. Built-in Pro → `<gw>/pro/v1`, Cloud → `<gw>/cloud/v1` |
| `MIHANI_GATEWAY_TOKEN` | the client token (must be in the gateway's `CLIENT_TOKENS`)         |

When `MIHANI_GATEWAY` is unset, the client behaves exactly as before. **Do not
ship a default that points at the gateway until it is deployed and you have
retired the embedded keys.** The migration order matters (see Migration below).

## Migration plan (order matters)

1. **Rotate** the current upstream keys (they leaked via old binaries). New keys
   = the ones in `.env.local`.
2. **Deploy** the gateway with the new keys + a client token.
3. **Switch the client** to the gateway (set `MIHANI_GATEWAY`, or later make it
   the shipped default).
4. **Stop shipping** embedded keys: remove `PRO_KEY`/`CLOUD_KEY` from the
   embedded credential blob / stop setting `BLOB` in CI, so new binaries contain
   no upstream keys at all. (The old binaries that already shipped the keys
   remain compromised — that is why rotation in step 1 is mandatory.)

## Security model & honest limits

- The upstream keys now live **only** on the gateway host. A leaked client
  binary reveals at most the **client token**, which grants rate-limited access
  to your two providers — it is revocable (drop it from `CLIENT_TOKENS`) and
  does not expose the upstream keys.
- This does **not** give per-user cost accounting. The client still self-reports
  usage against a local daily budget. True multi-user billing needs **per-user
  tokens** mapped to a balance/credit store, enforced by the gateway — the
  architecture supports it (`CLIENT_TOKENS` is already a set), the accounting
  store is the remaining work.
- The gateway should sit behind TLS (reverse proxy / platform). The upstream
  base URLs and model list are still visible in open source; if you want those
  hidden too, the gateway would need to hide the upstream identity from error
  messages as well (currently neutral).
