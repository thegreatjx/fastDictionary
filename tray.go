package main

import (
	_ "embed"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

//go:embed resources/tray.png
var embeddedTrayIcon []byte

// trayIcon returns the embedded high-DPI modern tray icon.
func trayIcon() []byte {
	return embeddedTrayIcon
}

// toggleTrayFlyout opens or closes the native Menubar Flyout attached to the tray icon
func (a *app) toggleTrayFlyout() {
	if a.trayFlyout != nil && !a.trayFlyout.IsDestroyed() {
		a.trayFlyout.Close()
		a.trayFlyout = nil
		return
	}
	a.trayFlyout = mygo.NewFlyout(mygo.FlyoutOptions{
		Tray:      a.tray,
		Placement: mygo.PlacementBottom,
		Width:     320,
		Height:    220,
		Focusable: true,
		Shadow:    true,
		Content:   ui.View(safeView("tray", a.trayFlyoutView)),
	})
	a.trayFlyout.OnClosed(func() {
		a.trayFlyout = nil
	})
}

// trayFlyoutView renders the native tray quick-assistant panel
func (a *app) trayFlyoutView(c *ui.Context) {
	cfg := a.store.Get()
	applyTheme(c, cfg.ThemeMode, c.Theme().Dark)
	t := c.Theme()

	c.Root().Background(ui.Transparent)
	box := ui.Column(c).Fill().Background(t.Surface).Radius(10).Border(1, t.Border).Padding(12).Gap(10)
	box.Children(func() {
		// 1. Top bar: [📘 Dictionary] [⚙️ 设置] [✕]
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(6).Children(func() {
			ui.Icon(c, iconDict).Size(16, 16).TextColor(t.Accent)
			ui.Text(c, "fast词典").FontSize(14).FontWeight(700).TextColor(t.Text)
			ui.Spacer(c).Grow(1)
			btnSet := ui.Button(c, "⚙️ 设置").Height(22).Padding(1, 6).Radius(4)
			if btnSet.Clicked() {
				if a.trayFlyout != nil {
					a.trayFlyout.Close()
				}
				a.showSettings()
			}
		})

		// 2. Quick input search
		ui.Box(c).FillWidth().Padding(6, 8).Radius(6).Background(t.SurfaceHover).Border(1, t.Border).Children(func() {
			ui.TextInput(c, &a.trayInput).Placeholder("输入查词 / 翻译，回车即查").AutoFocus().OnSubmit(func() {
				txt := strings.TrimSpace(a.trayInput)
				if txt != "" {
					if a.trayFlyout != nil {
						a.trayFlyout.Close()
					}
					a.openPopupAction(modeTranslate, txt)
				}
			})
		})

		// 3. Selection mode switch segmented
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(6).Children(func() {
			ui.Text(c, "划词:").FontSize(11).TextColor(t.TextMuted)
			modeIdx := 0
			switch cfg.SelectionTrigger {
			case "auto":
				modeIdx = 1
			case "hotkey":
				modeIdx = 2
			}
			ui.Segmented(c, &modeIdx, "显示图标", "直接翻译", "仅快捷键").Height(22).FontSize(11).OnChange(func() {
				modes := []string{"icon", "auto", "hotkey"}
				a.store.Update(func(c *Config) { c.SelectionTrigger = modes[modeIdx] })
				a.applySelectionMode()
				// The tray menu's radio is fixed when built; without this it keeps
				// showing the mode that was active before the flyout changed it.
				a.rebuildTray()
			})
		})

		ui.Divider(c).FillWidth()

		// 4. Quick Links row: [🔖 生词本 (X条)] [退出应用]
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(6).Children(func() {
			vocabCount := GetVocabularyStore().Count()
			btnVocab := ui.Button(c, "").Height(24).Padding(1, 8).Radius(4)
			btnVocab.Children(func() {
				ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
					ui.Icon(c, iconBookmark).Size(12, 12).TextColor(t.Accent)
					ui.Textf(c, "生词本 (%d)", vocabCount).FontSize(11).TextColor(t.Text)
				})
			})
			if btnVocab.Clicked() {
				if a.trayFlyout != nil {
					a.trayFlyout.Close()
				}
				a.page = "vocabulary"
				a.showSettings()
			}

			btnSnip := ui.Button(c, "").Height(24).Padding(1, 8).Radius(4).Tooltip("截屏识别并翻译 (" + cfg.HotkeyScreenshot + ")")
			btnSnip.Children(func() {
				ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
					ui.Icon(c, iconScreenshot).Size(12, 12).TextColor(t.Accent)
					ui.Text(c, "截图翻译").FontSize(11).TextColor(t.Text)
				})
			})
			if btnSnip.Clicked() {
				if a.trayFlyout != nil {
					a.trayFlyout.Close()
				}
				a.TriggerScreenshotOCR()
			}

			ui.Spacer(c).Grow(1)

			quitBg := ui.Hex("#FEE2E2")
			quitFg := ui.Hex("#DC2626")
			if t.Dark {
				quitBg = withAlpha(t.Danger, 0.18)
				quitFg = t.Danger
			}
			btnQuit := ui.Button(c, "退出应用").Height(24).Padding(1, 8).Radius(4).
				Background(quitBg).TextColor(quitFg)
			if btnQuit.Clicked() {
				a.quitApp()
			}
		})
	})
}
