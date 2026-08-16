package history

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Entry represents a history entry.
type Entry struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	Content   string    `json:"content"`
	Result    string    `json:"result,omitempty"`
	Timestamp time.Time `json:"timestamp"`
	SessionID string    `json:"session_id"`
}

// Session represents a command session.
type Session struct {
	ID        string    `json:"id"`
	StartedAt time.Time `json:"started_at"`
	EndedAt   time.Time `json:"ended_at,omitempty"`
	Entries   []Entry   `json:"entries"`
}

// Manager handles command history persistence.
type Manager struct {
	filePath  string
	maxEntries int
	mu        sync.RWMutex
	entries   []Entry
	sessions  map[string]*Session
	currentSessionID string
	dirty     bool
}

// NewManager creates a new history manager.
func NewManager(filePath string, maxEntries int) (*Manager, error) {
	mgr := &Manager{
		filePath:  filePath,
		maxEntries: maxEntries,
		entries:   make([]Entry, 0),
		sessions:  make(map[string]*Session),
		dirty:     false,
	}

	if err := mgr.Load(); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to load history: %w", err)
	}

	return mgr, nil
}

// Load loads history from file.
func (m *Manager) Load() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	data, err := os.ReadFile(m.filePath)
	if err != nil {
		return err
	}

	var stored struct {
		Entries  []Entry           `json:"entries"`
		Sessions map[string]*Session `json:"sessions"`
	}

	if err := json.Unmarshal(data, &stored); err != nil {
		return fmt.Errorf("failed to parse history file: %w", err)
	}

	m.entries = stored.Entries
	m.sessions = stored.Sessions

	// Trim to max entries if needed
	if len(m.entries) > m.maxEntries {
		m.entries = m.entries[len(m.entries)-m.maxEntries:]
	}

	return nil
}

// Save saves history to file.
func (m *Manager) Save() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.dirty {
		return nil
	}

	// Ensure directory exists
	dir := filepath.Dir(m.filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create history directory: %w", err)
	}

	stored := struct {
		Entries  []Entry           `json:"entries"`
		Sessions map[string]*Session `json:"sessions"`
	}{
		Entries:  m.entries,
		Sessions: m.sessions,
	}

	data, err := json.MarshalIndent(stored, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal history: %w", err)
	}

	if err := os.WriteFile(m.filePath, data, 0600); err != nil {
		return fmt.Errorf("failed to write history file: %w", err)
	}

	m.dirty = false
	return nil
}

// Add adds a new entry to history.
func (m *Manager) Add(entryType, content, result string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	entry := Entry{
		ID:        fmt.Sprintf("entry-%d-%d", time.Now().UnixNano(), len(m.entries)),
		Type:      entryType,
		Content:   content,
		Result:    result,
		Timestamp: time.Now(),
		SessionID: m.currentSessionID,
	}

	m.entries = append(m.entries, entry)
	m.dirty = true

	// Add to current session
	if m.currentSessionID != "" {
		if session, ok := m.sessions[m.currentSessionID]; ok {
			session.Entries = append(session.Entries, entry)
		}
	}

	// Trim to max entries
	if len(m.entries) > m.maxEntries {
		m.entries = m.entries[len(m.entries)-m.maxEntries:]
	}
}

// StartSession starts a new session.
func (m *Manager) StartSession(sessionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.currentSessionID = sessionID
	m.sessions[sessionID] = &Session{
		ID:        sessionID,
		StartedAt: time.Now(),
		Entries:   make([]Entry, 0),
	}
	m.dirty = true
}

// EndSession ends the current session.
func (m *Manager) EndSession() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.currentSessionID != "" {
		if session, ok := m.sessions[m.currentSessionID]; ok {
			session.EndedAt = time.Now()
		}
	}
	m.dirty = true
}

// GetRecent returns recent history entries.
func (m *Manager) GetRecent(n int) []Entry {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if n <= 0 || n > len(m.entries) {
		n = len(m.entries)
	}

	start := len(m.entries) - n
	if start < 0 {
		start = 0
	}

	return m.entries[start:]
}

// GetAll returns all history entries.
func (m *Manager) GetAll() []Entry {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]Entry, len(m.entries))
	copy(result, m.entries)
	return result
}

// Search searches history by query.
func (m *Manager) Search(query string) []Entry {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var results []Entry
	queryLower := toLower(query)

	for _, entry := range m.entries {
		if containsIgnoreCase(entry.Content, queryLower) ||
		   containsIgnoreCase(entry.Type, queryLower) ||
		   containsIgnoreCase(entry.Result, queryLower) {
			results = append(results, entry)
		}
	}

	return results
}

// GetSession returns a session by ID.
func (m *Manager) GetSession(sessionID string) *Session {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.sessions[sessionID]
}

// GetSessions returns all sessions.
func (m *Manager) GetSessions() []*Session {
	m.mu.RLock()
	defer m.mu.RUnlock()

	sessions := make([]*Session, 0, len(m.sessions))
	for _, s := range m.sessions {
		sessions = append(sessions, s)
	}

	return sessions
}

// Clear clears all history.
func (m *Manager) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.entries = make([]Entry, 0)
	m.sessions = make(map[string]*Session)
	m.currentSessionID = ""
	m.dirty = true
}

// ClearSession clears a specific session.
func (m *Manager) ClearSession(sessionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.sessions, sessionID)
	m.dirty = true
}

// Stats returns history statistics.
func (m *Manager) Stats() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return map[string]interface{}{
		"total_entries":  len(m.entries),
		"total_sessions": len(m.sessions),
		"max_entries":    m.maxEntries,
		"file_path":      m.filePath,
	}
}

func toLower(s string) string {
	result := make([]rune, len(s))
	for i, r := range s {
		if r >= 'A' && r <= 'Z' {
			result[i] = r + 32
		} else {
			result[i] = r
		}
	}
	return string(result)
}

func containsIgnoreCase(s, substr string) bool {
	sLower := toLower(s)
	return len(sLower) >= len(substr) && (sLower == substr || findSubstring(sLower, substr))
}

func findSubstring(s, substr string) bool {
	if len(substr) == 0 {
		return true
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
