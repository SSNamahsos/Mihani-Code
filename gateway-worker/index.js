// Mihani Gateway — Cloudflare Worker (simplified, correct routing)
// Routes /pro/* -> PRO_BASE, /cloud/* -> CLOUD_BASE
// Strips the /pro or /cloud prefix (and optional /v1) before forwarding.
//
// Secrets (set via wrangler secret or Dashboard):
//   PRO_BASE, PRO_KEY, CLOUD_BASE, CLOUD_KEY, CLIENT_TOKENS

const ROUTES = {
  pro:   { base: null,  key: null  },
  cloud: { base: null,  key: null  },
};

function init() {
  // Can't access env at module level in all CF runtimes, so initialize lazily.
}

function getRoute(pathname) {
  if (pathname.startsWith("/pro")) return "pro";
  if (pathname.startsWith("/cloud")) return "cloud";
  return null;
}

// Strip /pro or /cloud (and optional trailing /v1) from pathname.
// "/pro/v1/chat/completions" -> "/chat/completions"
// "/cloud/models" -> "/models"
function stripRoute(pathname) {
  let p = pathname;
  if (p.startsWith("/pro")) p = p.slice(4);
  else if (p.startsWith("/cloud")) p = p.slice(6);
  // Remove leading /v1 if present (client normalizes bases to end in /v1)
  if (p.startsWith("/v1")) p = p.slice(3);
  return p || "/";
}

async function proxy(req, upstreamBase, apiKey) {
  const reqUrl = new URL(req.url);
  const relPath = stripRoute(reqUrl.pathname);
  const target = upstreamBase + relPath;

  // Build clean headers — only pass through what upstream expects
  const upHeaders = new Headers();
  const passthrough = new Set([
    "authorization", "content-type", "accept", "accept-language",
    "user-agent", "origin", "referer", "cache-control",
    "connection", "pragma", "x-requested-with",
  ]);
  for (const [k] of req.headers) {
    if (passthrough.has(k.toLowerCase())) {
      upHeaders.set(k, req.headers.get(k));
    }
  }
  upHeaders.set("Host", new URL(target).host);
  upHeaders.set("Authorization", "Bearer " + apiKey);
  upHeaders.set("User-Agent", "mihani-code/0.2.28");

  const bodyBuf = req.body ? await req.arrayBuffer() : undefined;
  const upstream = new Request(target, {
    method: req.method,
    headers: upHeaders,
    body: bodyBuf,
  });

  const resp = await fetch(upstream);
  const ct = resp.headers.get("Content-Type") || "";
  const outHeaders = new Headers();
  outHeaders.set("Content-Type", ct);
  outHeaders.set("Cache-Control", "no-store");
  const text = await resp.text();
  return new Response(text, { status: resp.status, headers: outHeaders });
}

export default {
  async fetch(req, env, ctx) {
    const url = new URL(req.url);
    const ip = req.headers.get("CF-Connecting-IP") || "local";
    // Health check — no auth needed
    if (url.pathname === "/health" && req.method === "GET") {
      return new Response(
        JSON.stringify({ ok: true }),
        { headers: { "Content-Type": "application/json" } }
      );
    }

    const route = getRoute(url.pathname);
    if (!route) {
      return new Response("not found", { status: 404 });
    }

    // Auth
    const auth = req.headers.get("Authorization") || "";
    const tok = auth.startsWith("Bearer ") ? auth.slice(7).trim() : auth;
    const tokens = (env.CLIENT_TOKENS || "").split(",").map(t => t.trim()).filter(Boolean);
    if (tokens.length > 0 && !tokens.includes(tok)) {
      return new Response(
        JSON.stringify({ error: { message: "invalid or missing client token" } }),
        { status: 401, headers: { "Content-Type": "application/json" } }
      );
    }

    // Rate limit (per-IP, memory-based)
    // (skipped for simplicity — Cloudflare has its own rate limiting)

    const upstreamBase = route === "pro" ? (env.PRO_BASE || "") : (env.CLOUD_BASE || "");
    const apiKey = route === "pro" ? (env.PRO_KEY || "") : (env.CLOUD_KEY || "");

    if (!upstreamBase || !apiKey) {
      return new Response(
        JSON.stringify({ error: { message: "provider not configured" } }),
        { status: 500, headers: { "Content-Type": "application/json" } }
      );
    }

    try {
      return await proxy(req, upstreamBase, apiKey);
    } catch (err) {
      return new Response(
        JSON.stringify({ error: { message: "upstream error" } }),
        { status: 502, headers: { "Content-Type": "application/json" } }
      );
    }
  },
};
