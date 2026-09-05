// Package secrets redacts provider API keys from tool output, the
// transcript, and persisted files — so credentials can never leak to the
// model, the user, or disk.
//
// For local development only: drop a git-ignored blob.bin with
// XOR-masked hex entries (see secrets.go). The committed placeholder is
// empty, so distributed binaries build with NO embedded credentials.
//
// To use the built-in providers without embedded keys, deploy the
// key-protecting gateway (gateway-worker/) and point the client at it
// via MIHANI_GATEWAY.
package secrets

import (
	"embed"
	"encoding/hex"
	"strings"
	"sync"
)

//go:embed *.bin
var blobFS embed.FS

// xorMask decodes the embedded blobs below. It exists only in code, not in
// any config file or environment variable.
var xorMask = []byte("mihani-secret-v1")

func decode(blob string) string {
	raw, err := hex.DecodeString(strings.TrimSpace(blob))
	if err != nil || len(raw) == 0 {
		return ""
	}
	out := make([]byte, len(raw))
	for i, b := range raw {
		out[i] = b ^ xorMask[i%len(xorMask)]
	}
	return string(out)
}

// blobLines returns the non-empty encoded entries across every embedded .bin
// file, so the committed placeholder alone yields no credentials while a
// dropped-in blob.bin simply appends its real entries.
func blobLines() []string {
	entries, err := blobFS.ReadDir(".")
	if err != nil {
		return nil
	}
	var kept []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".bin") {
			continue
		}
		data, readErr := blobFS.ReadFile(entry.Name())
		if readErr != nil {
			continue
		}
		for _, line := range strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n") {
			if strings.TrimSpace(line) != "" {
				kept = append(kept, line)
			}
		}
	}
	return kept
}

func line(i int) string {
	lines := blobLines()
	if i >= len(lines) {
		return ""
	}
	return decode(lines[i])
}

// Embedded provider credentials (XOR-obfuscated, loaded from blob.bin).
var (
	primaryKey = line(0)
	secondKey  = line(1)
)

// Primary returns the credential for the primary built-in endpoint.
func Primary() string { return primaryKey }

// Secondary returns the credential for the secondary built-in endpoint.
func Secondary() string { return secondKey }

const redacted = "[redacted]"

var (
	mu       sync.RWMutex
	registry []string
)

func init() {
	Register(primaryKey)
	Register(secondKey)
}

// Register adds a secret to the redaction set. Short values are ignored to
// avoid mangling ordinary text.
func Register(secret string) {
	if len(secret) < 8 {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	for _, existing := range registry {
		if existing == secret {
			return
		}
	}
	registry = append(registry, secret)
}

// Redact replaces every registered secret found in s with [redacted].
func Redact(s string) string {
	mu.RLock()
	defer mu.RUnlock()
	for _, secret := range registry {
		if strings.Contains(s, secret) {
			s = strings.ReplaceAll(s, secret, redacted)
		}
	}
	return s
}
