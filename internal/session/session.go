package session

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

type Record struct {
	ID        string           `json:"id"`
	Workspace string           `json:"workspace"`
	Title     string           `json:"title,omitempty"`
	Provider  string           `json:"provider"`
	Model     string           `json:"model"`
	Mode      string           `json:"mode"`
	CreatedAt time.Time        `json:"created_at,omitempty"`
	UpdatedAt time.Time        `json:"updated_at"`
	History   []map[string]any `json:"history,omitempty"`
}

// ID returns the workspace-hash id kept for backward compatibility.
func ID(workspace string) string {
	sum := sha256.Sum256([]byte(workspace))
	return hex.EncodeToString(sum[:])[:16]
}

// NewID mints a fresh timestamped session id.
func NewID() string {
	now := time.Now().UTC()
	var rnd [4]byte
	_, _ = rand.Read(rnd[:])
	return now.Format("20060102-150405") + "-" + hex.EncodeToString(rnd[:])
}

func dir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".mihani", "sessions"), nil
}

func Save(record Record) error {
	d, err := dir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(d, 0700); err != nil {
		return err
	}
	if record.ID == "" {
		record.ID = ID(record.Workspace)
	}
	if record.CreatedAt.IsZero() {
		if existing, err := Load(record.ID); err == nil {
			record.CreatedAt = existing.CreatedAt
			record.Title = firstNonEmpty(record.Title, existing.Title)
		}
		record.CreatedAt = time.Now().UTC()
	}
	record.UpdatedAt = time.Now().UTC()
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(d, record.ID+".json"), data, 0600)
}

func Load(id string) (Record, error) {
	d, err := dir()
	if err != nil {
		return Record{}, err
	}
	data, err := os.ReadFile(filepath.Join(d, id+".json"))
	if err != nil {
		return Record{}, err
	}
	var record Record
	err = json.Unmarshal(data, &record)
	return record, err
}

// List returns every stored session ordered most-recent first.
func List() ([]Record, error) {
	d, err := dir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(d)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var records []Record
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		id := filepath.Base(entry.Name())
		record, err := Load(id[:len(id)-len(".json")])
		if err != nil {
			continue
		}
		records = append(records, record)
	}
	sort.Slice(records, func(i, j int) bool {
		return records[i].UpdatedAt.After(records[j].UpdatedAt)
	})
	return records, nil
}

// LatestForWorkspace returns the newest session recorded in the workspace.
func LatestForWorkspace(workspace string) (Record, error) {
	records, err := List()
	if err != nil {
		return Record{}, err
	}
	for _, r := range records {
		if r.Workspace == workspace {
			return r, nil
		}
	}
	return Record{}, fmt.Errorf("no previous session for this workspace")
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
