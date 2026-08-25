// Package secrets holds provider API keys out of the source tree and provides
// a universal redactor so secret values can never reach tool output, the
// transcript, or persisted files.
//
// Credentials live in internal/secrets/blob.bin (git-ignored). Each line is a
// hex string XOR-encoded with the in-code mask; blob.example.bin is the
// committed placeholder for source-only builds. This keeps real keys out of
// version control while shipping them inside distributed binaries.
//
// Keys are never written to config.json, never exported to environment
// variables, and every string that flows back from a tool is passed through
// Redact before the model or UI sees it.
package secrets

import (
	_ "embed"
	"encoding/hex"
	"strings"
	"sync"
)

//go:embed blob.bin
var rawBlob string

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

func blobLines() []string {
	return strings.Split(strings.ReplaceAll(rawBlob, "\r\n", "\n"), "\n")
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
