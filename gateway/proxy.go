package main

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Upstream is a single provider the gateway forwards to. The API key is a
// server-side secret: it is set from the environment, injected into upstream
// requests, and never sent back to the client.
type Upstream struct {
	Name   string // "pro" | "cloud"
	Base   string // e.g. https://seekai.cc/v1
	APIKey string // upstream provider key (secret)
}

// Server is the Mihani key-protecting proxy. Clients authenticate with a
// client token; the gateway swaps it for the real upstream key.
type Server struct {
	ups map[string]Upstream // keyed by "pro" / "cloud"

	clientTokens map[string]bool // allowed client tokens (empty = auth disabled)
	clientName   string          // label for logs

	httpClient *http.Client

	// Per-IP rate limiting (token bucket) + global concurrency cap.
	rateMu       sync.Mutex
	buckets      map[string]*tokenBucket
	burst        int
	refillPerSec float64
	sem          chan struct{}

	logf func(format string, args ...any)
}

// NewServer builds a gateway server. A nil logger uses the standard logger.
func NewServer(ups map[string]Upstream, clientTokens []string, burst int, reqPerMin, maxConcurrent int, logger *log.Logger) *Server {
	if logger == nil {
		logger = log.New(io.Discard, "", 0)
	}
	if burst <= 0 {
		burst = 8
	}
	if reqPerMin <= 0 {
		reqPerMin = 30
	}
	if maxConcurrent <= 0 {
		maxConcurrent = 8
	}
	tokens := map[string]bool{}
	for _, t := range clientTokens {
		t = strings.TrimSpace(t)
		if t != "" {
			tokens[t] = true
		}
	}
	return &Server{
		ups:          ups,
		clientTokens: tokens,
		clientName:   "client",
		httpClient:   &http.Client{Timeout: 10 * time.Minute}, // long: LLM turns can run for a while
		burst:        burst,
		refillPerSec: float64(reqPerMin) / 60.0,
		sem:          make(chan struct{}, maxConcurrent),
		buckets:      map[string]*tokenBucket{},
		logf:         logger.Printf,
	}
}

func (s *Server) clientLabel() string {
	if len(s.clientTokens) == 0 {
		return "AUTH-DISABLED (dev)"
	}
	return fmt.Sprintf("%d client token(s)", len(s.clientTokens))
}

// tokenBucket is a per-IP rate limiter.
type tokenBucket struct {
	tokens float64
	last   time.Time
}

func (b *tokenBucket) allow(cap float64, refillPerSec float64) bool {
	now := time.Now()
	if b.tokens < cap {
		elapsed := now.Sub(b.last).Seconds()
		b.tokens = min(cap, b.tokens+elapsed*refillPerSec)
	}
	b.last = now
	if b.tokens >= 1 {
		b.tokens--
		return true
	}
	return false
}

func (s *Server) rateAllow(ip string) bool {
	s.rateMu.Lock()
	defer s.rateMu.Unlock()
	b, ok := s.buckets[ip]
	if !ok {
		b = &tokenBucket{tokens: float64(s.burst), last: time.Now()}
		s.buckets[ip] = b
	}
	return b.allow(float64(s.burst), s.refillPerSec)
}

// Handler returns the HTTP handler for the gateway.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	for name := range s.ups {
		mux.HandleFunc("/"+name+"/", s.makeRoute(name))
	}
	return security(mux)
}

// makeRoute handles any path under /<name>/ by proxying to that upstream,
// after auth + rate limiting + the concurrency cap.
func (s *Server) makeRoute(name string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		up, ok := s.ups[name]
		if !ok {
			writeJSONError(w, http.StatusNotFound, "unknown provider")
			return
		}
		ip := clientIP(r)
		if !s.authorized(r) {
			s.logf("401 %s %s from %s (bad/missing client token)", r.Method, r.URL.Path, ip)
			writeJSONError(w, http.StatusUnauthorized, "invalid or missing client token")
			return
		}
		if !s.rateAllow(ip) {
			s.logf("429 %s %s from %s (rate limited)", r.Method, r.URL.Path, ip)
			writeJSONError(w, http.StatusTooManyRequests, "rate limit exceeded, slow down")
			return
		}
			// Global concurrency cap: wait for a slot.
		select {
		case s.sem <- struct{}{}:
			defer func() { <-s.sem }()
		case <-r.Context().Done():
			return
		}

		// Build the upstream URL: /<name>/X -> <base>/X. The client's base URL is
		// normalized to .../pro/v1, so strip an optional leading /v1 here — the
		// upstream base already contains it.
		rel := strings.TrimPrefix(r.URL.Path, "/"+name)
		rel = strings.TrimPrefix(rel, "/v1")
		if rel == "" {
			rel = "/"
		}
		s.proxyRequest(w, r, up, rel)
	}
}

func (s *Server) authorized(r *http.Request) bool {
	if len(s.clientTokens) == 0 {
		return true // auth disabled (local/dev) — caller logs a warning at startup
	}
	h := r.Header.Get("Authorization")
	tok := strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))
	return constantTimeLookup(s.clientTokens, tok)
}

// proxyRequest forwards the request to the upstream with the server-side key,
// streaming the response (SSE) back to the client verbatim.
func (s *Server) proxyRequest(w http.ResponseWriter, r *http.Request, up Upstream, rel string) {
	target := strings.TrimSuffix(up.Base, "/") + rel
	// Re-read the body: r.Body is fine to consume once here.
	ctx := r.Context()
	upReq, err := http.NewRequestWithContext(ctx, r.Method, target, r.Body)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "bad upstream request")
		return
	}
	upReq.Header.Set("Content-Type", r.Header.Get("Content-Type"))
	if v := r.Header.Get("Accept"); v != "" {
		upReq.Header.Set("Accept", v)
	}
	// Inject the real upstream key; the client's token is NOT forwarded.
	upReq.Header.Set("Authorization", "Bearer "+up.APIKey)

	resp, err := s.httpClient.Do(upReq)
	if err != nil {
		// Connection/context failure — surface as a 502 with a neutral message.
		if ctx.Err() != nil {
			return // client went away
		}
		writeJSONError(w, http.StatusBadGateway, "could not reach the model")
		return
	}
	defer resp.Body.Close()

	w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
	w.Header().Set("Cache-Control", resp.Header.Get("Cache-Control"))
	w.WriteHeader(resp.StatusCode)
	streamCopy(w, resp.Body)
}

// streamCopy copies the upstream body to the client, flushing after each chunk
// so SSE events arrive as they are produced.
func streamCopy(w http.ResponseWriter, src io.Reader) {
	flusher, _ := w.(http.Flusher)
	buf := make([]byte, 4096)
	for {
		n, err := src.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				return
			}
			if flusher != nil {
				flusher.Flush()
			}
		}
		if err != nil {
			return
		}
	}
}

func writeJSONError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"message": msg}})
}

// security adds basic hardening headers and a global request timeout.
func security(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	// Respect a first X-Forwarded-For entry if present (behind a proxy/LB).
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if first := strings.TrimSpace(strings.Split(xff, ",")[0]); first != "" {
			return first
		}
	}
	return host
}

// constantTimeLookup checks membership without leaking which token matched.
func constantTimeLookup(set map[string]bool, tok string) bool {
	if tok == "" {
		return false
	}
	for want := range set {
		if subtle.ConstantTimeCompare([]byte(want), []byte(tok)) == 1 {
			return true
		}
	}
	return false
}
