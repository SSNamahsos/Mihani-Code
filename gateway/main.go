package main

import (
	"bufio"
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Environment variables the gateway reads. Upstream API keys are secrets that
// stay on the server; clients authenticate only with a client token.
//
//	PRO_BASE, PRO_KEY       upstream for the "Mihani Pro" route (/pro/...)
//	CLOUD_BASE, CLOUD_KEY   upstream for the "Mihani Cloud" route (/cloud/...)
//	CLIENT_TOKENS           comma-separated client tokens the app presents
//	PORT                    listen port (default :8080)
//	RATE_PER_MIN            sustained requests/minute per IP (default 30)
//	BURST                   per-IP burst (default 8)
//	MAX_CONCURRENT          global in-flight upstream requests (default 8)
//
// PRO_BASE/PRO_KEY and CLOUD_BASE/CLOUD_KEY are required. CLIENT_TOKENS is
// required in production; if empty the gateway runs with auth disabled and
// logs a loud warning (intended only for local development).
func main() {
	logger := log.New(os.Stdout, "[mihani-gateway] ", log.LstdFlags)

	// Load local env from a git-ignored file so secrets stay out of the code
	// and out of the repo. Existing env vars (CI/secret managers) win.
	loadDotEnv()

	ups := map[string]Upstream{
		"pro":   {Name: "pro", Base: getenv("PRO_BASE"), APIKey: getenv("PRO_KEY")},
		"cloud": {Name: "cloud", Base: getenv("CLOUD_BASE"), APIKey: getenv("CLOUD_KEY")},
	}
	missing := []string{}
	for name, up := range ups {
		if up.Base == "" || up.APIKey == "" {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		logger.Fatalf("missing required env for upstream %v — set PRO_BASE/PRO_KEY and CLOUD_BASE/CLOUD_KEY", missing)
	}

	tokens := splitTokens(getenv("CLIENT_TOKENS"))
	if len(tokens) == 0 {
		logger.Printf("WARNING: CLIENT_TOKENS is empty — client auth is DISABLED. This must only be used for local development, never in production.")
	}

	port := getenv("PORT")
	if port == "" {
		port = "8080"
	}
	if !strings.HasPrefix(port, ":") {
		port = ":" + port
	}

	srv := NewServer(ups, tokens,
		envInt("BURST", 8), envInt("RATE_PER_MIN", 30), envInt("MAX_CONCURRENT", 8),
		logger)

	httpSrv := &http.Server{
		Addr:              port,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 30 * time.Second,
		// No Read/Write Timeout: LLM responses stream for a long time and must
		// not be cut off. Per-request context + concurrency cap bound resources.
	}

	go func() {
		logger.Printf("listening on %s (upstreams: %s, %s; %s)",
			port, ups["pro"].Name, ups["cloud"].Name, srv.clientLabel())
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatalf("server error: %v", err)
		}
	}()

	// Graceful shutdown on SIGINT/SIGTERM.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	logger.Printf("shutting down")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = httpSrv.Shutdown(ctx)
}

func getenv(k string) string { return strings.TrimSpace(os.Getenv(k)) }

func envInt(k string, def int) int {
	if v := getenv(k); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func splitTokens(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// loadDotEnv reads KEY=VALUE lines from GATEWAY_ENV, or .env.local, or .env in
// the working directory (first that exists). Values already present in the
// environment are NOT overridden, so CI/secret-manager values win. Comments
// (#) and blank lines are skipped; optional surrounding quotes are stripped.
func loadDotEnv() {
	candidates := []string{}
	if p := strings.TrimSpace(os.Getenv("GATEWAY_ENV")); p != "" {
		candidates = append(candidates, p)
	}
	candidates = append(candidates, ".env.local", ".env")
	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			applyDotEnv(path)
			return
		}
	}
}

func applyDotEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		eq := strings.IndexByte(line, '=')
		if eq <= 0 {
			continue
		}
		k := strings.TrimSpace(line[:eq])
		v := strings.TrimSpace(line[eq+1:])
		if len(v) >= 2 && (v[0] == '"' || v[0] == '\'') && v[len(v)-1] == v[0] {
			v = v[1 : len(v)-1]
		}
		if _, exists := os.LookupEnv(k); !exists {
			os.Setenv(k, v)
		}
	}
}
