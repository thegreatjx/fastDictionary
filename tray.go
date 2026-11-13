package main

import (
	_ "embed"
	"fmt"
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
		Width:     TrayWidth,
		Height:    TrayHeight,
		Focusable: true,
		Shadow:    true,
		Content:   ui.View(safeView("tray", a.trayFlyoutView)),
	})
	a.trayFlyout.OnClosed(func() {
		a.trayFlyout = nil
	})
}

// trayStatusText 只描述当前配置，不冒充运行时健康检查。
func trayStatusText(cfg Config) string {
	switch cfg.SelectionTrigger {
	case "auto":
		return "划词：直接翻译"
	case "hotkey":
		return "划词：仅快捷键"
	default:
		return "划词：显示图标"
	}
}

// trayFlyoutView 是单层 Surface 面板：顶栏 / 搜索 / 两列快捷按钮 / 状态与退出底栏。
// 搜索框不再套第二层装饰盒，退出按钮常态无底无框、不亮红。
func (a *app) trayFlyoutView(c *ui.Context) {
	cfg := a.store.Get()
	applyTheme(c, cfg.ThemeMode, c.Theme().Dark)
	t := c.Theme()
	at := appTheme(t)

	c.Root().Background(ui.Transparent)
	panel := ui.Column(c).Fill().Background(at.Surface).Radius(RadiusWindow).
		Border(1, at.BorderWindow).Padding(Space3).Gap(Space3).
		Shadow(0, 10, 28, 0, at.ShadowColor)
	if at.Dark {
		panel.InsetShadow(0, 0, 1, 0, at.InsetColor)
	}
	panel.Children(func() {
		// 1. 顶栏：单色 Logo + 应用名 + Ghost 齿轮。
		ui.Row(c).FillWidth().Height(HeightIconMd).AlignItems(ui.Center).Gap(Space2).Children(func() {
			ui.Icon(c, iconDict).Size(IconSize, IconSize).TextColor(at.Accent)
			ui.Text(c, "fast词典").Font(fontUI).FontSize(FontControl).
				FontWeight(600).TextColor(at.Text).Grow(1)
			if IconButton(c, t, iconGear, "设置").Clicked() {
				if a.trayFlyout != nil {
					a.trayFlyout.Close()
				}
				a.showSettings()
			}
		})

		// 2. 搜索框：TextInputBase 无外观，图标与键帽都在同一行内，只有一层容器。
		search := ui.Box(c).FillWidth().Height(HeightSearch).Radius(RadiusSM).
			Background(at.Surface).Border(1, at.BorderCtrl).Padding(0, Space3)
		search.Children(func() {
			ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(Space2).Children(func() {
				ui.Icon(c, iconSearch).Size(IconSize, IconSize).TextColor(at.TextMuted)
				input := ui.TextInputBase(c, &a.trayInput).Grow(1).
					Font(fontUI).FontSize(FontControl).TextColor(at.Text).
					Placeholder("输入查词 / 翻译，回车即查").AutoFocus().
					FocusRingColor(at.AccentRing).OnSubmit(func() {
					txt := strings.TrimSpace(a.trayInput)
					if txt != "" {
						if a.trayFlyout != nil {
							a.trayFlyout.Close()
						}
						a.openPopupAction(modeTranslate, txt)
					}
				})
				if input.Focused() {
					search.Border(1, at.AccentRing)
				}
				if strings.TrimSpace(a.trayInput) != "" {
					clear := IconButton(c, t, iconClose, "清空").
						Height(HeightIconSm).Width(HeightIconSm)
					if clear.Clicked() {
						a.trayInput = ""
					}
				} else {
					KeycapBadge(c, t, "Enter")
				}
			})
		})

		// 3. 快捷功能：两列等宽、等高 Secondary。
		ui.Row(c).FillWidth().Gap(Space2).Children(func() {
			vocab := SecondaryButton(c, t,
				fmt.Sprintf("生词本 (%d)", GetVocabularyStore().Count()), iconBookmark).Grow(1)
			if vocab.Clicked() {
				if a.trayFlyout != nil {
					a.trayFlyout.Close()
				}
				a.page = "vocabulary"
				a.showSettings()
			}

			snip := SecondaryButton(c, t, "截图翻译", iconScreenshot).Grow(1).
				Tooltip("截屏识别并翻译 (" + cfg.HotkeyScreenshot + ")")
			if snip.Clicked() {
				if a.trayFlyout != nil {
					a.trayFlyout.Close()
				}
				a.TriggerScreenshotOCR()
			}
		})

		// 划词模式切换保留原功能，用同一套自适应 Segmented。
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(Space2).Children(func() {
			ui.Text(c, "划词").Font(fontUI).FontSize(FontHelper).
				TextColor(at.TextMuted).SingleLine()
			modeIdx := 0
			switch cfg.SelectionTrigger {
			case "auto":
				modeIdx = 1
			case "hotkey":
				modeIdx = 2
			}
			SegmentedControl(c, &modeIdx, "显示图标", "直接翻译", "仅快捷键").OnChange(func() {
				modes := []string{"icon", "auto", "hotkey"}
				a.store.Update(func(c *Config) { c.SelectionTrigger = modes[modeIdx] })
				a.applySelectionMode()
				// The tray menu's radio is fixed when built; without this it keeps
				// showing the mode that was active before the flyout changed it.
				a.rebuildTray()
			})
		})

		// 4. 底栏：1px 分割线 + 状态 + Danger-Ghost 退出。
		ui.Box(c).FillWidth().BorderWidth(1, 0, 0, 0).BorderColor(at.BorderSubtle).
			Padding(Space2, 0, 0, 0).Children(func() {
			ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(Space2).Children(func() {
				ui.Text(c, "●").FontSize(FontMicro).TextColor(at.Success).SingleLine()
				ui.Text(c, trayStatusText(cfg)).Font(fontUI).FontSize(FontHelper).
					TextColor(at.TextMuted).Grow(1).SingleLine()
				if DangerGhostButton(c, t, "退出应用").Clicked() {
					a.quitApp()
				}
			})
		})
	})
}
