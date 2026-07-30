// Package annotate holds the session-scoped store of line comments —
// added by a human in the TUI, kode's own embedded agent, or an external MCP
// caller (e.g. Claude Code) — keyed by file and line so the diff view can
// render them inline as a thread. The store is also backed by a JSON file on
// disk (see LoadFile/SetPersistPath/Reload), so an agent that isn't
// connected live over MCP can still author notes by writing that file
// directly, and a running kode session can pick them up with a refresh.
package annotate

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Author sources. Human and KodeAgent are fixed; an MCP caller's source is
// "mcp:<client-name>", taken from that caller's MCP clientInfo.name.
const (
	Human     = "human"
	KodeAgent = "kode-agent"
)

type Annotation struct {
	ID        string    `json:"id,omitempty"`
	File      string    `json:"file"`
	Line      int       `json:"line"` // canonical line number: new-file line if present, else old-file line
	Author    string    `json:"author"`
	Text      string    `json:"text"`
	CreatedAt time.Time `json:"created_at,omitempty"`
}

// computeID derives a stable, content-based ID when one isn't already set.
// Stability matters: reloading the same JSON file twice must recognize
// already-known entries as duplicates rather than re-adding them, and this
// works because CreatedAt is fixed once (either loaded from the file or set
// once at first Add) rather than recomputed per reload.
func computeID(a Annotation) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%d\x00%s\x00%s\x00%s",
		a.File, a.Line, a.Author, a.Text, a.CreatedAt.UTC().Format(time.RFC3339Nano))))
	return hex.EncodeToString(sum[:6])
}

// LoadFile parses a JSON array of annotations from path. A missing file is
// not an error — it just means no annotations have been pushed yet.
func LoadFile(path string) ([]Annotation, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var list []Annotation
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return list, nil
}

// Store is safe for concurrent use: the TUI's Update loop and the MCP
// server's goroutine both add annotations.
type Store struct {
	mu          sync.RWMutex
	annotations []Annotation
	persistPath string

	// onChange, if set, is called after every newly-added annotation (not
	// re-called for duplicates) — kode wires this to push a tea.Msg into the
	// running Bubble Tea program so the diff view repaints regardless of
	// whether the annotation came from a keypress, an external MCP call, or
	// a JSON file reload.
	onChange func(Annotation)
}

func NewStore() *Store {
	return &Store{}
}

func (s *Store) OnChange(fn func(Annotation)) {
	s.mu.Lock()
	s.onChange = fn
	s.mu.Unlock()
}

// SetPersistPath configures where the store's contents are written after
// every new annotation. Pass "" to disable persistence.
func (s *Store) SetPersistPath(path string) {
	s.mu.Lock()
	s.persistPath = path
	s.mu.Unlock()
}

// Add inserts a into the store, assigning it a content-derived ID if it
// doesn't already have one. If an annotation with the same ID already exists
// (e.g. it was already loaded from the persisted file), Add is a no-op and
// returns the existing entry — this makes Reload safe to call repeatedly.
func (s *Store) Add(a Annotation) Annotation {
	result, isNew := s.insert(a)
	if isNew {
		s.persist()
		s.fire(result)
	}
	return result
}

// Reload re-reads the persisted JSON file and merges in any annotations not
// already known to the store (e.g. pushed there by an agent writing the file
// directly, outside of any live MCP connection). Returns how many were new.
func (s *Store) Reload(path string) (int, error) {
	list, err := LoadFile(path)
	if err != nil {
		return 0, err
	}

	var added []Annotation
	for _, a := range list {
		if result, isNew := s.insert(a); isNew {
			added = append(added, result)
		}
	}
	if len(added) > 0 {
		s.persist()
		for _, a := range added {
			s.fire(a)
		}
	}
	return len(added), nil
}

func (s *Store) insert(a Annotation) (Annotation, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if a.CreatedAt.IsZero() {
		a.CreatedAt = time.Now()
	}
	if a.ID == "" {
		a.ID = computeID(a)
	}
	for _, existing := range s.annotations {
		if existing.ID == a.ID {
			return existing, false
		}
	}
	s.annotations = append(s.annotations, a)
	return a, true
}

func (s *Store) fire(a Annotation) {
	s.mu.RLock()
	onChange := s.onChange
	s.mu.RUnlock()
	if onChange != nil {
		onChange(a)
	}
}

// persist writes the full store to disk. Failures are logged, not returned
// or panicked on — persistence is a convenience, and a write failure must
// never take down the TUI or drop the in-memory annotation.
func (s *Store) persist() {
	s.mu.RLock()
	path := s.persistPath
	list := make([]Annotation, len(s.annotations))
	copy(list, s.annotations)
	s.mu.RUnlock()

	if path == "" {
		return
	}

	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		log.Printf("kode: annotate: marshal %s: %v", path, err)
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		log.Printf("kode: annotate: mkdir for %s: %v", path, err)
		return
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		log.Printf("kode: annotate: write %s: %v", tmp, err)
		return
	}
	if err := os.Rename(tmp, path); err != nil {
		log.Printf("kode: annotate: rename %s -> %s: %v", tmp, path, err)
	}
}

func (s *Store) ForFile(file string) []Annotation {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []Annotation
	for _, a := range s.annotations {
		if a.File == file {
			out = append(out, a)
		}
	}
	return out
}

func (s *Store) All() []Annotation {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Annotation, len(s.annotations))
	copy(out, s.annotations)
	return out
}
