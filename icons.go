package main

import (
	"embed"

	"github.com/egoist/mygo/ui"
)

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
	iconCheck       = mustIcon("check")
	iconMicrosoft   = mustIcon("microsoft")
	iconRefresh     = mustIcon("refresh")
	iconExpand      = mustIcon("expand")
	iconLayout      = mustIcon("pilcrow")
	iconScreenshot  = mustIcon("screenshot")
	iconChevronDown = mustIcon("chevrondown")
	iconChevronUp   = mustIcon("chevronup")
	iconInfo        = mustIcon("info")
	iconGear        = mustIcon("gear")
	iconKeyboard    = mustIcon("keyboard")
)
