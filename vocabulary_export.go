package main

import (
	"encoding/json"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"strings"
	"time"
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
		switch classifyVocabEntry(e) {
		case vocabCategoryWords:
			words++
		case vocabCategorySentences:
			sentences++
		case vocabCategoryWriting:
			writing++
		}
	}
	return
}

func (s *VocabularyStore) matchesCategory(e VocabularyEntry, category string) bool {
	cat, ok := vocabCategoryFromKey(category)
	if !ok {
		return true
	}
	return classifyVocabEntry(e) == cat
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
	sb.WriteString(".action-btn[disabled]{opacity:.55;cursor:not-allowed;pointer-events:none;}\n")
	sb.WriteString(".snapshot-note{background:#FEFCE8;border:1px solid #FEF08A;color:#713F12;font-size:12px;line-height:1.6;padding:8px 12px;border-radius:6px;margin-bottom:16px;}\n")
	sb.WriteString(".tabs{display:flex;gap:8px;margin-bottom:20px;background:#FFF;padding:4px;border-radius:8px;border:1px solid #E2E8F0;}\n")
	sb.WriteString(".tab-btn{border:none;background:transparent;color:#64748B;font-size:13px;font-weight:600;padding:6px 14px;border-radius:6px;cursor:pointer;}\n")
	sb.WriteString(".tab-btn.active{background:#2563EB;color:#FFF;}\n")
	sb.WriteString(".search{width:100%;box-sizing:border-box;padding:10px 14px;border-radius:8px;border:1px solid #E2E8F0;font-size:14px;margin-bottom:24px;}\n")
	sb.WriteString(".card{background:#FFF;border:1px solid #E2E8F0;border-radius:10px;padding:18px 20px;margin-bottom:14px;box-shadow:0 1px 3px rgba(0,0,0,0.04);}\n")
	sb.WriteString(".card-term{font-size:18px;font-weight:700;}\n")
	sb.WriteString(".card-phonetic{color:#64748B;font-size:13px;margin-left:8px;}\n")
	sb.WriteString(".card-pos{background:#EFF6FF;color:#2563EB;font-size:11px;font-weight:600;padding:2px 6px;border-radius:4px;margin-left:8px;}\n")
	sb.WriteString(".card-diff{background:#F0FDF4;color:#16A34A;font-size:10px;font-weight:700;padding:2px 7px;border-radius:10px;margin-left:8px;text-transform:uppercase;}\n")
	sb.WriteString(".card-diff.a1,.card-diff.a2{background:#ECFDF5;color:#059669;}\n")
	sb.WriteString(".card-diff.b1,.card-diff.b2{background:#EFF6FF;color:#2563EB;}\n")
	sb.WriteString(".card-diff.c1,.card-diff.c2{background:#FAF5FF;color:#7E22CE;}\n")
	sb.WriteString(".context-box{background:#F8FAFC;border:1px solid #E2E8F0;border-radius:6px;padding:10px 14px;margin-top:10px;font-size:13px;line-height:1.6;}\n")
	sb.WriteString(".tips-box{background:#FEFCE8;border:1px solid #FEF08A;color:#713F12;padding:8px 12px;border-radius:6px;font-size:12px;margin-top:10px;}\n")
	sb.WriteString(".fd-hl, .hl{color:#2563EB;font-weight:700;background:rgba(37,99,235,0.08);padding:1px 4px;border-radius:3px;}\n")
	sb.WriteString("@media (prefers-color-scheme:dark){body{background:#0F172A;color:#F1F5F9;}.header h1{color:#F1F5F9 !important;}.tabs{background:#1E293B;border-color:#334155;}.tab-btn{color:#94A3B8;}.tab-btn.active{background:#3B82F6;color:#FFF;}.search{background:#1E293B;border-color:#334155;color:#F1F5F9;}.card{background:#1E293B;border-color:#334155;box-shadow:0 1px 3px rgba(0,0,0,0.3);}.card-term{color:#F1F5F9;}.card-phonetic{color:#94A3B8;}.card-pos{background:rgba(59,130,246,0.2);color:#60A5FA;}.context-box{background:#0F172A;border-color:#334155;color:#E2E8F0;}.tips-box{background:rgba(234,179,8,0.12);border-color:rgba(234,179,8,0.3);color:#FDE047;}.action-btn{background:#1E293B;border-color:#334155;color:#E2E8F0;}.action-btn:hover{background:#334155;border-color:#475569;color:#FFF;}.fd-hl, .hl{color:#60A5FA;background:rgba(96,165,250,0.15);}}\n")
	sb.WriteString("</style>\n</head>\n<body>\n<div class=\"container\">\n")
	sb.WriteString("<div class=\"header\"><div style=\"display:flex;align-items:center;gap:12px;\"><span style=\"font-size:26px;\">📘</span><h1 style=\"margin:0;font-size:22px;color:#0F172A;font-weight:700;\">fast词典 生词库</h1></div>")
	sb.WriteString("<div style=\"display:flex;gap:8px;\">")
	sb.WriteString("<button class=\"action-btn\" onclick=\"exportJSON()\">📤 导出 JSON</button>")
	sb.WriteString("<button class=\"action-btn\" onclick=\"exportCSV()\">📊 导出 CSV</button>")
	sb.WriteString("<button class=\"action-btn\" onclick=\"exportAnki()\">📇 导出 Anki</button>")
	sb.WriteString("<button class=\"action-btn\" disabled title=\"网页版是离线静态快照；导入请打开桌面客户端的生词本页面\">📥 导入仅桌面端</button>")
	sb.WriteString("</div></div>\n")
	sb.WriteString(fmt.Sprintf("<div class=\"snapshot-note\">本页是 %s 导出的静态快照；之后在桌面端增删的生词不会同步到这里。</div>\n", time.Now().Format("2006-01-02 15:04")))
	sb.WriteString("<div class=\"tabs\">")
	sb.WriteString("<button class=\"tab-btn active\" onclick=\"filterCat(CAT.ALL,this)\">全部笔记</button>")
	sb.WriteString("<button class=\"tab-btn\" onclick=\"filterCat(CAT.WORDS,this)\">📘 生词库</button>")
	sb.WriteString("<button class=\"tab-btn\" onclick=\"filterCat(CAT.SENTENCES,this)\">🔍 长难句本</button>")
	sb.WriteString("<button class=\"tab-btn\" onclick=\"filterCat(CAT.WRITING,this)\">✏️ 写作批改</button>")
	sb.WriteString("</div>\n")
	sb.WriteString("<input type=\"text\" id=\"search\" class=\"search\" placeholder=\"搜索词条、释义或例句...\" oninput=\"doSearch()\">\n")
	sb.WriteString("<div id=\"list\">\n")

	// allCats mirrors allEntries so the page's own scripts group entries with
	// the same rule as the desktop app instead of re-implementing it in JS.
	allCats := make([]string, len(entries))
	for i, e := range entries {
		cat := classifyVocabEntry(e)
		allCats[i] = cat.String()
		// Every field is user data: a saved sentence containing markup (or
		// the literal </script> the AI sometimes emits) used to break the
		// whole page. Escape for text and attributes alike.
		esc := html.EscapeString
		searchContent := strings.ToLower(strings.Join([]string{e.Term, e.Definition, e.Context, e.ContextTranslation, e.MemoryTips}, " "))
		sb.WriteString(fmt.Sprintf("<div class=\"card\" data-cat=\"%s\" data-text=\"%s\">", cat.String(), esc(searchContent)))
		sb.WriteString(fmt.Sprintf("<div style=\"display:flex;align-items:center;\"><span class=\"card-term\">%s</span>", esc(e.Term)))
		if e.Phonetic != "" {
			sb.WriteString(fmt.Sprintf("<span class=\"card-phonetic\">%s</span>", esc(e.Phonetic)))
		}
		if e.PartOfSpeech != "" {
			sb.WriteString(fmt.Sprintf("<span class=\"card-pos\">%s</span>", esc(e.PartOfSpeech)))
		}
		if e.Difficulty != "" {
			diffLower := strings.ToLower(strings.TrimSpace(e.Difficulty))
			sb.WriteString(fmt.Sprintf("<span class=\"card-diff %s\">%s</span>", diffLower, esc(strings.ToUpper(e.Difficulty))))
		}
		sb.WriteString(fmt.Sprintf("<span style=\"margin-left:auto;color:#94A3B8;font-size:11px;\">%s</span></div>", e.CreatedAt.Format("2006-01-02 15:04")))
		if e.Definition != "" {
			sb.WriteString(fmt.Sprintf("<div style=\"margin-top:8px;font-size:14px;\">%s</div>", esc(e.Definition)))
		}
		if e.Context != "" {
			targetTerm := e.ContextTerm
			if targetTerm == "" {
				targetTerm = e.Term
			}
			sb.WriteString(fmt.Sprintf("<div class=\"context-box\"><div>%s</div>", highlightTermInContext(e.Context, targetTerm)))
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

	sb.WriteString("<div id=\"noResult\" style=\"display:none;text-align:center;padding:48px 16px;color:#94A3B8;font-size:14px;\"><div style=\"font-size:28px;margin-bottom:8px;\">🔍</div><div>未找到匹配的笔记</div><div style=\"font-size:12px;margin-top:4px;\">尝试缩短搜索词或切换上方分类</div></div>\n")
	sb.WriteString("</div></div>\n")
	sb.WriteString("<script>\n")
	// The JSON blob inside <script> needs </ escaped (a term or example
	// containing </script> would otherwise end the script tag and break the
	// whole page); html.EscapeString is not enough inside a script element.
	sb.WriteString(fmt.Sprintf("let allEntries = %s;\n", strings.ReplaceAll(string(entriesJSON), "</", "<\\/")))
	sb.WriteString(fmt.Sprintf("let allCats = %s;\n", strings.Join(allCats, ",")))
	sb.WriteString("const CAT=Object.freeze({ALL:'all',WORDS:'words',SENTENCES:'sentences',WRITING:'writing'});\n")
	sb.WriteString("let curCat=CAT.ALL;\n")
	sb.WriteString("function filterCat(c,b){curCat=c;document.querySelectorAll('.tab-btn').forEach(x=>x.classList.remove('active'));b.classList.add('active');doSearch();}\n")
	sb.WriteString("function doSearch(){let q=document.getElementById('search').value.toLowerCase().trim();let v=0;document.querySelectorAll('.card').forEach(x=>{let mC=(curCat==='all'||x.getAttribute('data-cat')===curCat);let mQ=(!q||x.getAttribute('data-text').includes(q));let show=(mC&&mQ);x.style.display=show?'block':'none';if(show)v++;});let nr=document.getElementById('noResult');if(nr)nr.style.display=(v===0)?'block':'none';}\n")
	sb.WriteString("function exportJSON(){const blob=new Blob([JSON.stringify(allEntries,null,2)],{type:'application/json'});const url=URL.createObjectURL(blob);const a=document.createElement('a');a.href=url;a.download='fast词典_生词库.json';a.click();URL.revokeObjectURL(url);}\n")
	sb.WriteString("function exportCSV(){let csv='\uFEFF单词,音标,词性,释义,难度,例句,例句翻译,记忆技巧,分类,添加时间\r\n';allEntries.forEach(e=>{const esc=s=>String(s||'').replace(/,/g,'，').replace(/\n/g,' ');csv+=[esc(e.term),esc(e.phonetic),esc(e.part_of_speech),esc(e.definition),esc(e.difficulty),esc(e.context),esc(e.context_translation),esc(e.memory_tips),esc(e.source),esc(e.created_at)].join(',')+'\r\n';});const blob=new Blob([csv],{type:'text/csv;charset=utf-8;'});const url=URL.createObjectURL(blob);const a=document.createElement('a');a.href=url;a.download='fast词典_生词库.csv';a.click();URL.revokeObjectURL(url);}\n")
	sb.WriteString("function exportAnki(){let tsv='#separator:tab\\n#html:true\\n#tags column:4\\n';const clean=s=>String(s||'').replace(/\\t/g,' ').replace(/\\r?\\n/g,'<br>');const escH=s=>String(s||'').replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/\"/g,'&quot;');allEntries.forEach((e,i)=>{const isWord=(allCats[i]==='words');let f='',b='',tags='fastDictionary';if(isWord){f='<div style=\"font-size:22px;font-weight:700;\">'+escH(e.term)+'</div>'+(e.phonetic?'<div style=\"color:#64748b;margin-top:4px;\">'+escH(e.phonetic)+'</div>':'');b='<div style=\"font-size:20px;font-weight:700;\">'+escH(e.term)+'</div>'+(e.definition?'<div style=\"margin:8px 0;\">'+escH(e.definition)+'</div>':'')+(e.context?'<div style=\"background:#f8fafc;padding:8px;border-radius:6px;margin-top:8px;\">'+escH(e.context)+(e.context_translation?'<div style=\"color:#64748b;font-size:12px;\">'+escH(e.context_translation)+'</div>':'')+'</div>':'')+(e.memory_tips?'<div style=\"background:#fefce8;padding:6px;margin-top:8px;font-size:12px;\">💡 '+escH(e.memory_tips)+'</div>':'');tags+=' 生词 '+(e.difficulty?'CEFR-'+e.difficulty.toUpperCase():'');}else if(allCats[i]==='writing'){f='<div>✏️ 写作批改</div><div style=\"font-size:16px;margin-top:8px;\">'+escH(e.term)+'</div>';b='<div style=\"color:#15803d;font-weight:600;\">✓ 改后版本：</div><div>'+escH(e.context)+'</div><div style=\"margin-top:6px;\">'+escH(e.definition)+'</div>';tags+=' 写作批改';}else{f='<div>🔍 长难句拆解</div><div style=\"font-size:16px;margin-top:8px;\">'+escH(e.term)+'</div>';b='<div style=\"font-size:16px;font-weight:600;\">'+escH(e.term)+'</div><div style=\"color:#475569;margin:6px 0;\">'+escH(e.context_translation)+'</div><div>'+escH(e.definition)+'</div>';tags+=' 长难句 '+(e.difficulty?'CEFR-'+e.difficulty.toUpperCase():'');}tsv+=[clean(f),clean(b),clean(e.term),clean(tags.trim())].join('\\t')+'\\n';});const blob=new Blob([tsv],{type:'text/tab-separated-values;charset=utf-8;'});const url=URL.createObjectURL(blob);const a=document.createElement('a');a.href=url;a.download='fast词典_生词库_Anki.txt';a.click();URL.revokeObjectURL(url);}\n")
	sb.WriteString("</script>\n</body></html>")

	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(outPath, []byte(sb.String()), 0o644); err != nil {
		return "", err
	}
	return outPath, nil
}
