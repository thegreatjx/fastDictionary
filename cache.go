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

	"github.com/egoist/mygo"
)

const queryCacheLimit = 64

type cachedTranslation struct {
	Target   string      `json:"target"`
	Engine   string      `json:"engine"`
	Detected string      `json:"detected,omitempty"`
	EchoNote string      `json:"echo_note,omitempty"`
	Dict     *YoudaoDict `json:"dict,omitempty"`
	GDict    *DictResult `json:"gdict,omitempty"`
}

type queryCache struct {
	mu           sync.Mutex
	saveMu       sync.Mutex
	wg           sync.WaitGroup
	path         string
	Translations map[string]cachedTranslation `json:"translations"`
	TransOrder   []string                     `json:"trans_order,omitempty"`
	Notes        map[string][]NoteItem        `json:"notes"`
	NotesOrder   []string                     `json:"notes_order,omitempty"`
	AI           map[string]string            `json:"ai"`
	AIOrder      []string                     `json:"ai_order,omitempty"`
}

func defaultQueryCachePath() string {
	if dir, err := mygo.App.Path(mygo.PathCache); err == nil && dir != "" {
		return filepath.Join(dir, "query_cache.json")
	}
	return ""
}

func newQueryCache(path string) *queryCache {
	qc := &queryCache{path: path}
	if path != "" {
		qc.load()
	}
	return qc
}

func (c *queryCache) load() {
	data, err := os.ReadFile(c.path)
	if err != nil {
		return
	}
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	var disk queryCache
	if err := json.Unmarshal(data, &disk); err != nil {
		log.Printf("fastDictionary: cache parse skipped: %v", err)
		return
	}
	c.mu.Lock()
	c.Translations = disk.Translations
	c.TransOrder = disk.TransOrder
	c.Notes = disk.Notes
	c.NotesOrder = disk.NotesOrder
	c.AI = disk.AI
	c.AIOrder = disk.AIOrder
	c.mu.Unlock()
}

func (c *queryCache) persistAsync() {
	if c == nil || c.path == "" {
		return
	}
	path := c.path
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		defer recoverLog("cache.go:persistAsync")
		c.saveMu.Lock()
		defer c.saveMu.Unlock()
		c.mu.Lock()
		data, err := json.Marshal(c)
		c.mu.Unlock()
		if err != nil {
			return
		}
		if err := atomicWriteFile(path, data, atomicWriteOptions{retries: 3}); err != nil {
			log.Printf("fastDictionary: cache persist failed: %v", err)
		}
	}()
}

func (c *queryCache) flush() {
	if c != nil {
		c.wg.Wait()
	}
}

func putBoundedLRU[V any](m *map[string]V, order *[]string, key string, val V, limit int) {
	if *m == nil {
		*m = make(map[string]V, limit)
	}
	if _, ok := (*m)[key]; ok {
		kept := (*order)[:0]
		for _, k := range *order {
			if k != key {
				kept = append(kept, k)
			}
		}
		*order = kept
	}
	(*m)[key] = val
	*order = append(*order, key)
	for len(*order) > limit {
		oldest := (*order)[0]
		*order = (*order)[1:]
		delete(*m, oldest)
	}
}

func normalizeCacheText(text string) string {
	return strings.TrimSpace(text)
}

func translationCacheKey(engineMode, from, to, layoutMode string, keepParagraphs bool, text string) string {
	return fmt.Sprintf("%s\x00%s\x00%s\x00%s\x00%t\x00%s",
		engineMode, from, to, layoutMode, keepParagraphs, normalizeCacheText(text))
}

func notesCacheKey(model, text string) string {
	return model + "\x00" + normalizeCacheText(text)
}

func (c *queryCache) getTranslation(key string) (cachedTranslation, bool) {
	if c == nil {
		return cachedTranslation{}, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.Translations == nil {
		return cachedTranslation{}, false
	}
	v, ok := c.Translations[key]
	if !ok {
		return cachedTranslation{}, false
	}
	return cloneCachedTranslation(v), true
}

func (c *queryCache) putTranslation(key string, val cachedTranslation) {
	if c == nil || (val.Target == "" && val.EchoNote == "" && val.Dict == nil && val.GDict == nil) {
		return
	}
	c.mu.Lock()
	putBoundedLRU(&c.Translations, &c.TransOrder, key, cloneCachedTranslation(val), queryCacheLimit)
	c.mu.Unlock()
	c.persistAsync()
}

func cloneCachedTranslation(v cachedTranslation) cachedTranslation {
	if v.Dict != nil {
		cp := *v.Dict
		cp.Explains = append([]string(nil), v.Dict.Explains...)
		v.Dict = &cp
	}
	if v.GDict != nil {
		cp := *v.GDict
		cp.Entries = append([]DictEntry(nil), v.GDict.Entries...)
		v.GDict = &cp
	}
	return v
}

func (c *queryCache) getNotes(key string) ([]NoteItem, bool) {
	if c == nil {
		return nil, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.Notes == nil {
		return nil, false
	}
	items, ok := c.Notes[key]
	if !ok {
		return nil, false
	}
	out := make([]NoteItem, len(items))
	copy(out, items)
	return out, true
}

func (c *queryCache) putNotes(key string, items []NoteItem) {
	if c == nil || len(items) == 0 {
		return
	}
	cp := make([]NoteItem, len(items))
	copy(cp, items)
	c.mu.Lock()
	putBoundedLRU(&c.Notes, &c.NotesOrder, key, cp, queryCacheLimit)
	c.mu.Unlock()
	c.persistAsync()
}

func (c *queryCache) getAI(key string) string {
	if c == nil {
		return ""
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.AI == nil {
		return ""
	}
	return c.AI[key]
}

func (c *queryCache) putAI(key, raw string) {
	if c == nil || strings.TrimSpace(raw) == "" {
		return
	}
	c.mu.Lock()
	putBoundedLRU(&c.AI, &c.AIOrder, key, raw, queryCacheLimit)
	c.mu.Unlock()
	c.persistAsync()
}

func (c *queryCache) aiCount() int {
	if c == nil {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.AI)
}

func (c *queryCache) clear() {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.Translations = nil
	c.TransOrder = nil
	c.Notes = nil
	c.NotesOrder = nil
	c.AI = nil
	c.AIOrder = nil
	c.mu.Unlock()
	c.persistAsync()
}
