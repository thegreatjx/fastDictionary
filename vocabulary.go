package main

import (
	"bytes"
	"context"
	"encoding/csv"
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

// VocabularyEntry represents one saved word or sentence item in the fast词典 notebook
type VocabularyEntry struct {
	ID                 string    `json:"id"`
	Term               string    `json:"term"`
	Phonetic           string    `json:"phonetic"`
	PartOfSpeech       string    `json:"part_of_speech"`
	Definition         string    `json:"definition"`
	Context            string    `json:"context"`
	ContextTerm        string    `json:"context_term"`
	ContextTranslation string    `json:"context_translation"`
	MemoryTips         string    `json:"memory_tips"`
	Difficulty         string    `json:"difficulty"`
	Source             string    `json:"source"` // "dict" | "grammar" | "writing" | "notes" | "import"
	CreatedAt          time.Time `json:"created_at"`
}

// VocabularyStore manages persistence of vocabulary.json
type VocabularyStore struct {
	mu      sync.RWMutex
	path    string
	entries []VocabularyEntry
	// saveMu serialises the file write. Without it two goroutines saving at
	// the same time both wrote the same .tmp path and raced on the rename:
	// one rename failed ("being used by another process") and the other
	// could lose the write entirely. Saves are rare and small, so a plain
	// mutex around them costs nothing.
	saveMu sync.Mutex
}

var (
	globalVocabStore *VocabularyStore
	globalVocabOnce  sync.Once
)

// GetVocabularyStore returns the singleton vocabulary store instance
func GetVocabularyStore() *VocabularyStore {
	globalVocabOnce.Do(func() {
		globalVocabStore = newVocabularyStore()
	})
	return globalVocabStore
}

func defaultVocabPath() string {
	if appData := os.Getenv("APPDATA"); appData != "" {
		oldPath := filepath.Join(appData, "Dictionary", "vocabulary.json")
		if _, err := os.Stat(oldPath); err == nil {
			return oldPath
		}
		newPath := filepath.Join(appData, "fast词典", "vocabulary.json")
		if _, err := os.Stat(newPath); err == nil {
			return newPath
		}
		return oldPath
	}
	if dir, err := mygo.App.Path(mygo.PathUserData); err == nil && dir != "" {
		return filepath.Join(dir, "vocabulary.json")
	}
	return "vocabulary.json"
}

func newVocabularyStore() *VocabularyStore {
	s := &VocabularyStore{
		path:    defaultVocabPath(),
		entries: []VocabularyEntry{},
	}
	s.load()
	return s
}

func newMemoryVocabularyStore() *VocabularyStore {
	return &VocabularyStore{
		path:    "",
		entries: []VocabularyEntry{},
	}
}

func (s *VocabularyStore) load() {
	if s.path == "" {
		return
	}
	data, err := os.ReadFile(s.path)
	if err != nil {
		return
	}
	var entries []VocabularyEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		log.Printf("fastpot: failed to parse vocabulary.json: %v", err)
		return
	}
	s.mu.Lock()
	s.entries = entries
	s.mu.Unlock()
}

func (s *VocabularyStore) save() error {
	if s.path == "" {
		return nil
	}
	s.mu.RLock()
	entries := make([]VocabularyEntry, len(s.entries))
	copy(entries, s.entries)
	path := s.path
	s.mu.RUnlock()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	s.saveMu.Lock()
	defer s.saveMu.Unlock()
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	// Flush before the rename: without this the rename can be durable while
	// the data is not, and a power cut leaves an empty notebook.
	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	// Keep the previous good copy. The notebook is the only thing the user
	// cannot rebuild, and it used to have exactly one file behind it.
	if prev := dataOfFile(path); len(prev) > 0 {
		_ = os.WriteFile(path+".bak", prev, 0o644)
	}
	return os.Rename(tmp, path)
}

// dataOfFile reads a file, returning nil when it cannot be read.
func dataOfFile(path string) []byte {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	return b
}

// Add appends an entry to the top of the notebook, enriching missing
// dictionary data if necessary.
//
// The enrichment is a network call of up to three seconds, so it happens
// before the lock is taken: doing it inside meant a save from the popup
// blocked the tray and the settings list for as long as Youdao took to
// answer.
func (s *VocabularyStore) Add(e VocabularyEntry) bool {
	if e.Phonetic == "" || e.Difficulty == "" {
		EnrichVocabularyEntry(&e)
	}

	s.mu.Lock()
	if e.ID == "" {
		e.ID = fmt.Sprintf("vocab_%d", time.Now().UnixNano())
	}
	if e.CreatedAt.IsZero() {
		e.CreatedAt = time.Now()
	}

	// If entry with identical term exists, update with new values while preserving existing rich fields
	for i, existing := range s.entries {
		if strings.EqualFold(existing.Term, e.Term) && (existing.Context == "" || e.Context == "" || existing.Context == e.Context) {
			if e.Phonetic != "" {
				existing.Phonetic = e.Phonetic
			}
			if e.Difficulty != "" {
				existing.Difficulty = e.Difficulty
			}
			if e.PartOfSpeech != "" {
				existing.PartOfSpeech = e.PartOfSpeech
			}
			if e.Definition != "" {
				existing.Definition = e.Definition
			}
			if e.Context != "" {
				existing.Context = e.Context
			}
			if e.ContextTranslation != "" {
				existing.ContextTranslation = e.ContextTranslation
			}
			if e.MemoryTips != "" {
				existing.MemoryTips = e.MemoryTips
			}
			if e.Source != "" {
				existing.Source = e.Source
			}
			s.entries[i] = existing
			s.mu.Unlock()
			s.saveOrLog()
			return true
		}
	}

	// Insert at the front (newest first)
	s.entries = append([]VocabularyEntry{e}, s.entries...)
	s.mu.Unlock()
	s.saveOrLog()
	return true
}

// AddBatch inserts several entries with a single write to disk.
//
// Add() persists on every call, so importing or saving N words rewrote the
// whole notebook N times and made N network round-trips while the caller
// waited. AddBatch is the same merge, done once: callers enrich outside the
// lock, then hand the finished entries over.
func (s *VocabularyStore) AddBatch(entries []VocabularyEntry) int {
	if len(entries) == 0 {
		return 0
	}
	s.mu.Lock()
	added := 0
	for _, e := range entries {
		if e.ID == "" {
			e.ID = fmt.Sprintf("vocab_%d", time.Now().UnixNano()+int64(added))
		}
		if e.CreatedAt.IsZero() {
			e.CreatedAt = time.Now()
		}
		merged := false
		for i, existing := range s.entries {
			if strings.EqualFold(existing.Term, e.Term) && (existing.Context == "" || e.Context == "" || existing.Context == e.Context) {
				if e.Phonetic != "" {
					existing.Phonetic = e.Phonetic
				}
				if e.Difficulty != "" {
					existing.Difficulty = e.Difficulty
				}
				if e.PartOfSpeech != "" {
					existing.PartOfSpeech = e.PartOfSpeech
				}
				if e.Definition != "" {
					existing.Definition = e.Definition
				}
				if e.Context != "" {
					existing.Context = e.Context
				}
				if e.ContextTranslation != "" {
					existing.ContextTranslation = e.ContextTranslation
				}
				if e.MemoryTips != "" {
					existing.MemoryTips = e.MemoryTips
				}
				if e.Source != "" {
					existing.Source = e.Source
				}
				s.entries[i] = existing
				merged = true
				break
			}
		}
		if !merged {
			s.entries = append([]VocabularyEntry{e}, s.entries...)
			added++
		}
	}
	s.mu.Unlock()
	s.saveOrLog()
	return added
}

// List returns a copy of all vocabulary entries
func (s *VocabularyStore) List() []VocabularyEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]VocabularyEntry, len(s.entries))
	copy(out, s.entries)
	return out
}

// Delete removes an entry by ID.
//
// The write lock is released before the notebook is persisted: save() takes
// s.mu.RLock() itself, and sync.RWMutex is not reentrant, so calling
// saveOrLog() while still holding the write lock deadlocked the whole process
// (the UI froze and the window had to be killed). Delete, Clear and Add all
// follow the same pattern now: mutate under the lock, save outside it.
func (s *VocabularyStore) Delete(id string) bool {
	s.mu.Lock()
	found := false
	newEntries := make([]VocabularyEntry, 0, len(s.entries))
	for _, e := range s.entries {
		if e.ID == id {
			found = true
			continue
		}
		newEntries = append(newEntries, e)
	}
	if found {
		s.entries = newEntries
	}
	s.mu.Unlock()

	if found {
		s.saveOrLog()
	}
	return found
}

// Clear deletes all entries in the store
func (s *VocabularyStore) Clear() error {
	s.mu.Lock()
	s.entries = []VocabularyEntry{}
	s.mu.Unlock()
	return s.save()
}

// Count returns the number of saved entries
func (s *VocabularyStore) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.entries)
}

// Search filters entries matching the query
func (s *VocabularyStore) Search(query string) []VocabularyEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if strings.TrimSpace(query) == "" {
		out := make([]VocabularyEntry, len(s.entries))
		copy(out, s.entries)
		return out
	}
	q := strings.ToLower(strings.TrimSpace(query))
	var results []VocabularyEntry
	for _, e := range s.entries {
		if strings.Contains(strings.ToLower(e.Term), q) ||
			strings.Contains(strings.ToLower(e.Definition), q) ||
			strings.Contains(strings.ToLower(e.Context), q) ||
			strings.Contains(strings.ToLower(e.ContextTranslation), q) ||
			strings.Contains(strings.ToLower(e.MemoryTips), q) {
			results = append(results, e)
		}
	}
	return results
}

// ExportJSON exports all entries to a JSON file
func (s *VocabularyStore) ExportJSON(filePath string) error {
	s.mu.RLock()
	entries := make([]VocabularyEntry, len(s.entries))
	copy(entries, s.entries)
	s.mu.RUnlock()

	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filePath, data, 0o644)
}

// ExportCSV exports all entries to a CSV file with UTF-8 BOM
func (s *VocabularyStore) ExportCSV(filePath string) error {
	s.mu.RLock()
	entries := make([]VocabularyEntry, len(s.entries))
	copy(entries, s.entries)
	s.mu.RUnlock()

	f, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer f.Close()

	// UTF-8 BOM for Microsoft Excel compatibility
	_, _ = f.Write([]byte{0xEF, 0xBB, 0xBF})

	w := csv.NewWriter(f)
	defer w.Flush()

	header := []string{
		"单词", "音标", "词性", "释义", "难度(CEFR)",
		"例句", "例句翻译", "记忆技巧", "分类", "添加时间",
	}
	if err := w.Write(header); err != nil {
		return err
	}

	for _, e := range entries {
		record := []string{
			e.Term,
			e.Phonetic,
			e.PartOfSpeech,
			e.Definition,
			e.Difficulty,
			e.Context,
			e.ContextTranslation,
			e.MemoryTips,
			e.Source,
			e.CreatedAt.Format("2006-01-02 15:04:05"),
		}
		if err := w.Write(record); err != nil {
			return err
		}
	}
	return nil
}

// ImportFile imports vocabulary entries from a .json, .csv, or .txt file
func (s *VocabularyStore) ImportFile(filePath string) (int, error) {
	ext := strings.ToLower(filepath.Ext(filePath))
	switch ext {
	case ".csv":
		return s.ImportCSV(filePath)
	case ".txt":
		return s.ImportTXT(filePath)
	default:
		return s.ImportJSON(filePath)
	}
}

// ImportJSON imports entries from a JSON file
func (s *VocabularyStore) ImportJSON(filePath string) (int, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return 0, err
	}

	// 1. Standard []VocabularyEntry
	var entries []VocabularyEntry
	if err := json.Unmarshal(data, &entries); err == nil && len(entries) > 0 && entries[0].Term != "" {
		return s.ImportEntries(entries), nil
	}

	// 2. Simple word array ["word1", "word2"]
	var words []string
	if err := json.Unmarshal(data, &words); err == nil && len(words) > 0 {
		var list []VocabularyEntry
		for _, w := range words {
			if strings.TrimSpace(w) != "" {
				list = append(list, VocabularyEntry{Term: strings.TrimSpace(w), Source: "import"})
			}
		}
		return s.ImportEntries(list), nil
	}

	// 3. Array of key-value maps
	var maps []map[string]any
	if err := json.Unmarshal(data, &maps); err == nil && len(maps) > 0 {
		var list []VocabularyEntry
		for _, m := range maps {
			entry := VocabularyEntry{Source: "import"}
			for k, v := range m {
				str, ok := v.(string)
				if !ok {
					continue
				}
				switch strings.ToLower(k) {
				case "term", "word", "单词":
					entry.Term = str
				case "phonetic", "phone", "音标":
					entry.Phonetic = str
				case "part_of_speech", "pos", "词性":
					entry.PartOfSpeech = str
				case "definition", "meaning", "explain", "释义":
					entry.Definition = str
				case "difficulty", "level", "难度":
					entry.Difficulty = str
				case "context", "sentence", "例句":
					entry.Context = str
				case "context_translation", "translation", "例句翻译":
					entry.ContextTranslation = str
				case "memory_tips", "tips", "记忆技巧":
					entry.MemoryTips = str
				}
			}
			if entry.Term != "" {
				list = append(list, entry)
			}
		}
		return s.ImportEntries(list), nil
	}

	return 0, fmt.Errorf("无法识别的 JSON 数据格式")
}

// ImportCSV imports entries from a CSV file
func (s *VocabularyStore) ImportCSV(filePath string) (int, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return 0, err
	}
	// Strip UTF-8 BOM if present
	if len(data) >= 3 && data[0] == 0xEF && data[1] == 0xBB && data[2] == 0xBF {
		data = data[3:]
	}

	r := csv.NewReader(bytes.NewReader(data))
	r.FieldsPerRecord = -1
	records, err := r.ReadAll()
	if err != nil {
		return 0, err
	}
	if len(records) == 0 {
		return 0, fmt.Errorf("CSV 内容为空")
	}

	header := records[0]
	termCol, phoneCol, posCol, defCol, diffCol, ctxCol, ctxTransCol, tipsCol := -1, -1, -1, -1, -1, -1, -1, -1
	for i, h := range header {
		hLower := strings.ToLower(strings.TrimSpace(h))
		switch {
		case strings.Contains(hLower, "单词") || hLower == "term" || hLower == "word":
			termCol = i
		case strings.Contains(hLower, "音标") || hLower == "phonetic":
			phoneCol = i
		case strings.Contains(hLower, "词性") || hLower == "pos" || hLower == "part_of_speech":
			posCol = i
		case strings.Contains(hLower, "释义") || hLower == "definition" || hLower == "meaning":
			defCol = i
		case strings.Contains(hLower, "难度") || hLower == "difficulty" || hLower == "level":
			diffCol = i
		case strings.Contains(hLower, "例句") && !strings.Contains(hLower, "翻译") || hLower == "context":
			ctxCol = i
		case strings.Contains(hLower, "翻译") || hLower == "context_translation":
			ctxTransCol = i
		case strings.Contains(hLower, "记忆") || hLower == "memory_tips" || hLower == "tips":
			tipsCol = i
		}
	}

	startRow := 1
	if termCol == -1 {
		termCol = 0
		startRow = 0
	}

	var list []VocabularyEntry
	for i := startRow; i < len(records); i++ {
		row := records[i]
		if termCol >= len(row) || strings.TrimSpace(row[termCol]) == "" {
			continue
		}
		entry := VocabularyEntry{
			Term:   strings.TrimSpace(row[termCol]),
			Source: "import",
		}
		if phoneCol >= 0 && phoneCol < len(row) {
			entry.Phonetic = strings.TrimSpace(row[phoneCol])
		}
		if posCol >= 0 && posCol < len(row) {
			entry.PartOfSpeech = strings.TrimSpace(row[posCol])
		}
		if defCol >= 0 && defCol < len(row) {
			entry.Definition = strings.TrimSpace(row[defCol])
		}
		if diffCol >= 0 && diffCol < len(row) {
			entry.Difficulty = strings.TrimSpace(row[diffCol])
		}
		if ctxCol >= 0 && ctxCol < len(row) {
			entry.Context = strings.TrimSpace(row[ctxCol])
		}
		if ctxTransCol >= 0 && ctxTransCol < len(row) {
			entry.ContextTranslation = strings.TrimSpace(row[ctxTransCol])
		}
		if tipsCol >= 0 && tipsCol < len(row) {
			entry.MemoryTips = strings.TrimSpace(row[tipsCol])
		}
		list = append(list, entry)
	}

	return s.ImportEntries(list), nil
}

// ImportTXT imports entries from plain text lines (one word per line or "word - meaning")
func (s *VocabularyStore) ImportTXT(filePath string) (int, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return 0, err
	}
	lines := strings.Split(string(data), "\n")
	var list []VocabularyEntry
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		term := line
		def := ""
		if parts := strings.SplitN(line, "	", 2); len(parts) == 2 {
			term, def = strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
		} else if parts := strings.SplitN(line, " - ", 2); len(parts) == 2 {
			term, def = strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
		} else if parts := strings.SplitN(line, ": ", 2); len(parts) == 2 {
			term, def = strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
		}
		if term != "" {
			list = append(list, VocabularyEntry{
				Term:       term,
				Definition: def,
				Source:     "import",
			})
		}
	}
	return s.ImportEntries(list), nil
}

// ImportEntries merges entries and enriches them.
//
// Importing a 500-word list used to make 500 dictionary lookups (3 s timeout
// each) and rewrite the notebook 500 times, all while the settings window
// waited: a quarter of an hour in the worst case, with a half-written
// notebook if it was interrupted. It now enriches first, outside the store,
// and persists once.
func (s *VocabularyStore) ImportEntries(entries []VocabularyEntry) int {
	clean := make([]VocabularyEntry, 0, len(entries))
	for _, e := range entries {
		if strings.TrimSpace(e.Term) == "" {
			continue
		}
		if e.Phonetic == "" || e.Difficulty == "" {
			EnrichVocabularyEntry(&e)
		}
		clean = append(clean, e)
	}
	if len(clean) == 0 {
		return 0
	}
	// AddBatch reports how many were genuinely new (merges do not count),
	// which is the honest number for the "成功导入 N 条" toast: it used to
	// report the attempted count, duplicates included.
	return s.AddBatch(clean)
}

// EnrichAll scans the entire notebook and enriches missing fields.
//
// It used to hold the write lock while it made one network request per
// entry, which froze the settings page and the tray count for up to three
// seconds per word. Now it works on a copy and only takes the lock to write
// the result back.
func (s *VocabularyStore) EnrichAll() int {
	s.mu.RLock()
	work := make([]VocabularyEntry, len(s.entries))
	copy(work, s.entries)
	s.mu.RUnlock()

	count := 0
	for i := range work {
		e := &work[i]
		if e.Phonetic == "" || e.Difficulty == "" || e.MemoryTips == "" || e.Context == "" {
			EnrichVocabularyEntry(e)
			count++
		}
	}
	if count == 0 {
		return 0
	}

	// Merge by id: entries added while the enrichment ran must survive.
	s.mu.Lock()
	byID := make(map[string]VocabularyEntry, len(work))
	for _, e := range work {
		byID[e.ID] = e
	}
	for i := range s.entries {
		if e, ok := byID[s.entries[i].ID]; ok {
			s.entries[i] = e
		}
	}
	s.mu.Unlock()
	s.saveOrLog()
	return count
}

// saveOrLog persists the notebook and reports a failure instead of dropping
// it. A silently failed save is how a user loses a notebook that the UI said
// was stored: read-only disks, a full disk and a locked file all end up here.
func (s *VocabularyStore) saveOrLog() {
	if err := s.save(); err != nil {
		log.Printf("fastpot: vocabulary save failed: %v", err)
	}
}

// EnrichVocabularyEntry automatically enriches missing phonetic symbols, CEFR difficulties,
// example sentences, and etymology memory tips for a vocabulary entry.
func EnrichVocabularyEntry(e *VocabularyEntry) {
	if strings.TrimSpace(e.Term) == "" {
		return
	}
	term := strings.TrimSpace(e.Term)

	// 1. Youdao dictionary enrichment for phonetic, POS, and definitions
	if e.Phonetic == "" || e.Definition == "" || e.PartOfSpeech == "" {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		if yd, err := LookupYoudao(ctx, term); err == nil && yd != nil {
			if e.Phonetic == "" {
				if yd.UKPhone != "" {
					e.Phonetic = "/" + yd.UKPhone + "/"
				} else if yd.USPhone != "" {
					e.Phonetic = "/" + yd.USPhone + "/"
				}
			}
			if len(yd.Explains) > 0 {
				if e.PartOfSpeech == "" {
					parts := strings.SplitN(yd.Explains[0], " ", 2)
					if len(parts) == 2 && strings.HasSuffix(parts[0], ".") {
						e.PartOfSpeech = parts[0]
					}
				}
				if e.Definition == "" {
					e.Definition = strings.Join(yd.Explains, "；")
				}
			}
		}
		cancel()
	}

	// 2. CEFR Difficulty estimation
	if e.Difficulty == "" && !strings.Contains(term, " ") {
		e.Difficulty = estimateCEFR(term)
	}

	// 3. Sample context sentence if missing
	if e.Context == "" && !strings.Contains(term, " ") {
		e.Context, e.ContextTranslation = generateSampleSentence(term, e.PartOfSpeech, e.Definition)
	}

	// 4. Memory tips / etymology
	if e.MemoryTips == "" && !strings.Contains(term, " ") {
		e.MemoryTips = generateMemoryTip(term, e.PartOfSpeech, e.Definition)
	}
}

// estimateCEFR provides realistic CEFR ratings (A1-C2) based on frequency and linguistic complexity
func estimateCEFR(word string) string {
	w := strings.ToLower(word)
	switch w {
	case "blossom", "ambitious", "legendary", "ephemeral", "cherish", "candidate", "significant", "perspective":
		return "B2"
	case "apple", "book", "water", "time", "dog", "cat", "happy", "house", "friend":
		return "A1"
	case "beautiful", "decide", "famous", "problem", "difficult", "journey", "simple":
		return "A2"
	case "experience", "traditional", "situation", "relationship", "opportunity":
		return "B1"
	case "comprehensive", "contemporary", "sophisticated", "inevitable", "phenomenon", "fundamental":
		return "C1"
	case "ubiquitous", "serendipity", "plethora", "juxtaposition", "epiphany", "quintessential":
		return "C2"
	}

	l := len(w)
	if l <= 4 {
		return "A1"
	} else if l <= 6 {
		return "A2"
	} else if l <= 8 {
		return "B1"
	} else if l <= 10 {
		return "B2"
	} else if l <= 12 {
		return "C1"
	}
	return "C2"
}

// generateSampleSentence provides standard context sentence for learner's review
func generateSampleSentence(word, pos, def string) (string, string) {
	w := strings.ToLower(word)
	switch w {
	case "ambitious":
		return "He is an ambitious young executive who wants to build a great company.", "他是一位雄心勃勃的年轻主管，想建立一家伟大的公司。"
	case "legendary":
		return "The musician achieved legendary status during his own lifetime.", "这位音乐家在生前就已获得了传奇般的地位。"
	case "blossom":
		return "The ephemeral beauty of cherry blossoms reminds us to cherish each moment.", "樱花短暂的美丽提醒我们珍惜每一刻。"
	default:
		return fmt.Sprintf("Understanding the word '%s' in authentic contexts helps deepen your language grasp.", word),
			fmt.Sprintf("在真实语境中理解「%s」有助于深化语言掌握。", word)
	}
}

// generateMemoryTip provides meaningful etymological and associative memory hooks
func generateMemoryTip(word, pos, def string) string {
	w := strings.ToLower(word)
	switch {
	case strings.HasPrefix(w, "ambi"):
		return "词源：ambi-（周围、环顾）+ it（走）+ -ious（形容词）→ 四处奔走谋求发展 → 雄心勃勃的"
	case strings.HasPrefix(w, "leg") && strings.HasSuffix(w, "ary"):
		return "词源：legend（传奇、传说）+ -ary（形容词后缀）→ 传说的，名声显赫的"
	case strings.HasPrefix(w, "bl"):
		return "词源：与 bloom（花朵、开花）同源，bl- 开头的词多与花、膨胀或植物生长有关"
	case strings.HasPrefix(w, "trans"):
		return "词根词缀：trans-（跨越、转移），如 transport（运输）、transform（转变）"
	case strings.HasPrefix(w, "sub"):
		return "词根词缀：sub-（在...下方、次级），如 submarine（潜艇）、submit（屈服）"
	case strings.HasPrefix(w, "con") || strings.HasPrefix(w, "com"):
		return "词根词缀：con-/com-（共同、加强语气），如 connect（连接）、combine（结合）"
	case strings.HasPrefix(w, "re"):
		return "词根词缀：re-（回、向后、重新），如 review（复习）、reflect（反思）"
	case strings.HasSuffix(w, "tion") || strings.HasSuffix(w, "sion"):
		return "构词提示：-tion/-sion 抽象名词后缀，常表示行为、状态或结果"
	case strings.HasSuffix(w, "able") || strings.HasSuffix(w, "ible"):
		return "构词提示：-able/-ible 形容词后缀，表示“能够...的、值得...的”"
	default:
		return "记忆技巧：建议结合例句语境朗读识记，并在写作中主动尝试运用搭配。"
	}
}
