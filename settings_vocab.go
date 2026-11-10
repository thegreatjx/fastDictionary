package main

import (
	"fmt"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"strings"
)

func (a *app) pageCustomActions(c *ui.Context, t *ui.Theme, cfg Config) {
	if a.editingActionID == "" {
		// List View
		heading(c, t, "AI 指令", "管理与定制划词悬浮窗中的独立 AI 指令。支持独立开关、模型配置与提示词深度定制。")

		ui.Column(c).FillWidth().Gap(10).Children(func() {
			for _, action := range cfg.CustomActions {
				act := action
				ui.Box(c).FillWidth().Padding(14, 16).Radius(8).
					Background(t.Surface).Border(1, t.Border).
					Shadow(0, 1, 4, 0, ui.RGBA(0, 0, 0, 0.04)).Children(func() {
					ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(14).Children(func() {
						// Action Icon
						var actIcon *ui.SVG
						switch act.ID {
						case "default-sentence-analysis":
							actIcon = iconGrammar
						case "default-improve-writing":
							actIcon = iconPolish
						default:
							actIcon = iconSparkles
						}
						ui.Box(c).Padding(8, 8).Radius(6).Background(t.SurfaceHover).Children(func() {
							ui.Icon(c, actIcon).Size(22, 22).TextColor(t.Accent)
						})

						// Title, Model badge & description
						ui.Column(c).Grow(1).Gap(3).Children(func() {
							ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
								ui.Text(c, act.Name).FontSize(15).FontWeight(600).TextColor(t.Text)
								if act.Model != "" {
									mBg := ui.Hex("#EFF6FF")
									mBorder := ui.Hex("#BFDBFE")
									mFg := ui.Hex("#1D4ED8")
									if t.Dark {
										mBg = withAlpha(t.Accent, 0.16)
										mBorder = withAlpha(t.Accent, 0.30)
										mFg = t.Accent
									}
									ui.Box(c).Padding(1, 6).Radius(10).Background(mBg).Border(1, mBorder).Children(func() {
										ui.Text(c, act.Model).FontSize(11).FontWeight(600).TextColor(mFg)
									})
								} else {
									ui.Box(c).Padding(1, 6).Radius(10).Background(t.SurfaceHover).Children(func() {
										ui.Text(c, "默认模型").FontSize(11).TextColor(t.TextMuted)
									})
								}
							})

							desc := "自定义 AI 指令"
							switch act.ID {
							case "default-sentence-analysis":
								desc = "长难句语法树拆解：主干、从句微标、连词与波浪线谓语标注"
							case "default-improve-writing":
								desc = "写作评改与润色：场景得体性、逐项错误修改建议与一键复制"
							}
							ui.Text(c, desc).FontSize(12).TextColor(t.TextMuted).Wrap()
						})

						// [定制化] button
						btnCustom := ui.Button(c, "定制化").Height(28).Padding(2, 14).Radius(5)
						if btnCustom.Clicked() {
							a.editingActionID = act.ID
							a.customizingAction = act
							a.actionSaveFeedback = ""
						}

						// Toggle Switch
						enabled := act.Enabled
						ui.Switch(c, &enabled).OnChange(func() {
							a.store.Update(func(c *Config) {
								for i := range c.CustomActions {
									if c.CustomActions[i].ID == act.ID {
										c.CustomActions[i].Enabled = enabled
									}
								}
							})
							if a.cfgWin != nil {
								a.cfgWin.Update(func() {})
							}
						})
					})
				})
			}
		})
	} else {
		// Customization View: Fully responsive cards with multi-line TextArea
		act := a.customizingAction
		heading(c, t, "定制化指令："+act.Name, "自由配置此指令调用的独立模型、系统提示词与输入模板。")

		// Top nav bar: [← 返回指令列表]  [恢复默认提示词]
		ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
			btnBack := ui.Button(c, "← 返回指令列表").Height(28).Padding(2, 12).Radius(5)
			if btnBack.Clicked() {
				a.editingActionID = ""
			}

			ui.Spacer(c).Grow(1)

			if strings.HasPrefix(a.editingActionID, "default-") {
				btnReset := ui.Button(c, "恢复默认提示词").Height(28).Padding(2, 12).Radius(5)
				if btnReset.Clicked() {
					a.store.Update(func(c *Config) {
						c.ResetAction(a.editingActionID)
					})
					if def := cfg.GetAction(a.editingActionID); def != nil {
						a.customizingAction = *def
					}
					a.actionSaveFeedback = "已恢复为默认提示词设置"
				}
			}
		})

		ui.Column(c).FillWidth().Gap(14).Children(func() {
			// 1. Basic configuration card (名称与模型)
			ui.Box(c).FillWidth().Padding(14, 16).Radius(8).
				Background(t.Surface).Border(1, t.Border).Children(func() {
				ui.Column(c).FillWidth().Gap(12).Children(func() {
					// 指令名称
					ui.Column(c).FillWidth().Gap(4).Children(func() {
						ui.Text(c, "指令名称").FontSize(13).FontWeight(600).TextColor(t.Text)
						ui.TextInput(c, &a.customizingAction.Name).FillWidth()
					})

					// 模型选择
					ui.Column(c).FillWidth().Gap(4).Children(func() {
						ui.Text(c, "调用模型").FontSize(13).FontWeight(600).TextColor(t.Text)
						models := []string{"跟随全局 (默认)"}
						for _, m := range a.getAvailableModels() {
							if m != "" && m != "跟随全局 (默认)" {
								models = append(models, m)
							}
						}
						curModel := a.customizingAction.Model
						if curModel == "" {
							curModel = "跟随全局 (默认)"
						} else {
							found := false
							for _, m := range models {
								if m == curModel {
									found = true
									break
								}
							}
							if !found {
								models = append(models, curModel)
							}
						}
						ui.Select(c, &curModel, models).FillWidth().OnChange(func() {
							if curModel == "跟随全局 (默认)" {
								a.customizingAction.Model = ""
							} else {
								a.customizingAction.Model = curModel
							}
						})
						ui.Text(c, "从本地 magpie 网关支持的模型列表中选择；也可在下方手动输入任意自定义模型 ID。").FontSize(11).TextColor(t.TextMuted)
					})

					// 自定义模型 ID
					ui.Column(c).FillWidth().Gap(4).Children(func() {
						ui.Text(c, "自定义模型 ID (可选)").FontSize(13).FontWeight(600).TextColor(t.Text)
						ui.TextInput(c, &a.customizingAction.Model).FillWidth().Placeholder("留空则跟随全局默认模型")
					})
				})
			})

			// 2. System Prompt Card (多行 TextArea，不再挤在单行)
			ui.Box(c).FillWidth().Padding(14, 16).Radius(8).
				Background(t.Surface).Border(1, t.Border).Children(func() {
				ui.Column(c).FillWidth().Gap(6).Children(func() {
					ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
						ui.Text(c, "系统提示词 (System Prompt)").FontSize(13).FontWeight(600).TextColor(t.Text)
						ui.Spacer(c).Grow(1)
						ui.Text(c, "指导 AI 的角色设定、专业规则与输出规范约定").FontSize(11).TextColor(t.TextMuted)
					})
					ui.TextArea(c, &a.customizingAction.SystemPrompt).Height(160).FillWidth()
				})
			})

			// 3. User Prompt Card (带占位符插入按钮与多行输入)
			ui.Box(c).FillWidth().Padding(14, 16).Radius(8).
				Background(t.Surface).Border(1, t.Border).Children(func() {
				ui.Column(c).FillWidth().Gap(6).Children(func() {
					ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
						ui.Text(c, "用户提示词 (User Prompt)").FontSize(13).FontWeight(600).TextColor(t.Text)
						ui.Spacer(c).Grow(1)
						ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
							ui.Text(c, "快捷插入变量：").FontSize(11).TextColor(t.TextMuted)
							btn1 := ui.Button(c, "{{selection}}").Height(22).Padding(1, 6).Radius(4)
							if btn1.Clicked() {
								a.customizingAction.Prompt += "{{selection}}"
							}
							btn2 := ui.Button(c, "{{paragraphs}}").Height(22).Padding(1, 6).Radius(4)
							if btn2.Clicked() {
								a.customizingAction.Prompt += "{{paragraphs}}"
							}
							btn3 := ui.Button(c, "{{targetLanguage}}").Height(22).Padding(1, 6).Radius(4)
							if btn3.Clicked() {
								a.customizingAction.Prompt += "{{targetLanguage}}"
							}
						})
					})
					ui.TextArea(c, &a.customizingAction.Prompt).Height(90).FillWidth()
					ui.Text(c, "用户请求模板，支持 {{selection}} (选中文本)、{{paragraphs}} (周围段落)、{{targetLanguage}} (目标语言) 占位符。").FontSize(11).TextColor(t.TextMuted)
				})
			})

			// 4. Output Schema Card
			ui.Box(c).FillWidth().Padding(10, 14).Radius(8).
				Background(t.SurfaceHover).Children(func() {
				ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(8).Children(func() {
					ui.Text(c, "输出字段约定 (Schema)：").FontSize(12).FontWeight(600).TextColor(t.Text)
					ui.Text(c, strings.Join(a.customizingAction.OutputSchema, " · ")).FontSize(12).TextColor(t.Accent).Grow(1).Wrap()
				})
			})

			// 5. Actions row
			ui.Row(c).FillWidth().Gap(10).AlignItems(ui.Center).Padding(4, 0).Children(func() {
				if ui.PrimaryButton(c, "保存修改").Width(140).Height(32).Clicked() {
					a.store.Update(func(c *Config) {
						for i := range c.CustomActions {
							if c.CustomActions[i].ID == a.editingActionID {
								c.CustomActions[i] = a.customizingAction
							}
						}
					})
					a.actionSaveFeedback = "已成功保存指令「" + a.customizingAction.Name + "」的定制修改"
					a.editingActionID = ""
				}
				if ui.Button(c, "取消").Width(90).Height(32).Clicked() {
					a.editingActionID = ""
				}
			})
		})
	}
}

func (a *app) pageVocabulary(c *ui.Context, t *ui.Theme, cfg Config) {
	vocabStore := GetVocabularyStore()
	// Show the path the store actually uses rather than a hardcoded guess:
	// defaultVocabPath answers from mygo's user data directory, which is
	// derived from the exe name, so any literal written here drifts the
	// moment the app is renamed.
	vocabPath := vocabStore.Path()
	if vocabPath == "" {
		vocabPath = "仅内存（未落盘）"
	}
	heading(c, t, "生词本与长难句笔记", "本地持久化存储（"+vocabPath+"）。支持分类归档、格式导出导入与智能补全。")

	// Filter & Action Bar
	ui.Box(c).FillWidth().Padding(10, 12).Radius(8).Background(t.Surface).Border(1, t.Border).Children(func() {
		ui.Column(c).FillWidth().Gap(8).Children(func() {
			// Row 1: Full-width Category Tabs — one pass over the store,
			// not four counters each scanning every entry per frame.
			cAll, cWords, cSentences, cWriting := vocabStore.CategoryCounts()
			catNames := []string{
				fmt.Sprintf("全部 (%d)", cAll),
				fmt.Sprintf("📘 生词 (%d)", cWords),
				fmt.Sprintf("🔍 长难句 (%d)", cSentences),
				fmt.Sprintf("✏️ 写作 (%d)", cWriting),
			}
			ui.Row(c).FillWidth().Children(func() {
				ui.Segmented(c, &a.vocabCategoryIdx, catNames...).FillWidth().Height(28).FontSize(12).
					OnChange(func() { a.vocabConfirmDeleteID = "" })
			})

			// Row 2: search on its own line. Sharing the row with six
			// buttons overflowed the form as soon as the window was narrow.
			ui.SearchField(c, &a.vocabSearchQuery).FillWidth().OnChange(func() { a.vocabConfirmDeleteID = "" })

			// Row 3: the actions, which wrap freely now.
			ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(6).Children(func() {
				if a.lastDeletedVocab != nil {
					btnUndo := ui.Button(c, "↩ 撤销删除「"+truncateFor(a.lastDeletedVocab.Term, 10)+"」").
						Height(28).Padding(2, 8).Radius(5).Background(t.SurfaceHover).
						TextColor(t.Accent).Tooltip("恢复刚刚删除的生词条目")
					if btnUndo.Clicked() {
						vocabStore.Add(*a.lastDeletedVocab)
						if err := vocabStore.LastSaveError(); err != nil {
							a.actionSavedMsg = "恢复失败: " + err.Error()
						} else {
							a.actionSavedMsg = "已恢复「" + a.lastDeletedVocab.Term + "」✓"
							a.lastDeletedVocab = nil
						}
						a.settingsRefresh()
					}
				}

				// [📥 导入]
				btnImport := ui.Button(c, "📥 导入").Height(28).Padding(2, 8).Radius(5).
					Tooltip("从外部 JSON/CSV/TXT 导入生词")
				if btnImport.Clicked() {
					go func() {
						if !a.tryBeginVocabJob() {
							return
						}
						defer endVocabJob()
						paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
							Title:   "导入生词文件",
							Filters: []mygo.FileFilter{{Name: "生词文件 (*.json, *.csv, *.txt)", Extensions: []string{"json", "csv", "txt"}}},
						})
						if err == nil && len(paths) > 0 {
							n, err := vocabStore.ImportFile(paths[0])
							a.mu.Lock()
							if err == nil {
								a.actionSavedMsg = fmt.Sprintf("成功导入 %d 条生词 ✓", n)
							} else {
								a.actionSavedMsg = "导入失败: " + err.Error()
							}
							a.mu.Unlock()
							if a.cfgWin != nil {
								a.cfgWin.Update(func() {})
							}
						}
					}()
				}

				// [📊 导出 CSV]
				btnExpCSV := ui.Button(c, "📊 导出 CSV").Height(28).Padding(2, 8).Radius(5).
					Tooltip("导出生词库为 CSV 表格（支持 Excel / Anki 直接打开）")
				if btnExpCSV.Clicked() {
					go func() {
						if !a.tryBeginVocabJob() {
							return
						}
						defer endVocabJob()
						path, err := mygo.Dialog.Save(mygo.SaveDialogOptions{
							Title:       "导出生词库 CSV",
							DefaultPath: "fast词典_生词库.csv",
							Filters:     []mygo.FileFilter{{Name: "CSV 表格", Extensions: []string{"csv"}}},
						})
						if err == nil && path != "" {
							if err := vocabStore.ExportCSV(path); err == nil {
								a.mu.Lock()
								a.actionSavedMsg = "已成功导出 CSV 表格 ✓"
								a.mu.Unlock()
							} else {
								a.mu.Lock()
								a.actionSavedMsg = "导出 CSV 失败: " + err.Error()
								a.mu.Unlock()
							}
							if a.cfgWin != nil {
								a.cfgWin.Update(func() {})
							}
						}
					}()
				}

				// [📇 导出 Anki]
				btnExpAnki := ui.Button(c, "📇 导出 Anki").Height(28).Padding(2, 8).Radius(5).
					Tooltip("导出为 Anki 记忆卡片格式（含完整排版与高亮样式，可直接导入 Anki）")
				if btnExpAnki.Clicked() {
					go func() {
						if !a.tryBeginVocabJob() {
							return
						}
						defer endVocabJob()
						path, err := mygo.Dialog.Save(mygo.SaveDialogOptions{
							Title:       "导出 Anki 记忆卡片",
							DefaultPath: "fast词典_生词库_Anki.txt",
							Filters:     []mygo.FileFilter{{Name: "Anki 卡片 (*.txt)", Extensions: []string{"txt"}}},
						})
						if err == nil && path != "" {
							if err := vocabStore.ExportAnki(path); err == nil {
								a.mu.Lock()
								a.actionSavedMsg = fmt.Sprintf("已成功导出 %d 张 Anki 记忆卡片 ✓", vocabStore.Count())
								a.mu.Unlock()
							} else {
								a.mu.Lock()
								a.actionSavedMsg = "导出 Anki 失败: " + err.Error()
								a.mu.Unlock()
							}
							if a.cfgWin != nil {
								a.cfgWin.Update(func() {})
							}
						}
					}()
				}

				// [✨ 补全]
				btnEnrich := ui.Button(c, "✨ 补全").Height(28).Padding(2, 8).Radius(5).
					Tooltip("为生词自动补全音标、CEFR 难度、例句与记忆技巧")
				if btnEnrich.Clicked() {
					go func() {
						if !a.tryBeginVocabJob() {
							return
						}
						defer endVocabJob()
						n, err := vocabStore.EnrichAll()
						a.mu.Lock()
						if err != nil {
							a.actionSavedMsg = "补全失败: " + err.Error()
						} else {
							a.actionSavedMsg = fmt.Sprintf("已智能补全 %d 条生词数据 ✓", n)
						}
						a.mu.Unlock()
						if a.cfgWin != nil {
							a.cfgWin.Update(func() {})
						}
					}()
				}

				// [🌐 网页版]
				btnWeb := ui.Button(c, "🌐 网页版").Height(28).Padding(2, 8).Radius(5).
					Tooltip("在默认浏览器中全屏查阅生词本")
				if btnWeb.Clicked() {
					if path, err := vocabStore.ExportHTML(); err == nil {
						go mygo.Shell.OpenExternal(path)
					}
				}

				// 清空 is destructive and irreversible, so it asks once.
				// It used to sit next to 导入/导出 with the same weight and
				// wiped the whole notebook on a single click.
				if vocabStore.Count() > 0 {
					if a.confirmClearVocab {
						btnYes := ui.Button(c, "确认清空").Height(28).Padding(2, 8).Radius(5).
							Background(t.Danger).TextColor(ui.Hex("#FFFFFF")).Tooltip("删除全部生词，无法恢复")
						if btnYes.Clicked() {
							if err := vocabStore.Clear(); err != nil {
								a.actionSavedMsg = "清空失败: " + err.Error()
							} else {
								a.actionSavedMsg = "生词本已清空"
							}
							a.confirmClearVocab = false
							a.settingsRefresh()
						}
						btnNo := ui.Button(c, "取消").Height(28).Padding(2, 8).Radius(5)
						if btnNo.Clicked() {
							a.confirmClearVocab = false
							a.settingsRefresh()
						}
					} else {
						btnClear := ui.Button(c, "清空").Height(28).Padding(2, 8).Radius(5).
							TextColor(t.Danger).Tooltip("清空当前所有笔记（会先确认）")
						if btnClear.Clicked() {
							a.confirmClearVocab = true
							a.settingsRefresh()
						}
					}
				}
			})
		})
	})

	catKey := "all"
	switch a.vocabCategoryIdx {
	case 1:
		catKey = "words"
	case 2:
		catKey = "sentences"
	case 3:
		catKey = "writing"
	}

	entries := vocabStore.Filter(catKey, a.vocabSearchQuery)
	if len(entries) == 0 {
		ui.Box(c).FillWidth().Padding(36, 20).Radius(8).Background(t.Surface).Border(1, t.Border).Children(func() {
			ui.Column(c).FillWidth().AlignItems(ui.Center).Gap(8).Children(func() {
				if a.vocabSearchQuery != "" {
					ui.Text(c, "🔍").FontSize(24)
					ui.Text(c, fmt.Sprintf("未找到与「%s」相关的笔记", a.vocabSearchQuery)).FontSize(14).FontWeight(600).TextColor(t.Text)
					ui.Text(c, "尝试缩短搜索词，或切换上方分类查看。").FontSize(12).TextColor(t.TextMuted)
					btnClearSearch := ui.Button(c, "清除搜索关键词").Height(26).Padding(2, 12).Radius(5).Background(t.SurfaceHover)
					if btnClearSearch.Clicked() {
						a.vocabSearchQuery = ""
						a.settingsRefresh()
					}
				} else {
					ui.Text(c, "📘").FontSize(24)
					title := "笔记本还是空的"
					desc := "在划词弹窗里点击【保存】即可自动归档；存下来的词可以在这里展开完整词卡与导出 Anki。"
					switch a.vocabCategoryIdx {
					case 1:
						title = "暂无生词收录"
						desc = "在划词翻译查词时，点击【保存生词】或【猜你想存】，即可将陌生词汇自动收录并补全音标与 CEFR 难度。"
					case 2:
						title = "暂无长难句分析"
						desc = "划词选中长句子后切换到【长难句】Tab，点击【保存到长难句本】即可归档语法树与拆解。"
					case 3:
						title = "暂无写作批改笔记"
						desc = "选中需要润色的句子后切换到【写作】Tab，点击【保存到笔记本】即可归档修改建议。"
					}
					ui.Text(c, title).FontSize(14).FontWeight(600).TextColor(t.Text)
					ui.Text(c, desc).FontSize(12).TextColor(t.TextMuted).Wrap()
					ui.Text(c, "常用快捷键：Alt+D 极速划词 · Alt+Q 输入翻译 · Alt+C 截图翻译").FontSize(11).TextColor(t.Accent)
				}
			})
		})
		return
	}

	// A virtual list: every entry used to be built into a full card tree
	// each frame (~25-40 nodes each), so a thousand entries meant tens of
	// thousands of nodes per keystroke in the search box. ui.List builds
	// only the rows in view; the row height adapts to the entry's content.
	list := ui.List(c, &a.vocabList, len(entries), func(i int) {
		entry := entries[i]
		expanded := a.vocabExpandedID == entry.ID
		ui.Box(c).FillWidth().Padding(12, 14).Radius(8).
			Background(t.Surface).Border(1, t.Border).
			Shadow(0, 1, 4, 0, ui.RGBA(0, 0, 0, 0.04)).Children(func() {
			ui.Column(c).FillWidth().Gap(8).Children(func() {
				// 1. Meta & Actions Header: Tags on left, Time & Delete on right (SpaceBetween, never overflows)
				ui.Row(c).FillWidth().AlignItems(ui.Center).Justify(ui.SpaceBetween).Children(func() {
					// Left: badges
					ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
						srcLabel := "📘 生词"
						switch classifyVocabEntry(entry) {
						case vocabCategorySentences:
							srcLabel = "🔍 长难句"
						case vocabCategoryWriting:
							srcLabel = "✏️ 写作"
						}
						ui.Box(c).Padding(1, 6).Radius(4).Background(t.SurfaceHover).Children(func() {
							ui.Text(c, srcLabel).FontSize(11).FontWeight(600).TextColor(t.Accent)
						})

						// Phonetic, part of speech and difficulty repeat the
						// card's own header once the row is expanded, so they
						// only appear in the collapsed summary.
						if !expanded {
							if entry.Phonetic != "" {
								ui.Text(c, entry.Phonetic).FontSize(12).TextColor(t.TextMuted)
							}

							if entry.PartOfSpeech != "" {
								ui.Box(c).Padding(1, 6).Radius(4).Background(t.SurfaceHover).Children(func() {
									ui.Text(c, entry.PartOfSpeech).FontSize(10).FontWeight(600).TextColor(t.Accent)
								})
							}

							if entry.Difficulty != "" {
								d := strings.ToUpper(strings.TrimSpace(entry.Difficulty))
								bg, fg := cefrColors(t, d)
								ui.Box(c).Padding(1, 6).Radius(10).Background(bg).Children(func() {
									ui.Text(c, d).FontSize(10).FontWeight(700).TextColor(fg)
								})
							}
						}
					})

					// Right: timestamp, expand toggle, delete. The toggle lives
					// up here rather than under the card because an expanded
					// card is tall enough to push a trailing control out of
					// sight, leaving no way to close it again.
					ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
						ui.Text(c, entry.CreatedAt.Format("2006-01-02 15:04")).FontSize(11).TextColor(t.TextMuted)

						isWordEntry := entry.Source != "grammar" && entry.Source != "writing" && len(strings.Fields(entry.Term)) <= 2
						if isWordEntry {
							btnExpand := ui.Button(c, "").Height(22).Padding(1, 8).Radius(4).Tooltip("展开完整词卡")
							btnExpand.Children(func() {
								ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
									if expanded {
										ui.Icon(c, iconChevronUp).Size(11, 11).TextColor(t.TextMuted)
									} else {
										ui.Icon(c, iconChevronDown).Size(11, 11).TextColor(t.TextMuted)
									}
									label := "词卡"
									if expanded {
										label = "收起"
									}
									ui.Text(c, label).FontSize(11).TextColor(t.TextMuted)
								})
							})
							if btnExpand.Clicked() {
								if expanded {
									a.vocabExpandedID = ""
								} else {
									a.vocabExpandedID = entry.ID
								}
								a.settingsRefresh()
							}
						}

						if a.vocabConfirmDeleteID == entry.ID {
							btnConfirm := ui.Button(c, "确认删除").Height(22).Padding(1, 8).Radius(4).
								Background(t.Danger).TextColor(ui.Hex("#FFFFFF")).Tooltip("确认从生词本删除此条目")
							if btnConfirm.Clicked() {
								entryCopy := entry
								if vocabStore.Delete(entry.ID) {
									a.lastDeletedVocab = &entryCopy
									if err := vocabStore.LastSaveError(); err != nil {
										a.actionSavedMsg = "删除未写入磁盘: " + err.Error()
									} else {
										a.actionSavedMsg = "已删除「" + entry.Term + "」"
									}
								}
								if a.vocabExpandedID == entry.ID {
									a.vocabExpandedID = ""
								}
								a.vocabConfirmDeleteID = ""
								a.settingsRefresh()
							}
							btnCancel := ui.Button(c, "取消").Height(22).Padding(1, 6).Radius(4).Tooltip("取消删除")
							if btnCancel.Clicked() {
								a.vocabConfirmDeleteID = ""
								a.settingsRefresh()
							}
						} else {
							btnDel := ui.Button(c, "删除").Height(22).Padding(1, 8).Radius(4).
								TextColor(t.TextMuted).Tooltip("从生词本删除此条目（点击后需二次确认）")
							if btnDel.Clicked() {
								a.vocabConfirmDeleteID = entry.ID
								a.settingsRefresh()
							}
						}
					})
				})

				// The row carries the same fields the card does, so drawing
				// both would print the definition, example and memory tips
				// twice and push 收起词卡 below the fold. Expanded shows the
				// card alone; collapsed shows the compact summary.
				isWordEntry := entry.Source != "grammar" && entry.Source != "writing" && len(strings.Fields(entry.Term)) <= 2
				if expanded && isWordEntry {
					// The full word card. It used to be the whole point of a
					// 词典 tab; keeping it here means the notebook is the one
					// place a saved word can be studied in full, without a
					// second AI pass or a second copy of the same text.
					renderVocabWordCard(c, t, entry)
				} else {
					// 2. Term / Sentence text (Full width, wraps gracefully).
					// The speaker sits with the term because that is what gets
					// spoken: the definition is a translation, and reading it
					// back in the source language's voice says nothing useful.
					// Speaking costs nothing extra — a word tries Youdao's
					// free audio first and falls back to Edge TTS, the same
					// path the popup uses.
					ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
						ui.Text(c, entry.Term).FontSize(15).FontWeight(700).TextColor(t.Text).Wrap()
						if renderIconButton(c, iconSpeak, "朗读", 22, 12, false).Clicked() {
							PlaySpeechLang(entry.Term, "", false)
						}
					})

					// 2. Definition
					if entry.Definition != "" {
						ui.Text(c, entry.Definition).FontSize(13).TextColor(t.Text).Wrap()
					}

					// 3. Context & Translation
					if entry.Context != "" {
						ui.Box(c).FillWidth().Padding(8, 10).Radius(6).Background(t.SurfaceHover).Children(func() {
							ui.Column(c).FillWidth().Gap(4).Children(func() {
								ui.Text(c, entry.Context).FontSize(12).FontWeight(500).TextColor(t.Text).Wrap()
								if entry.ContextTranslation != "" {
									ui.Text(c, entry.ContextTranslation).FontSize(12).TextColor(t.TextMuted).Wrap()
								}
							})
						})
					}

					// 4. Memory Tips. The tint follows the theme; the fixed
					// amber was a bright block with dark text under it in
					// dark mode.
					if entry.MemoryTips != "" {
						tipBg, tipBorder, _, tipBody := memoryTipColors(t)
						ui.Box(c).FillWidth().Padding(6, 10).Radius(4).Background(tipBg).Border(1, tipBorder).Children(func() {
							ui.Row(c).FillWidth().Gap(4).AlignItems(ui.Start).Children(func() {
								ui.Text(c, "💡").FontSize(12)
								ui.Text(c, entry.MemoryTips).FontSize(12).TextColor(tipBody).Wrap().Grow(1)
							})
						})
					}
				}
			})
		})
	})
	list.FillWidth()
}

// renderVocabWordCard draws the full dictionary card for a saved entry — the
// layout the 词典 tab used to own, rebuilt from what the notebook already
// holds. It does not call the model: a saved entry is a fixed record, and
// re-asking would return a different answer than the one the user chose to
// keep. savedTerm is the entry's own term so the card reads 已保存 instead of
// inviting a second write of the same word.
func renderVocabWordCard(c *ui.Context, t *ui.Theme, entry VocabularyEntry) {
	if entry.Term == "" || (entry.Definition == "" && entry.Context == "" && entry.MemoryTips == "") {
		return
	}
	res := &DictionaryResult{
		Term:               entry.Term,
		Phonetic:           entry.Phonetic,
		PartOfSpeech:       entry.PartOfSpeech,
		Definition:         entry.Definition,
		Context:            entry.Context,
		ContextTerm:        entry.ContextTerm,
		ContextTranslation: entry.ContextTranslation,
		MemoryTips:         entry.MemoryTips,
		Difficulty:         entry.Difficulty,
	}
	ui.Divider(c).FillWidth()
	renderDictionaryCard(c, t, entry.Term, res, entry.Term, nil)
}
