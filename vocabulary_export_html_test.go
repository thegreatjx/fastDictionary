package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// exportHTMLPage writes one entry and returns the exported page text.
func exportHTMLPage(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	s := &VocabularyStore{
		path:    filepath.Join(dir, "vocabulary.json"),
		entries: []VocabularyEntry{{ID: "1", Term: "lucid", Definition: "清晰的", Source: "dict"}},
	}
	path, err := s.ExportHTML()
	if err != nil {
		t.Fatalf("ExportHTML: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read exported html: %v", err)
	}
	return string(data)
}

// TestExportHTMLIsExplicitlyASnapshot 钉住「导出的是快照」这件事写在页面上。
//
// 以前页面没有任何时间或来源说明，用户导出一份 HTML 后回到桌面端加词，
// 再打开网页版会以为数据丢了。
func TestExportHTMLIsExplicitlyASnapshot(t *testing.T) {
	page := exportHTMLPage(t)
	if !strings.Contains(page, "导出的静态快照") {
		t.Fatalf("exported page does not say it is a snapshot")
	}
	if !regexp.MustCompile(`本页是 \d{4}-\d{2}-\d{2} \d{2}:\d{2} 导出的静态快照`).MatchString(page) {
		t.Fatalf("exported page has no export timestamp")
	}
}

// TestExportHTMLImportIsDisabledNotFake 钉住假导入按钮已经拿掉。
//
// 以前是一个真的文件选择框，选完文件只弹一句「请用桌面端」，用户会以为
// 自己操作错了。现在直接显示为不可用，并说明原因。
func TestExportHTMLImportIsDisabledNotFake(t *testing.T) {
	page := exportHTMLPage(t)
	if strings.Contains(page, `type="file"`) || strings.Contains(page, "handleImport") {
		t.Fatalf("exported page still ships the fake import control")
	}
	if !strings.Contains(page, "导入仅桌面端") || !strings.Contains(page, "disabled") {
		t.Fatalf("exported page does not show a disabled import button")
	}
	if !strings.Contains(page, "网页版是离线静态快照") {
		t.Fatalf("disabled import button does not explain why")
	}
}

// TestExportHTMLUsesCategoryConstants 钉住分类值不再散落成字符串字面量。
func TestExportHTMLUsesCategoryConstants(t *testing.T) {
	page := exportHTMLPage(t)
	if !strings.Contains(page, "const CAT=Object.freeze(") {
		t.Fatalf("exported page has no frozen category constants")
	}
	if !strings.Contains(page, "let curCat=CAT.ALL;") {
		t.Fatalf("initial category is not read from CAT")
	}
	for _, want := range []string{"filterCat(CAT.ALL,this)", "filterCat(CAT.WORDS,this)",
		"filterCat(CAT.SENTENCES,this)", "filterCat(CAT.WRITING,this)"} {
		if !strings.Contains(page, want) {
			t.Errorf("exported page missing %q", want)
		}
	}
	if strings.Contains(page, "filterCat('all',this)") {
		t.Errorf("exported page still passes raw category strings")
	}
}
