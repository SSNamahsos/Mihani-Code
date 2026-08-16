// Package history manages command and chat history.
package history

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Entry represents a single history entry.
type Entry struct {
	Timestamp time.Time `json:"timestamp"`
	Type      string    `json:"type"` // "command", "chat", "file_op"
	Content   string    `json:"content"`
	Metadata  string    `json:"metadata,omitempty"`
}

// Session represents a complete session with multiple entries.
type Session struct {
	ID        string    `json:"id"`
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`
	Entries   []Entry   `json:"entries"`
}

// Manager handles history persistence and retrieval.
type Manager struct {
	historyFile string
	maxEntries  int
	entries     []Entry
	currentSession *Session
}

// NewManager creates a new history manager.
func NewManager(historyFile string, maxEntries int) (*Manager, error) {
	m := &Manager{
		historyFile: historyFile,
		maxEntries:  maxEntries,
		entries:     make([]Entry, 0),
	}

	if err := m.load(); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to load history: %w", err)
	}

	return m, nil
}

// Add adds an entry to the history.
func (m *Manager) Add(entryType, content, metadata string) {
	entry := Entry{
		Timestamp: time.Now(),
		Type:      entryType,
		Content:   content,
		Metadata:  metadata,
	}

	m.entries = append(m.entries, entry)

	// Trim if exceeds max
	if len(m.entries) > m.maxEntries {
		m.entries = m.entries[len(m.entries)-m.maxEntries:]
	}

	// Add to current session
	if m.currentSession != nil {
		m.currentSession.Entries = append(m.currentSession.Entries, entry)
	}
}

// GetRecent returns the most recent entries.
func (m *Manager) GetRecent(n int) []Entry {
	if n > len(m.entries) {
		n = len(m.entries)
	}
	start := len(m.entries) - n
	return m.entries[start:]
}

// GetAll returns all history entries.
func (m *Manager) GetAll() []Entry {
	return m.entries
}

// Search searches history by content.
func (m *Manager) Search(query string) []Entry {
	var results []Entry
	for _, entry := range m.entries {
		if containsIgnoreCase(entry.Content, query) {
			results = append(results, entry)
		}
	}
	return results
}

// Clear clears all history.
func (m *Manager) Clear() error {
	m.entries = make([]Entry, 0)
	if m.currentSession != nil {
		m.currentSession.Entries = make([]Entry, 0)
	}
	return m.save()
}

// StartSession begins a new session.
func (m *Manager) StartSession(id string) {
	m.currentSession = &Session{
		ID:        id,
		StartTime: time.Now(),
		Entries:   make([]Entry, 0),
	}
}

// EndSession ends the current session.
func (m *Manager) EndSession() {
	if m.currentSession != nil {
		m.currentSession.EndTime = time.Now()
	}
}

// GetCurrentSession returns the current session.
func (m *Manager) GetCurrentSession() *Session {
	return m.currentSession
}

// Save persists history to disk.
func (m *Manager) save() error {
	dir := filepath.Dir(m.historyFile)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create history directory: %w", err)
	}

	data, err := json.MarshalIndent(m.entries, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal history: %w", err)
	}

	if err := os.WriteFile(m.historyFile, data, 0600); err != nil {
		return fmt.Errorf("failed to write history file: %w", err)
	}

	return nil
}

// Save saves history to disk (public method).
func (m *Manager) Save() error {
	return m.save()
}

// load reads history from disk.
func (m *Manager) load() error {
	data, err := os.ReadFile(m.historyFile)
	if err != nil {
		return err
	}

	if err := json.Unmarshal(data, &m.entries); err != nil {
		return fmt.Errorf("failed to unmarshal history: %w", err)
	}

	return nil
}

// containsIgnoreCase checks if s contains substr (case-insensitive).
func containsIgnoreCase(s, substr string) bool {
	return len(s) >= len(substr) && 
		(s == substr || 
		 len(substr) == 0 ||
		 containsLower(s, substr))
}

func containsLower(s, substr string) bool {
	sLower := toLower(s)
	substrLower := toLower(substr)
	for i := 0; i <= len(sLower)-len(substrLower); i++ {
		if sLower[i:i+len(substrLower)] == substrLower {
			return true
		}
	}
	return false
}

func toLower(s string) string {
	result := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			result[i] = c + 32
		} else {
			result[i] = c
		}
	}
	return string(result)
}
