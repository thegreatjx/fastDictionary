package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
)

type NotebookEntry struct {
	ID          string                  `json:"id"`
	Kind        string                  `json:"kind"`
	Source      string                  `json:"source"`
	Translation string                  `json:"translation,omitempty"`
	Grammar     *SentenceAnalysisResult `json:"grammar,omitempty"`
	Writing     *ImproveWritingResult   `json:"writing,omitempty"`
	CreatedAt   time.Time               `json:"created_at"`
}

type NotebookStore struct {
	mu          sync.RWMutex
	path        string
	entries     []NotebookEntry
	saveMu      sync.Mutex
	lastSaveErr error
}

var (
	globalNotebookStore *NotebookStore
	globalNotebookOnce  sync.Once
)

func GetNotebookStore() *NotebookStore {
	globalNotebookOnce.Do(func() {
		globalNotebookStore = newNotebookStore()
		migrateNotebookFromVocab(globalNotebookStore, GetVocabularyStore())
	})
	return globalNotebookStore
}

func defaultNotebookPath() string {
	if dir, err := mygo.App.Path(mygo.PathUserData); err == nil && dir != "" {
		return filepath.Join(dir, "notebook.json")
	}
	if appData := os.Getenv("APPDATA"); appData != "" {
		return filepath.Join(appData, appName, "notebook.json")
	}
	return "notebook.json"
}

func newNotebookStore() *NotebookStore {
	s := &NotebookStore{
		path:    defaultNotebookPath(),
		entries: []NotebookEntry{},
	}
	s.load()
	return s
}

func newMemoryNotebookStore() *NotebookStore {
	return &NotebookStore{
		path:    "",
		entries: []NotebookEntry{},
	}
}

func (s *NotebookStore) load() {
	if s.path == "" {
		return
	}
	data, err := os.ReadFile(s.path)
	if err != nil {
		return
	}
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	var entries []NotebookEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		log.Printf("fastDictionary: failed to parse notebook.json: %v", err)
		recovery := s.path + ".corrupt"
		if _, statErr := os.Stat(recovery); os.IsNotExist(statErr) {
			if writeErr := os.WriteFile(recovery, data, 0o644); writeErr != nil {
				log.Printf("fastDictionary: could not keep corrupt notebook copy: %v", writeErr)
			} else {
				log.Printf("fastDictionary: damaged notebook kept at %s", recovery)
			}
		}
		return
	}
	s.mu.Lock()
	s.entries = entries
	s.mu.Unlock()
}

func (s *NotebookStore) save() error {
	if s.path == "" {
		return nil
	}
	s.mu.RLock()
	entries := make([]NotebookEntry, len(s.entries))
	copy(entries, s.entries)
	path := s.path
	s.mu.RUnlock()

	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	s.saveMu.Lock()
	defer s.saveMu.Unlock()
	return atomicWriteFile(path, data, atomicWriteOptions{retries: 6, backup: true})
}

func (s *NotebookStore) saveOrLog() error {
	err := s.save()
	s.mu.Lock()
	s.lastSaveErr = err
	s.mu.Unlock()
	if err != nil {
		log.Printf("fastDictionary: notebook save failed: %v", err)
	}
	return err
}

func (s *NotebookStore) LastSaveError() error {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastSaveErr
}

func (s *NotebookStore) Path() string {
	if s == nil {
		return ""
	}
	return s.path
}

func (s *NotebookStore) Count() int {
	if s == nil {
		return 0
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.entries)
}

func (s *NotebookStore) KindCounts() (all, grammar, writing int) {
	if s == nil {
		return 0, 0, 0
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, e := range s.entries {
		all++
		switch e.Kind {
		case "writing":
			writing++
		default:
			grammar++
		}
	}
	return all, grammar, writing
}

func normalizeNotebookKind(kind string) string {
	if strings.EqualFold(strings.TrimSpace(kind), "writing") {
		return "writing"
	}
	return "grammar"
}

func (s *NotebookStore) Contains(kind, source string) bool {
	if s == nil {
		return false
	}
	k := normalizeNotebookKind(kind)
	normSrc := strings.TrimSpace(source)
	if normSrc == "" {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, e := range s.entries {
		if normalizeNotebookKind(e.Kind) == k && strings.TrimSpace(e.Source) == normSrc {
			return true
		}
	}
	return false
}

func (s *NotebookStore) Add(e NotebookEntry) bool {
	if s == nil {
		return false
	}
	e.Kind = normalizeNotebookKind(e.Kind)
	e.Source = strings.TrimSpace(e.Source)
	if e.Source == "" {
		return false
	}

	s.mu.Lock()
	if e.ID == "" {
		e.ID = fmt.Sprintf("note_%d", time.Now().UnixNano())
	}
	if e.CreatedAt.IsZero() {
		e.CreatedAt = time.Now()
	}

	for i, existing := range s.entries {
		if normalizeNotebookKind(existing.Kind) == e.Kind && strings.TrimSpace(existing.Source) == e.Source {
			if e.Translation != "" {
				existing.Translation = e.Translation
			}
			if e.Grammar != nil {
				existing.Grammar = e.Grammar
			}
			if e.Writing != nil {
				existing.Writing = e.Writing
			}
			s.entries[i] = existing
			s.mu.Unlock()
			s.saveOrLog()
			return true
		}
	}

	s.entries = append([]NotebookEntry{e}, s.entries...)
	s.mu.Unlock()
	s.saveOrLog()
	return true
}

func (s *NotebookStore) Delete(id string) bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	found := false
	next := make([]NotebookEntry, 0, len(s.entries))
	for _, e := range s.entries {
		if e.ID == id {
			found = true
			continue
		}
		next = append(next, e)
	}
	if found {
		s.entries = next
	}
	s.mu.Unlock()
	if found {
		s.saveOrLog()
	}
	return found
}

func (s *NotebookStore) Clear() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	s.entries = []NotebookEntry{}
	s.mu.Unlock()
	return s.saveOrLog()
}

func (s *NotebookStore) Filter(kind, query string) []NotebookEntry {
	if s == nil {
		return nil
	}
	q := strings.ToLower(strings.TrimSpace(query))
	k := strings.TrimSpace(kind)
	s.mu.RLock()
	defer s.mu.RUnlock()

	var out []NotebookEntry
	for _, e := range s.entries {
		ek := normalizeNotebookKind(e.Kind)
		if k != "" && k != "all" && ek != k {
			continue
		}
		if q != "" && !notebookEntryMatches(e, q) {
			continue
		}
		out = append(out, e)
	}
	return out
}

func notebookEntryMatches(e NotebookEntry, q string) bool {
	if strings.Contains(strings.ToLower(e.Source), q) ||
		strings.Contains(strings.ToLower(e.Translation), q) {
		return true
	}
	if e.Grammar != nil {
		if strings.Contains(strings.ToLower(e.Grammar.BackboneSummary), q) ||
			strings.Contains(strings.ToLower(e.Grammar.Translation), q) {
			return true
		}
	}
	if e.Writing != nil {
		if strings.Contains(strings.ToLower(e.Writing.Setting), q) ||
			strings.Contains(strings.ToLower(e.Writing.Improved), q) ||
			strings.Contains(strings.ToLower(e.Writing.Summary), q) {
			return true
		}
	}
	return false
}

func migrateNotebookFromVocab(nb *NotebookStore, vocab *VocabularyStore) int {
	if nb == nil || vocab == nil {
		return 0
	}
	all := vocab.List()
	var pruneIDs []string
	var imported []NotebookEntry
	for _, v := range all {
		switch v.Source {
		case "grammar":
			src := strings.TrimSpace(v.Context)
			if src == "" {
				src = strings.TrimSpace(v.Term)
			}
			imported = append(imported, NotebookEntry{
				ID:          "mig_" + v.ID,
				Kind:        "grammar",
				Source:      src,
				Translation: v.ContextTranslation,
				CreatedAt:   v.CreatedAt,
			})
			pruneIDs = append(pruneIDs, v.ID)
		case "writing":
			src := strings.TrimSpace(v.Context)
			if src == "" {
				src = strings.TrimSpace(v.Term)
			}
			var w *ImproveWritingResult
			if v.Definition != "" || v.ContextTranslation != "" {
				w = &ImproveWritingResult{
					Improved: v.Definition,
					Summary:  v.ContextTranslation,
				}
			}
			imported = append(imported, NotebookEntry{
				ID:          "mig_" + v.ID,
				Kind:        "writing",
				Source:      src,
				Translation: v.ContextTranslation,
				Writing:     w,
				CreatedAt:   v.CreatedAt,
			})
			pruneIDs = append(pruneIDs, v.ID)
		}
	}
	if len(imported) == 0 {
		return 0
	}

	nb.mu.Lock()
	added := 0
	for _, item := range imported {
		if item.Source == "" {
			continue
		}
		dup := false
		for _, existing := range nb.entries {
			if normalizeNotebookKind(existing.Kind) == item.Kind && strings.TrimSpace(existing.Source) == item.Source {
				dup = true
				break
			}
		}
		if !dup {
			nb.entries = append(nb.entries, item)
			added++
		}
	}
	nb.mu.Unlock()

	if added > 0 {
		if err := nb.saveOrLog(); err != nil {
			return 0
		}
	}
	vocab.RemoveIDs(pruneIDs)
	return added
}
