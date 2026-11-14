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

// toggleTrayFlyout opens or closes the native Menubar Flyout attached to the tray icon.
//
// 托盘左键已经改成直达主窗口（见 toggleMainWindow）；这个面板保留下来，因为
// 主窗口的首页工作台就是同一套快捷功能，而面板自身仍有渲染测试和截图模式。
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

// trayStatusText 只描述运行状态，不复读上方已选的划词模式。
func trayStatusText(cfg Config) string {
	hk := strings.TrimSpace(cfg.HotkeySelection)
	if hk != "" {
		return "运行中 · 划词 " + hk
	}
	return "运行中"
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
		Border(1, at.BorderWindow).Padding(Space3).Gap(Space3)
	if at.Dark {
		panel.InsetShadow(0, 0, 1, 0, at.InsetColor)
	}
	panel.Children(func() {
		// 1. 顶栏：淡蓝微底盒 Logo + 应用名 + Ghost 设置齿轮
		ui.Row(c).FillWidth().Height(HeightIconMd).AlignItems(ui.Center).Gap(Space2).Children(func() {
			ui.Box(c).Size(HeightIconSm, HeightIconSm).Radius(RadiusSM).Center().
				Background(at.AccentSoft).Children(func() {
				ui.Icon(c, iconDict).Size(14, 14).TextColor(at.Accent)
			})
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
				a.showMainWindow("vocabulary")
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

		// 划词模式切换：Compact 版 + 显式值映射，不依赖魔法下标。
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(Space2).Children(func() {
			ui.Text(c, "划词").Font(fontUI).FontSize(FontHelper).
				TextColor(at.TextMuted).SingleLine()
			SegmentedControlMapped(c, true, cfg.SelectionTrigger, func(mode string) {
				a.store.Update(func(c *Config) { c.SelectionTrigger = mode })
				a.applySelectionMode()
				// The tray menu's radio is fixed when built; without this it keeps
				// showing the mode that was active before the flyout changed it.
				a.rebuildTray()
			},
				SegmentedOption[string]{Value: "icon", Label: "显示图标"},
				SegmentedOption[string]{Value: "auto", Label: "直接翻译"},
				SegmentedOption[string]{Value: "hotkey", Label: "仅快捷键"},
			)
		})

		// 4. 底栏：1px 分割线 + 状态 + Danger-Ghost 退出。
		ui.Box(c).FillWidth().BorderWidth(1, 0, 0, 0).BorderColor(at.BorderSubtle).
			Padding(Space2, 0, 0, 0).Children(func() {
			ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(Space2).Children(func() {
				ui.Box(c).Size(6, 6).Radius(RadiusXS).Background(at.Success).Shrink(0)
				ui.Text(c, trayStatusText(cfg)).Font(fontUI).FontSize(FontHelper).
					TextColor(at.TextMuted).Grow(1).SingleLine()
				if DangerGhostButton(c, t, "退出应用").Clicked() {
					a.quitApp()
				}
			})
		})
	})
}
