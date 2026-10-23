package main

import (
	"encoding/json"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"strings"
)

// CountByCategory returns the number of entries in a specific category
func (s *VocabularyStore) CountByCategory(category string) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if category == "" || category == "all" {
		return len(s.entries)
	}
	count := 0
	for _, e := range s.entries {
		if s.matchesCategory(e, category) {
			count++
		}
	}
	return count
}

// CategoryCounts counts every tab in one pass. The settings page rendered
// four separate counters per frame, each a full scan under the read lock:
// with a thousand entries that is four thousand matches per keystroke in
// the search box.
func (s *VocabularyStore) CategoryCounts() (all, words, sentences, writing int) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	all = len(s.entries)
	for _, e := range s.entries {
		// matchesCategory's own rules overlap (a writing entry with a
		// two-word term also matches "words"), so the writing check comes
		// first: Source is authoritative, the field-count heuristic is not.
		switch {
		case e.Source == "writing":
			writing++
		case s.matchesCategory(e, "words"):
			words++
		case s.matchesCategory(e, "sentences"):
			sentences++
		}
	}
	return
}

func (s *VocabularyStore) matchesCategory(e VocabularyEntry, category string) bool {
	switch category {
	case "words":
		return e.Source == "dict" || e.Source == "notes" || len(strings.Fields(e.Term)) <= 2
	case "sentences":
		return e.Source == "grammar" || (e.Source != "writing" && len(strings.Fields(e.Term)) > 2)
	case "writing":
		return e.Source == "writing"
	default:
		return true
	}
}

// Filter filters entries by category and keyword query
func (s *VocabularyStore) Filter(category string, query string) []VocabularyEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	q := strings.ToLower(strings.TrimSpace(query))
	var results []VocabularyEntry
	for _, e := range s.entries {
		if !s.matchesCategory(e, category) {
			continue
		}
		if q != "" {
			if !strings.Contains(strings.ToLower(e.Term), q) &&
				!strings.Contains(strings.ToLower(e.Definition), q) &&
				!strings.Contains(strings.ToLower(e.Context), q) &&
				!strings.Contains(strings.ToLower(e.ContextTranslation), q) &&
				!strings.Contains(strings.ToLower(e.MemoryTips), q) {
				continue
			}
		}
		results = append(results, e)
	}
	return results
}

// ExportHTML exports all vocabulary entries into an interactive, standalone HTML notebase page
func (s *VocabularyStore) ExportHTML() (string, error) {
	s.mu.RLock()
	entries := make([]VocabularyEntry, len(s.entries))
	copy(entries, s.entries)
	baseDir := filepath.Dir(s.path)
	s.mu.RUnlock()

	// An in-memory store (tests) has no file behind it: writing next to the
	// user's current working directory is worse than refusing.
	if s.path == "" {
		return "", fmt.Errorf("vocabulary store has no backing file")
	}

	outPath := filepath.Join(baseDir, "notebook.html")

	entriesJSON, _ := json.Marshal(entries)

	var sb strings.Builder
	sb.WriteString("<!DOCTYPE html>\n<html lang=\"zh-CN\">\n<head>\n<meta charset=\"UTF-8\">\n")
	sb.WriteString("<title>fast词典 · 生词本与长难句笔记</title>\n")
	sb.WriteString("<style>\nbody{margin:0;font-family:-apple-system,BlinkMacSystemFont,Segoe UI,Roboto,sans-serif;background:#F8FAFC;color:#0F172A;padding:32px 24px;display:flex;justify-content:center;}\n")
	sb.WriteString(".container{max-width:960px;width:100%;}\n")
	sb.WriteString(".header{display:flex;align-items:center;justify-content:space-between;margin-bottom:20px;}\n")
	sb.WriteString(".action-btn{border:1px solid #CBD5E1;background:#FFF;color:#334155;font-size:12px;font-weight:600;padding:6px 12px;border-radius:6px;cursor:pointer;display:inline-flex;align-items:center;transition:all 0.15s;text-decoration:none;}\n")
	sb.WriteString(".action-btn:hover{background:#F1F5F9;border-color:#94A3B8;color:#0F172A;}\n")
	sb.WriteString(".tabs{display:flex;gap:8px;margin-bottom:20px;background:#FFF;padding:4px;border-radius:8px;border:1px solid #E2E8F0;}\n")
	sb.WriteString(".tab-btn{border:none;background:transparent;color:#64748B;font-size:13px;font-weight:600;padding:6px 14px;border-radius:6px;cursor:pointer;}\n")
	sb.WriteString(".tab-btn.active{background:#2563EB;color:#FFF;}\n")
	sb.WriteString(".search{width:100%;box-sizing:border-box;padding:10px 14px;border-radius:8px;border:1px solid #E2E8F0;font-size:14px;margin-bottom:24px;}\n")
	sb.WriteString(".card{background:#FFF;border:1px solid #E2E8F0;border-radius:10px;padding:18px 20px;margin-bottom:14px;box-shadow:0 1px 3px rgba(0,0,0,0.04);}\n")
	sb.WriteString(".card-term{font-size:18px;font-weight:700;}\n")
	sb.WriteString(".card-phonetic{color:#64748B;font-size:13px;margin-left:8px;}\n")
	sb.WriteString(".card-pos{background:#EFF6FF;color:#2563EB;font-size:11px;font-weight:600;padding:2px 6px;border-radius:4px;margin-left:8px;}\n")
	sb.WriteString(".card-diff{background:#F0FDF4;color:#16A34A;font-size:10px;font-weight:700;padding:2px 7px;border-radius:10px;margin-left:8px;}\n")
	sb.WriteString(".context-box{background:#F8FAFC;border:1px solid #E2E8F0;border-radius:6px;padding:10px 14px;margin-top:10px;font-size:13px;line-height:1.6;}\n")
	sb.WriteString(".tips-box{background:#FEFCE8;border:1px solid #FEF08A;color:#713F12;padding:8px 12px;border-radius:6px;font-size:12px;margin-top:10px;}\n")
	sb.WriteString("</style>\n</head>\n<body>\n<div class=\"container\">\n")
	sb.WriteString("<div class=\"header\"><div style=\"display:flex;align-items:center;gap:12px;\"><span style=\"font-size:26px;\">📘</span><h1 style=\"margin:0;font-size:22px;color:#0F172A;font-weight:700;\">fast词典 生词库</h1></div>")
	sb.WriteString("<div style=\"display:flex;gap:8px;\">")
	sb.WriteString("<button class=\"action-btn\" onclick=\"exportJSON()\">📤 导出 JSON</button>")
	sb.WriteString("<button class=\"action-btn\" onclick=\"exportCSV()\">📊 导出 CSV</button>")
	sb.WriteString("<label class=\"action-btn\" style=\"cursor:pointer;\">📥 导入生词<input type=\"file\" id=\"importInput\" accept=\".json,.csv\" style=\"display:none;\" onchange=\"handleImport(event)\"></label>")
	sb.WriteString("</div></div>\n")
	sb.WriteString("<div class=\"tabs\">")
	sb.WriteString("<button class=\"tab-btn active\" onclick=\"filterCat('all',this)\">全部笔记</button>")
	sb.WriteString("<button class=\"tab-btn\" onclick=\"filterCat('words',this)\">📘 生词库</button>")
	sb.WriteString("<button class=\"tab-btn\" onclick=\"filterCat('sentences',this)\">🔍 长难句本</button>")
	sb.WriteString("<button class=\"tab-btn\" onclick=\"filterCat('writing',this)\">✏️ 写作批改</button>")
	sb.WriteString("</div>\n")
	sb.WriteString("<input type=\"text\" id=\"search\" class=\"search\" placeholder=\"搜索词条、释义或例句...\" oninput=\"doSearch()\">\n")
	sb.WriteString("<div id=\"list\">\n")

	for _, e := range entries {
		cat := "words"
		if e.Source == "grammar" || len(strings.Fields(e.Term)) > 2 {
			cat = "sentences"
		} else if e.Source == "writing" {
			cat = "writing"
		}
		// Every field is user data: a saved sentence containing markup (or
		// the literal </script> the AI sometimes emits) used to break the
		// whole page. Escape for text and attributes alike.
		esc := html.EscapeString
		sb.WriteString(fmt.Sprintf("<div class=\"card\" data-cat=\"%s\" data-text=\"%s %s\">", cat, esc(strings.ToLower(e.Term)), esc(strings.ToLower(e.Definition))))
		sb.WriteString(fmt.Sprintf("<div style=\"display:flex;align-items:center;\"><span class=\"card-term\">%s</span>", esc(e.Term)))
		if e.Phonetic != "" {
			sb.WriteString(fmt.Sprintf("<span class=\"card-phonetic\">%s</span>", esc(e.Phonetic)))
		}
		if e.PartOfSpeech != "" {
			sb.WriteString(fmt.Sprintf("<span class=\"card-pos\">%s</span>", esc(e.PartOfSpeech)))
		}
		if e.Difficulty != "" {
			sb.WriteString(fmt.Sprintf("<span class=\"card-diff\">%s</span>", esc(e.Difficulty)))
		}
		sb.WriteString(fmt.Sprintf("<span style=\"margin-left:auto;color:#94A3B8;font-size:11px;\">%s</span></div>", e.CreatedAt.Format("2006-01-02 15:04")))
		if e.Definition != "" {
			sb.WriteString(fmt.Sprintf("<div style=\"margin-top:8px;font-size:14px;\">%s</div>", esc(e.Definition)))
		}
		if e.Context != "" {
			sb.WriteString(fmt.Sprintf("<div class=\"context-box\"><div>%s</div>", esc(e.Context)))
			if e.ContextTranslation != "" {
				sb.WriteString(fmt.Sprintf("<div style=\"color:#64748B;font-size:12px;margin-top:4px;\">%s</div>", esc(e.ContextTranslation)))
			}
			sb.WriteString("</div>")
		}
		if e.MemoryTips != "" {
			sb.WriteString(fmt.Sprintf("<div class=\"tips-box\">💡 %s</div>", esc(e.MemoryTips)))
		}
		sb.WriteString("</div>\n")
	}

	sb.WriteString("</div></div>\n")
	sb.WriteString("<script>\n")
	// The JSON blob inside <script> needs </ escaped (a term or example
	// containing </script> would otherwise end the script tag and break the
	// whole page); html.EscapeString is not enough inside a script element.
	sb.WriteString(fmt.Sprintf("let allEntries = %s;\n", strings.ReplaceAll(string(entriesJSON), "</", "<\\/")))
	sb.WriteString("let curCat='all';\n")
	sb.WriteString("function filterCat(c,b){curCat=c;document.querySelectorAll('.tab-btn').forEach(x=>x.classList.remove('active'));b.classList.add('active');doSearch();}\n")
	sb.WriteString("function doSearch(){let q=document.getElementById('search').value.toLowerCase().trim();document.querySelectorAll('.card').forEach(x=>{let mC=(curCat==='all'||x.getAttribute('data-cat')===curCat);let mQ=(!q||x.getAttribute('data-text').includes(q));x.style.display=(mC&&mQ)?'block':'none';});}\n")
	sb.WriteString("function exportJSON(){const blob=new Blob([JSON.stringify(allEntries,null,2)],{type:'application/json'});const url=URL.createObjectURL(blob);const a=document.createElement('a');a.href=url;a.download='fast词典_生词库.json';a.click();URL.revokeObjectURL(url);}\n")
	sb.WriteString("function exportCSV(){let csv='\uFEFF单词,音标,词性,释义,难度,例句,例句翻译,记忆技巧,分类,添加时间\r\n';allEntries.forEach(e=>{const esc=s=>String(s||'').replace(/,/g,'，').replace(/\n/g,' ');csv+=[esc(e.term),esc(e.phonetic),esc(e.part_of_speech),esc(e.definition),esc(e.difficulty),esc(e.context),esc(e.context_translation),esc(e.memory_tips),esc(e.source),esc(e.created_at)].join(',')+'\r\n';});const blob=new Blob([csv],{type:'text/csv;charset=utf-8;'});const url=URL.createObjectURL(blob);const a=document.createElement('a');a.href=url;a.download='fast词典_生词库.csv';a.click();URL.revokeObjectURL(url);}\n")
	sb.WriteString("function handleImport(evt){const file=evt.target.files[0];if(!file)return;const reader=new FileReader();reader.onload=e=>{try{const text=e.target.result;if(file.name.endsWith('.json')){const items=JSON.parse(text);items.forEach(it=>{allEntries.unshift(it);});alert('成功导入 '+items.length+' 条生词，请刷新查阅！');window.location.reload();}else{alert('请使用 fast词典 桌面端进行完整 CSV / TXT 智能导入与补全！');}}catch(err){alert('导入解析失败：'+err);}};reader.readAsText(file);}\n")
	sb.WriteString("</script>\n</body></html>")

	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(outPath, []byte(sb.String()), 0o644); err != nil {
		return "", err
	}
	return outPath, nil
}
