package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func (a *app) resetNotebookConfirmations() {
	a.notebookConfirmDeleteID = ""
	a.confirmClearNotebook = false
}

func (a *app) resetNotebookViewState() {
	a.resetNotebookConfirmations()
	a.notebookExpandedID = ""
}

func (a *app) pageNotebook(c *ui.Context, t *ui.Theme, cfg Config) {
	at := appTheme(t)
	notebookStore := GetNotebookStore()
	PageTitle(c, t, "句法笔记", "集中归档长难句句法拆解与写作润色记录，随时展开复盘结构树与批注。")

	cAll, cGrammar, cWriting := notebookStore.KindCounts()
	ui.Box(c).FillWidth().Padding(Space3, Space3).Radius(RadiusTrack).Background(at.Surface).Border(1, at.BorderSubtle).Children(func() {
		ui.Column(c).FillWidth().Gap(Space2).Children(func() {
			SegmentedControlCompact(c, &a.notebookCategoryIdx,
				fmt.Sprintf("全部 (%d)", cAll),
				fmt.Sprintf("长难句 (%d)", cGrammar),
				fmt.Sprintf("写作 (%d)", cWriting),
			).OnChange(func() { a.resetNotebookViewState() })

			ui.SearchField(c, &a.notebookSearchQuery).
				Placeholder("搜索长难句、译文或写作笔记…").
				Label("搜索句法笔记").
				Height(HeightSearch).
				FillWidth().
				OnChange(func() { a.resetNotebookViewState() })

			ui.Row(c).FillWidth().Wrap().AlignItems(ui.Center).Gap(Space2).Children(func() {
				if a.lastDeletedNotebook != nil {
					btnUndo := AccentSoftButton(c, t, "↩ 撤销删除「"+truncateFor(a.lastDeletedNotebook.Source, 16)+"」").
						Height(HeightCompact).Tooltip("恢复刚刚删除的笔记条目")
					if btnUndo.Clicked() {
						notebookStore.Add(*a.lastDeletedNotebook)
						if err := notebookStore.LastSaveError(); err != nil {
							a.actionSavedMsg = "恢复失败: " + err.Error()
						} else {
							a.actionSavedMsg = "已恢复笔记 ✓"
							a.lastDeletedNotebook = nil
						}
						a.settingsRefresh()
					}
				}

				ui.Spacer(c).Grow(1)

				if notebookStore.Count() > 0 {
					if a.confirmClearNotebook {
						btnYes := ui.ButtonBase(c).Height(HeightCompact).Radius(RadiusSM).Padding(0, Space3).Center().
							Background(at.Danger).BorderWidth(0).TextColor(at.AccentText).Tooltip("删除全部句法与写作笔记，无法恢复")
						btnYes.Children(func() { buttonLabel(c, at, "确认清空", at.AccentText) })
						if btnYes.Clicked() {
							if err := notebookStore.Clear(); err != nil {
								a.actionSavedMsg = "清空失败: " + err.Error()
							} else {
								a.actionSavedMsg = "句法笔记已清空"
								a.notebookExpandedID = ""
							}
							a.confirmClearNotebook = false
							a.settingsRefresh()
						}
						btnNo := SecondaryButton(c, t, "取消").Height(HeightCompact)
						if btnNo.Clicked() {
							a.confirmClearNotebook = false
							a.settingsRefresh()
						}
					} else {
						btnClear := DangerGhostButton(c, t, "清空").Tooltip("清空当前所有句法与写作笔记（会先确认）")
						if btnClear.Clicked() {
							a.confirmClearNotebook = true
							a.settingsRefresh()
						}
					}
				}
			})
		})
	})

	kindKey := "all"
	switch a.notebookCategoryIdx {
	case 1:
		kindKey = "grammar"
	case 2:
		kindKey = "writing"
	}

	entries := notebookStore.Filter(kindKey, a.notebookSearchQuery)
	if len(entries) == 0 {
		ui.Box(c).FillWidth().Padding(36, 20).Radius(RadiusTrack).Background(at.Surface).Border(1, at.BorderSubtle).Children(func() {
			ui.Column(c).FillWidth().AlignItems(ui.Center).Gap(8).Children(func() {
				if a.notebookSearchQuery != "" {
					ui.Icon(c, iconSearch).Size(24, 24).TextColor(at.TextMuted)
					ui.Text(c, fmt.Sprintf("未找到与「%s」相关的句法或写作笔记", a.notebookSearchQuery)).
						FontSize(14).FontWeight(600).TextColor(at.Text)
					ui.Text(c, "尝试缩短搜索词，或切换上方分类查看。").FontSize(12).TextColor(at.TextMuted)
					if SecondaryButton(c, t, "清除搜索关键词").Height(HeightCompact).Clicked() {
						a.notebookSearchQuery = ""
						a.settingsRefresh()
					}
				} else {
					ui.Icon(c, iconGrammar).Size(24, 24).TextColor(at.TextMuted)
					title := "句法笔记还是空的"
					desc := "在划词弹窗或工作台的「长难句」点击【存入句法笔记】，或在「写作」点击【存入写作笔记】，即可在此复盘完整结构树与批注。"
					if a.notebookCategoryIdx == 1 {
						title = "暂无长难句拆解"
						desc = "选中长句并切换到「长难句」，点击【存入句法笔记】即可保存主干、括号法短语标注与结构树。"
					} else if a.notebookCategoryIdx == 2 {
						title = "暂无写作润色笔记"
						desc = "在「写作」生成批改与润色后，点击【存入写作笔记】即可归档原文、修改建议与导师总评。"
					}
					ui.Text(c, title).FontSize(14).FontWeight(600).TextColor(at.Text)
					ui.Text(c, desc).FontSize(12).TextColor(at.TextMuted).Wrap()
					hints := formatShortcutHints(
						shortcutHint{"划词翻译", cfg.HotkeySelection},
						shortcutHint{"输入翻译", cfg.HotkeyInput},
						shortcutHint{"截图翻译", cfg.HotkeyScreenshot},
					)
					if hints != "" {
						HelperText(c, t, "常用快捷键："+hints)
					}
				}
			})
		})
		renderNotebookPathFooter(c, t, notebookStore.Path())
		return
	}

	a.notebookList.Key = func(i int) any { return entries[i].ID }
	a.notebookList.MoveAnimation = &ui.Animation{Duration: 150 * time.Millisecond, Ease: ui.EaseOut}
	list := ui.List(c, &a.notebookList, len(entries), func(i int) {
		entry := entries[i]
		expanded := a.notebookExpandedID == entry.ID
		kindLabel := "长难句"
		expandLabel := "展开拆解"
		if normalizeNotebookKind(entry.Kind) == "writing" {
			kindLabel = "写作"
			expandLabel = "展开批注"
		}
		if expanded {
			expandLabel = "收起"
		}

		ui.Box(c).FillWidth().Padding(Space3, Space3+2).Radius(RadiusTrack).
			Background(at.Surface).Border(1, at.BorderSubtle).Children(func() {
			ui.Column(c).FillWidth().Gap(8).Children(func() {
				ui.Row(c).FillWidth().AlignItems(ui.Center).Justify(ui.SpaceBetween).Children(func() {
					ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
						ui.Box(c).Padding(1, 6).Radius(RadiusXS).Background(at.AccentSoft).Border(1, at.AccentRing).Children(func() {
							ui.Text(c, kindLabel).FontSize(11).FontWeight(600).TextColor(at.Accent)
						})
						if entry.Grammar != nil && len(entry.Grammar.Phrases) > 0 && !expanded {
							ui.Text(c, fmt.Sprintf("%d 处短语", len(entry.Grammar.Phrases))).
								Font(fontMono).FontFeatures("tnum").FontSize(10).TextColor(at.TextMuted)
						}
						if entry.Grammar != nil && len(entry.Grammar.Clauses) > 0 && !expanded {
							ui.Text(c, fmt.Sprintf("%d 处从句", len(entry.Grammar.Clauses))).
								Font(fontMono).FontFeatures("tnum").FontSize(10).TextColor(at.TextMuted)
						}
						if entry.Writing != nil && entry.Writing.Setting != "" && !expanded {
							ui.Text(c, entry.Writing.Setting).FontSize(11).TextColor(at.TextMuted)
						}
					})

					ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
						if !entry.CreatedAt.IsZero() {
							ui.Text(c, entry.CreatedAt.Format("2006-01-02 15:04")).
								Font(fontMono).FontFeatures("tnum").FontSize(11).TextColor(at.TextMuted)
						}

						btnExpand := ui.Button(c, "").Height(22).Padding(1, 8).Radius(4).Tooltip("展开或收起完整句法与批注")
						btnExpand.Children(func() {
							ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
								if expanded {
									ui.Icon(c, iconChevronUp).Size(11, 11).TextColor(at.TextSec)
								} else {
									ui.Icon(c, iconChevronDown).Size(11, 11).TextColor(at.TextSec)
								}
								ui.Text(c, expandLabel).FontSize(11).TextColor(at.TextSec)
							})
						})
						if btnExpand.Clicked() {
							if expanded {
								a.notebookExpandedID = ""
							} else {
								a.notebookExpandedID = entry.ID
								a.notebookOnlyTrunk = false
								a.notebookShowSlashes = false
								a.notebookShowTree = false
							}
							a.notebookConfirmDeleteID = ""
							a.settingsRefresh()
						}

						if a.notebookConfirmDeleteID == entry.ID {
							btnConfirm := ui.Button(c, "确认删除").Height(22).Padding(1, 8).Radius(4).
								Background(at.Danger).TextColor(at.AccentText).Tooltip("确认删除此笔记")
							if btnConfirm.Clicked() {
								entryCopy := entry
								if notebookStore.Delete(entry.ID) {
									a.lastDeletedNotebook = &entryCopy
									if err := notebookStore.LastSaveError(); err != nil {
										a.actionSavedMsg = "删除未写入磁盘: " + err.Error()
									} else {
										a.actionSavedMsg = "已删除笔记"
									}
								}
								if a.notebookExpandedID == entry.ID {
									a.notebookExpandedID = ""
								}
								a.notebookConfirmDeleteID = ""
								a.settingsRefresh()
							}
							btnCancel := ui.Button(c, "取消").Height(22).Padding(1, 6).Radius(4).Tooltip("取消删除")
							if btnCancel.Clicked() {
								a.notebookConfirmDeleteID = ""
								a.settingsRefresh()
							}
						} else {
							btnDel := ui.Button(c, "删除").Height(22).Padding(1, 8).Radius(4).
								TextColor(at.TextMuted).Tooltip("从句法笔记删除（点击后需二次确认）")
							if btnDel.Clicked() {
								a.notebookConfirmDeleteID = entry.ID
								a.settingsRefresh()
							}
						}
					})
				})

				ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Start).Children(func() {
					ui.Text(c, entry.Source).FontSize(14).FontWeight(600).TextColor(at.Text).Wrap().LineHeight(1.45).Grow(1)
					if renderIconButton(c, iconSpeak, "朗读", 22, 12, false).Clicked() {
						PlaySpeechLang(entry.Source, "", false)
					}
					if renderIconButton(c, iconCopy, "复制", 22, 12, false).Clicked() {
						mygo.Clipboard.WriteText(entry.Source)
						a.actionSavedMsg = "已复制原文 ✓"
						a.settingsRefresh()
					}
				})

				if !expanded {
					if summary := notebookEntrySummary(entry); summary != "" {
						ui.Box(c).FillWidth().Padding(8, 10).Radius(RadiusSM).Background(at.SurfaceHover).Children(func() {
							ui.Text(c, summary).FontSize(12).TextColor(at.TextSec).Wrap().LineHeight(1.5)
						})
					}
				} else {
					ui.Divider(c).FillWidth()
					if normalizeNotebookKind(entry.Kind) == "writing" {
						if entry.Writing != nil {
							renderImproveWritingCard(c, t, entry.Source, entry.Writing, true, func(txt string) {
								mygo.Clipboard.WriteText(txt)
								a.actionSavedMsg = "改后版本已复制 ✓"
								a.settingsRefresh()
							}, nil)
						} else if entry.Translation != "" {
							ui.Text(c, entry.Translation).FontSize(13).TextColor(at.Text).Wrap().LineHeight(1.55)
						}
					} else {
						if entry.Grammar != nil {
							st := grammarCardState{
								OnlyTrunk:   a.notebookOnlyTrunk,
								ShowSlashes: a.notebookShowSlashes,
								ShowTree:    a.notebookShowTree,
								OnToggleTrunk: func() {
									a.notebookOnlyTrunk = !a.notebookOnlyTrunk
									if a.notebookOnlyTrunk {
										a.notebookShowTree = false
									}
									a.settingsRefresh()
								},
								OnToggleSlash: func() {
									a.notebookShowSlashes = !a.notebookShowSlashes
									a.settingsRefresh()
								},
								OnToggleTree: func() {
									a.notebookShowTree = !a.notebookShowTree
									if a.notebookShowTree {
										a.notebookOnlyTrunk = false
									}
									a.settingsRefresh()
								},
								OnSpeak: func() { PlaySpeechLang(entry.Source, "", false) },
							}
							renderGrammarVisualCardEx(c, t, entry.Source, entry.Grammar, true, st)
						} else {
							ui.Column(c).FillWidth().Gap(8).Children(func() {
								if entry.Translation != "" {
									ui.Text(c, entry.Translation).FontSize(13).TextColor(at.Text).Wrap().LineHeight(1.55)
								}
								ui.Row(c).FillWidth().AlignItems(ui.Center).Justify(ui.SpaceBetween).Children(func() {
									ui.Text(c, "旧版迁移记录仅保留了原文与译文，可在工作台重新拆解并保存完整结构树。").
										FontSize(11).TextColor(at.TextMuted).Wrap().Grow(1)
									if SecondaryButton(c, t, "在工作台拆解").Height(HeightCompact).Clicked() {
										a.page = "home"
										a.home.tab = "grammar"
										a.home.input = entry.Source
										a.homeSubmit(entry.Source)
										a.settingsRefresh()
									}
								})
							})
						}
					}
				}
			})
		})
	})
	list.FillWidth()
	renderNotebookPathFooter(c, t, notebookStore.Path())
}

func notebookEntrySummary(e NotebookEntry) string {
	if normalizeNotebookKind(e.Kind) == "writing" {
		if e.Writing != nil && strings.TrimSpace(e.Writing.Improved) != "" {
			return "改写：" + e.Writing.Improved
		}
		return strings.TrimSpace(e.Translation)
	}
	if e.Grammar != nil && strings.TrimSpace(e.Grammar.BackboneSummary) != "" {
		return "主干：" + e.Grammar.BackboneSummary
	}
	return strings.TrimSpace(e.Translation)
}

func renderNotebookPathFooter(c *ui.Context, t *ui.Theme, nbPath string) {
	if nbPath == "" {
		nbPath = "仅内存（未落盘）"
	}
	at := appTheme(t)
	ui.Text(c, "存储位置："+nbPath).Font(fontMono).FontSize(FontMicro).TextColor(at.TextMuted).Wrap()
}
