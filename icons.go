package main

import (
	"embed"

	"github.com/egoist/mygo/ui"
)

// Lucide 图标：统一 24×24 视口、1.5 描边、currentColor 模板着色；
// 表示状态的几个（固定、选中）改用填充，和描边版共享同一条路径。
// 资源以 go:embed 打进二进制，避免每个图标再写一份 Go 字符串常量。
//
//go:embed resources/icons/*.svg
var iconFS embed.FS

func mustIcon(name string) *ui.SVG {
	b, err := iconFS.ReadFile("resources/icons/" + name + ".svg")
	if err != nil {
		panic("missing icon: " + name)
	}
	return ui.MustParseSVG(b)
}

var (
	iconTranslate   = mustIcon("translate")
	iconSpeak       = mustIcon("speak")
	iconDict        = mustIcon("dict")
	iconSearch      = mustIcon("search")
	iconSparkles    = mustIcon("sparkles")
	iconCopy        = mustIcon("copy")
	iconClose       = mustIcon("close")
	iconMore        = mustIcon("more")
	iconSwap        = mustIcon("swap")
	iconPin         = mustIcon("pin")
	iconPinned      = mustIcon("pinned")
	iconGrammar     = mustIcon("grammar")
	iconPolish      = mustIcon("polish")
	iconGripDots    = mustIcon("gripdots")
	iconBookmark    = mustIcon("bookmark")
	iconCheckSquare = mustIcon("checksquare")
	iconMicrosoft   = mustIcon("microsoft")
	iconRefresh     = mustIcon("refresh")
	iconExpand      = mustIcon("expand")
	iconScreenshot  = mustIcon("screenshot")
	iconChevronDown = mustIcon("chevrondown")
	iconChevronUp   = mustIcon("chevronup")
	iconInfo        = mustIcon("info")
	iconGear        = mustIcon("gear")
	iconKeyboard    = mustIcon("keyboard")
)
