package main

import (
	"encoding/json"
	"strings"

	"github.com/egoist/mygo/ui"
)

// SyntaxAnnotation represents one grammatical unit from Read-Frog's sentence analysis
type SyntaxAnnotation struct {
	Text       string `json:"text"`
	Type       string `json:"type"` // subject, predicate, object, complement, attributive, adverbial, appositive, connector
	Form       string `json:"form,omitempty"`
	Sense      string `json:"sense,omitempty"`
	Head       string `json:"head,omitempty"`
	Obstacle   string `json:"obstacle,omitempty"`
	Restore    string `json:"restore,omitempty"`
	Note       string `json:"note,omitempty"`
	Occurrence int    `json:"occurrence,omitempty"`
}

// SentenceAnalysisResult is the structured result returned by the grammar tutor LLM
type SentenceAnalysisResult struct {
	Annotations []SyntaxAnnotation `json:"annotations"`
	Translation string             `json:"translation"`
	Raw         string             `json:"-"`
}

// ParseSentenceAnalysis parses the model output into a structured result
func ParseSentenceAnalysis(raw string) *SentenceAnalysisResult {
	cleaned := CleanJSONText(raw)
	if cleaned == "" {
		return nil
	}

	type rawGrammar struct {
		Annotations    any    `json:"annotations"`
		AnnotationsZH  any    `json:"标注"`
		AnnotationsDol any    `json:"$1"`
		Translation    string `json:"translation"`
		TranslationZH  string `json:"译文"`
		TranslationDol string `json:"$2"`
	}

	var g rawGrammar
	if err := json.Unmarshal([]byte(cleaned), &g); err == nil {
		annVal := firstNonNil(g.Annotations, g.AnnotationsZH, g.AnnotationsDol)
		trans := firstNonEmpty(g.Translation, g.TranslationZH, g.TranslationDol)
		var list []SyntaxAnnotation
		if annVal != nil {
			if s, ok := annVal.(string); ok {
				_ = json.Unmarshal([]byte(s), &list)
			} else {
				data, _ := json.Marshal(annVal)
				_ = json.Unmarshal(data, &list)
			}
		}
		if len(list) > 0 || trans != "" {
			return &SentenceAnalysisResult{
				Annotations: list,
				Translation: trans,
				Raw:         raw,
			}
		}
	}

	// Fallback 1: try direct unmarshal
	var res SentenceAnalysisResult
	if err := json.Unmarshal([]byte(cleaned), &res); err == nil && len(res.Annotations) > 0 {
		res.Raw = raw
		return &res
	}

	// Fallback 2: try parsing as raw annotations array
	var list []SyntaxAnnotation
	if err := json.Unmarshal([]byte(cleaned), &list); err == nil && len(list) > 0 {
		return &SentenceAnalysisResult{
			Annotations: list,
			Translation: "",
			Raw:         raw,
		}
	}

	return nil
}

// Translated badge labels matching Read-Frog zh-CN
func clauseBadgeLabel(item SyntaxAnnotation) string {
	if item.Sense != "" {
		switch item.Sense {
		case "time":
			return "时间状从"
		case "place":
			return "地点状从"
		case "cause":
			return "原因状从"
		case "condition":
			return "条件状从"
		case "concession":
			return "让步状从"
		case "purpose":
			return "目的状从"
		case "result":
			return "结果状从"
		case "manner":
			return "方式状从"
		case "comparison":
			return "比较状从"
		}
	}
	if item.Form == "clause" {
		switch item.Type {
		case "subject":
			return "主语从句"
		case "object":
			return "宾语从句"
		case "complement":
			return "表语从句"
		case "attributive":
			return "定语从句"
		case "adverbial":
			return "状语从句"
		case "appositive":
			return "同位语从句"
		default:
			return "从句"
		}
	}
	if item.Form != "" {
		switch item.Form {
		case "infinitive":
			return "不定式"
		case "gerund":
			return "动名词"
		case "present-participle":
			return "现在分词"
		case "past-participle":
			return "过去分词"
		}
	}
	return ""
}

// obstacleBadgeLabel maps obstacle keywords to Chinese tags
func obstacleBadgeLabel(item SyntaxAnnotation) string {
	switch item.Obstacle {
	case "passive":
		return "被动"
	case "inversion":
		return "倒装"
	case "fronting":
		return "前置"
	case "ellipsis":
		return "省略"
	case "subjunctive":
		return "虚拟"
	case "dummy-it":
		return "形式主语"
	case "postponed-subject":
		return "真主语"
	case "cleft":
		return "强调句"
	case "parenthesis":
		return "插入语"
	case "split":
		return "拆分动词"
	case "idiom":
		return "习语表达"
	case "comparison":
		return "比较结构"
	case "negation":
		return "否定结构"
	}
	return ""
}

var grammarExplanations = map[string]string{
	"subject":    "【主语 (Subject)】：句子的陈述主体，动作的发起者或状态的承担者。主干核心成分之一。",
	"predicate":  "【谓语 (Predicate)】：说明主语“做什么”或“处于什么状态”，由动词构成。深蓝波浪线标注核心谓语动词。",
	"object":     "【宾语 (Object)】：动作的承受者或及物动词支配的对象。主干核心成分之一。",
	"connector":  "【连接词 (Connector)】：引导从句并起语法衔接纽带作用（如 that, which, if, because 等）。",
	"complement": "【表语/补语 (Complement)】：紧跟系动词，补充说明主语或宾语的性质、特征或状态。",
	"表语从句":       "【表语从句 (Predicative Clause)】：位于系动词后充当表语，详细解释主语的内容或性质。",
	"宾语从句":       "【宾语从句 (Object Clause)】：位于及物动词或介词后充当宾语，说明主句主语所想、所说或感知的事实。",
	"定语从句":       "【定语从句 (Attributive Clause)】：修饰先行词（名词或代词），起到形容词作用，限定或补充说明人或事物。",
	"主语从句":       "【主语从句 (Subject Clause)】：在复合句中充当主语，常用 it 作形式主语，而将真主语从句后置。",
	"同位语从句":      "【同位语从句 (Appositive Clause)】：位于抽象名词后，对其具体内容作完整阐释。",
	"条件状从":       "【条件状语从句 (Condition)】：通常由 if, unless 引导，表示主句动作发生的条件或假设。",
	"时间状从":       "【时间状语从句 (Time)】：由 when, while, as, before, after 引导，说明动作发生的时间背景。",
	"原因状从":       "【原因状语从句 (Cause)】：由 because, since, as 引导，说明动作产生的根源或理由。",
	"让步状从":       "【让步状语从句 (Concession)】：由 although, though, even if 引导，表示转折让步。",
	"结果状从":       "【结果状语从句 (Result)】：由 so...that 引导，说明前述原因直接导致的结果。",
	"目的状从":       "【目的状语从句 (Purpose)】：由 so that, in order that 引导，表示主句动作要达到的目的。",
	"被动":         "【被动语态 (Passive Voice)】：结构为 be + 过去分词，强调动作的承受者而非发出者。",
	"倒装":         "【倒装句 (Inversion)】：谓语动词或助动词置于主语之前，用于加强语气或承接上下文。",
	"前置":         "【成分前置 (Fronting)】：将宾语、状语或表语提到句首，起到突出强调或话题承接作用。",
	"省略":         "【省略句 (Ellipsis)】：省略句中重复或显而易见的成分，使表达更加精炼紧凑。",
	"虚拟":         "【虚拟语气 (Subjunctive)】：动词用过去时或 should+动词原形，表示非真实假设、愿望或建议。",
	"形式主语":       "【形式主语 (Dummy-it)】：用 it 占据句首主语位置，避免头重脚轻，真主语（从句或不定式）后置。",
}

// renderGrammarVisualCard renders the interactive visual grammar tree card matching user screenshot
func (a *app) renderGrammarVisualCard(c *ui.Context, t *ui.Theme, origText string, res *SentenceAnalysisResult, onSave func()) {
	// 1. Collect obstacles and structural components
	var obstacles []SyntaxAnnotation
	var subjects []string
	var predicates []string
	var objects []string
	var clauses []SyntaxAnnotation

	for _, ann := range res.Annotations {
		if ann.Obstacle != "" || ann.Restore != "" {
			obstacles = append(obstacles, ann)
		}
		if ann.Form == "clause" {
			clauses = append(clauses, ann)
			continue
		}
		switch strings.ToLower(ann.Type) {
		case "subject":
			subjects = append(subjects, ann.Text)
		case "predicate":
			predicates = append(predicates, ann.Text)
		case "object", "complement":
			objects = append(objects, ann.Text)
		}
	}

	ui.Box(c).FillWidth().Padding(14, 16).Radius(8).
		Background(t.Surface).Border(1, t.Border).
		Shadow(0, 2, 8, 0, ui.RGBA(0, 0, 0, 0.08)).Children(func() {
		ui.Column(c).FillWidth().Gap(10).Children(func() {
			// 1. Header: Mode indicator badge & Speak button (Centered)
			ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
				ui.Box(c).Padding(3, 10).Radius(12).Background(ui.Hex("#EFF6FF")).Border(1, ui.Hex("#BFDBFE")).Children(func() {
					ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
						ui.Text(c, "🔍").FontSize(11)
						ui.Text(c, "长难句深度拆解").FontSize(11).FontWeight(600).TextColor(ui.Hex("#1D4ED8"))
					})
				})

				ui.Spacer(c).Grow(1)

				btnSpeak := renderIconButton(c, iconSpeak, "朗读整句", 24, 13, false)
				if btnSpeak.Clicked() {
					PlaySpeech(origText, false)
				}
			})

			// 2. Main Sentence with Annotated Highlights (Spans & Badges)
			ui.Box(c).FillWidth().Padding(12, 14).Radius(6).Background(t.SurfaceHover).Border(1, t.Border).Children(func() {
				ui.Column(c).FillWidth().Gap(6).Children(func() {
					a.renderGrammarSpans(c, t, origText, res)
				})
			})

			// 3. Filter controls & Interactive Legend: [只看主干] [断句]  |  [主语] [谓语] [宾语]
			ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(8).Padding(2, 0, 2, 0).Children(func() {
				// [只看主干] Toggle button
				trunkBg := t.SurfaceHover
				trunkText := t.Text
				if a.onlyTrunk {
					trunkBg = ui.Hex("#2563EB")
					trunkText = ui.Hex("#FFFFFF")
				}
				btnTrunk := ui.Button(c, "").Height(24).Padding(2, 10).Radius(5).Background(trunkBg).TextColor(trunkText).Tooltip("只高亮主句核心骨架（主谓宾），淡化修饰成分")
				btnTrunk.Children(func() {
					ui.Text(c, "只看主干").FontSize(11).FontWeight(600)
				})
				if btnTrunk.Clicked() {
					a.onlyTrunk = !a.onlyTrunk
				}

				// [断句] Toggle button
				slashBg := t.SurfaceHover
				slashText := t.Text
				if a.showSlashes {
					slashBg = ui.Hex("#4F46E5")
					slashText = ui.Hex("#FFFFFF")
				}
				btnSlash := ui.Button(c, "").Height(24).Padding(2, 10).Radius(5).Background(slashBg).TextColor(slashText).Tooltip("切分意群与从句停顿位置，显示 / 断句标记")
				btnSlash.Children(func() {
					ui.Text(c, "断句").FontSize(11).FontWeight(600)
				})
				if btnSlash.Clicked() {
					a.showSlashes = !a.showSlashes
				}

				ui.Spacer(c).Grow(1)

				// Interactive Legend buttons: [主语] [谓语] [宾语]
				ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
					// 主语 button
					subjBg := t.SurfaceHover
					if a.filterRole == "subject" {
						subjBg = ui.Hex("#FEF08A")
					}
					btnSubj := ui.Button(c, "").Height(22).Padding(1, 6).Radius(4).Background(subjBg).Tooltip("点击高亮整句中所有主语")
					btnSubj.Children(func() {
						ui.Text(c, "主语").FontSize(11).FontWeight(600).Underline().DecorationColor(ui.Hex("#64748B")).TextColor(ui.Hex("#0F172A"))
					})
					if btnSubj.Clicked() {
						if a.filterRole == "subject" {
							a.filterRole = ""
							a.grammarHoverKnowledge = ""
						} else {
							a.filterRole = "subject"
							a.grammarHoverKnowledge = grammarExplanations["subject"]
						}
					}

					// 谓语 button
					predBg := t.SurfaceHover
					if a.filterRole == "predicate" {
						predBg = ui.Hex("#DBEAFE")
					}
					btnPred := ui.Button(c, "").Height(22).Padding(1, 6).Radius(4).Background(predBg).Tooltip("点击高亮整句中所有谓语波浪线")
					btnPred.Children(func() {
						ui.Text(c, "谓语").FontSize(11).FontWeight(600).WavyUnderline().DecorationColor(ui.Hex("#2563EB")).TextColor(ui.Hex("#2563EB"))
					})
					if btnPred.Clicked() {
						if a.filterRole == "predicate" {
							a.filterRole = ""
							a.grammarHoverKnowledge = ""
						} else {
							a.filterRole = "predicate"
							a.grammarHoverKnowledge = grammarExplanations["predicate"]
						}
					}

					// 宾语 button
					objBg := t.SurfaceHover
					if a.filterRole == "object" {
						objBg = ui.Hex("#DCFCE7")
					}
					btnObj := ui.Button(c, "").Height(22).Padding(1, 6).Radius(4).Background(objBg).Tooltip("点击高亮整句中所有宾语")
					btnObj.Children(func() {
						ui.Text(c, "宾语").FontSize(11).FontWeight(600).Underline().DecorationColor(ui.Hex("#0F172A")).TextColor(ui.Hex("#0F172A"))
					})
					if btnObj.Clicked() {
						if a.filterRole == "object" {
							a.filterRole = ""
							a.grammarHoverKnowledge = ""
						} else {
							a.filterRole = "object"
							a.grammarHoverKnowledge = grammarExplanations["object"]
						}
					}
				})
			})

			// 3.5 Live Grammar Knowledge Card (科普语法结构)
			if a.grammarHoverKnowledge != "" {
				ui.Box(c).FillWidth().Padding(8, 12).Radius(6).
					Background(ui.Hex("#EFF6FF")).Border(1, ui.Hex("#BFDBFE")).Children(func() {
					ui.Row(c).Gap(6).AlignItems(ui.Start).Children(func() {
						ui.Text(c, "💡").FontSize(12)
						ui.Text(c, a.grammarHoverKnowledge).FontSize(12).FontWeight(500).TextColor(ui.Hex("#1E40AF")).Wrap().Grow(1)
					})
				})
			}

			// 4. [阅读蛙特色 1] ★ 句式难点与语序还原看板 (Obstacles & Restorations)
			if len(obstacles) > 0 {
				ui.Box(c).FillWidth().Padding(10, 12).Radius(6).
					Background(ui.Hex("#FEFCE8")).Border(1, ui.Hex("#FEF08A")).Children(func() {
					ui.Column(c).FillWidth().Gap(8).Children(func() {
						ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
							ui.Text(c, "★ 句式难点突破").FontSize(12).FontWeight(700).TextColor(ui.Hex("#854D0E"))
						})
						for _, obs := range obstacles {
							obsItem := obs
							label := obstacleBadgeLabel(obsItem)
							if label == "" {
								label = obsItem.Obstacle
							}
							ui.Column(c).FillWidth().Gap(3).Padding(2, 4).Children(func() {
								ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
									ui.Box(c).Padding(1, 6).Radius(4).Background(ui.Hex("#FACC15")).Children(func() {
										ui.Text(c, label).FontSize(10).FontWeight(700).TextColor(ui.Hex("#713F12"))
									})
									ui.Text(c, obsItem.Text).FontSize(13).FontWeight(600).TextColor(ui.Hex("#713F12")).Wrap().Grow(1)
								})
								if obsItem.Restore != "" {
									ui.Row(c).Gap(6).AlignItems(ui.Start).Padding(1, 0, 1, 14).Children(func() {
										ui.Text(c, "还原语序：").FontSize(11).FontWeight(600).TextColor(ui.Hex("#B45309"))
										ui.Text(c, obsItem.Restore).FontSize(12).FontWeight(600).TextColor(ui.Hex("#1D4ED8")).Wrap().Grow(1)
									})
								}
								if obsItem.Note != "" {
									ui.Row(c).Gap(6).AlignItems(ui.Start).Padding(0, 0, 0, 14).Children(func() {
										ui.Text(c, "解析说明：").FontSize(11).TextColor(ui.Hex("#92400E"))
										ui.Text(c, obsItem.Note).FontSize(12).TextColor(ui.Hex("#78350F")).Wrap().Grow(1)
									})
								}
							})
						}
					})
				})
			}

			// 5. [阅读蛙特色 2] 句子主干骨架速查 (Trunk Core Summary)
			if len(predicates) > 0 || len(clauses) > 0 {
				ui.Box(c).FillWidth().Padding(8, 12).Radius(6).
					Background(t.SurfaceHover).Children(func() {
					ui.Column(c).FillWidth().Gap(4).Children(func() {
						if len(predicates) > 0 {
							trunkText := ""
							if len(subjects) > 0 {
								trunkText += "主语: " + strings.Join(subjects, " / ") + "  ➔  "
							}
							trunkText += "谓语: " + strings.Join(predicates, " / ")
							if len(objects) > 0 {
								trunkText += "  ➔  宾语: " + strings.Join(objects, " / ")
							}
							ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
								ui.Text(c, "主干核心:").FontSize(11).FontWeight(600).TextColor(t.TextMuted)
								ui.Text(c, trunkText).FontSize(12).FontWeight(600).TextColor(t.Text).Wrap().Grow(1)
							})
						}
						if len(clauses) > 0 {
							var clauseStrs []string
							for _, cl := range clauses {
								b := clauseBadgeLabel(cl)
								if b == "" {
									b = "从句"
								}
								s := b + ": " + cl.Text
								if cl.Head != "" {
									s += " (修饰: " + cl.Head + ")"
								}
								clauseStrs = append(clauseStrs, s)
							}
							ui.Row(c).Gap(6).AlignItems(ui.Start).Children(func() {
								ui.Text(c, "从句拆解:").FontSize(11).FontWeight(600).TextColor(t.TextMuted)
								ui.Text(c, strings.Join(clauseStrs, " · ")).FontSize(12).TextColor(t.TextMuted).Wrap().Grow(1)
							})
						}
					})
				})
			}

			ui.Divider(c).FillWidth()

			// 6. Natural Chinese Translation
			if res.Translation != "" {
				ui.Column(c).FillWidth().Gap(4).Children(func() {
					ui.Text(c, "地道精翻:").FontSize(11).FontWeight(600).TextColor(t.TextMuted)
					ui.Text(c, res.Translation).FontSize(14).FontWeight(500).TextColor(t.Text).Wrap()
				})
			}

			ui.Divider(c).FillWidth()

			// 7. Bottom action bar: [ 保存到长难句本 ]
			ui.Row(c).FillWidth().Justify(ui.End).AlignItems(ui.Center).Children(func() {
				saveLabel := "保存到长难句本"
				saveBg := ui.Hex("#EAB308")
				if a.notesSaved {
					saveLabel = "✓ 已存入笔记本"
					saveBg = ui.Hex("#10B981")
				}
				btnSave := ui.Button(c, "").Height(26).Padding(2, 14).Radius(5).
					Background(saveBg).TextColor(ui.Hex("#FFFFFF")).Tooltip("将该长难句分析持久化保存到本地笔记本")
				btnSave.Children(func() {
					ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
						ui.Icon(c, iconBookmark).Size(13, 13).TextColor(ui.Hex("#FFFFFF"))
						ui.Text(c, saveLabel).FontSize(11).FontWeight(600)
					})
				})
				if btnSave.Clicked() {
					if onSave != nil {
						onSave()
					}
					a.mu.Lock()
					a.notesSaved = true
					a.mu.Unlock()
					if a.win != nil {
						a.win.Update(func() {})
					}
				}
			})
		})
	})
}

// GrammarToken represents a single word with mapped syntax information
type GrammarToken struct {
	Word       string
	CleanWord  string
	Start      int
	End        int
	OpenClause string
	Role       string
	RoleLabel  string
	Obstacle   string
	HoverTip   string
	IsCore     bool
}

type clauseSpan struct {
	start int
	end   int
	label string
}

type roleSpan struct {
	start    int
	end      int
	role     string
	label    string
	obstacle string
	restore  string
}

// buildGrammarTokens calculates exact, non-duplicating syntax spans
func buildGrammarTokens(origText string, res *SentenceAnalysisResult) []GrammarToken {
	if res == nil {
		return nil
	}

	// 1. Gather all clause boundaries
	var clauses []clauseSpan
	for _, ann := range res.Annotations {
		badge := clauseBadgeLabel(ann)
		if badge != "" || ann.Form == "clause" {
			if badge == "" {
				badge = "从句"
			}
			idx := strings.Index(origText, ann.Text)
			if idx >= 0 {
				clauses = append(clauses, clauseSpan{
					start: idx,
					end:   idx + len(ann.Text),
					label: badge,
				})
			}
		}
	}

	// 2. Gather constituent role boundaries (word or short phrase)
	var roles []roleSpan
	for _, ann := range res.Annotations {
		if ann.Form == "clause" {
			continue // clauses handled above
		}
		rType := strings.ToLower(ann.Type)
		if rType == "subject" || rType == "predicate" || rType == "object" || rType == "connector" || rType == "complement" {
			idx := strings.Index(origText, ann.Text)
			if idx >= 0 {
				rLabel := "成分"
				switch rType {
				case "subject":
					rLabel = "主语"
				case "predicate":
					rLabel = "谓语"
				case "object":
					rLabel = "宾语"
				case "connector":
					rLabel = "连接词"
				case "complement":
					rLabel = "表语/补语"
				}
				roles = append(roles, roleSpan{
					start:    idx,
					end:      idx + len(ann.Text),
					role:     rType,
					label:    rLabel,
					obstacle: obstacleBadgeLabel(ann),
					restore:  ann.Restore,
				})
			}
		}
	}

	// 3. Tokenize words with precise character offsets
	var tokens []GrammarToken
	words := strings.Fields(origText)
	searchPos := 0
	clauseEmitted := make(map[int]bool)

	for _, w := range words {
		pos := strings.Index(origText[searchPos:], w)
		if pos < 0 {
			continue
		}
		absStart := searchPos + pos
		absEnd := absStart + len(w)
		searchPos = absEnd

		clean := strings.Trim(w, ",.?!;:()\"'")

		tok := GrammarToken{
			Word:      w,
			CleanWord: clean,
			Start:     absStart,
			End:       absEnd,
		}

		// Emit clause badge ONCE at the start of that clause
		for cIdx, c := range clauses {
			if absStart <= c.start && absEnd > c.start && !clauseEmitted[cIdx] {
				tok.OpenClause = c.label
				clauseEmitted[cIdx] = true
			}
		}

		// Check role overlap
		for _, r := range roles {
			if absStart < r.end && absEnd > r.start {
				tok.Role = r.role
				tok.RoleLabel = r.label
				tok.Obstacle = r.obstacle
				tok.IsCore = (r.role == "subject" || r.role == "predicate" || r.role == "object")

				tip := r.label
				if r.obstacle != "" {
					tip += " ★" + r.obstacle
				}
				if r.restore != "" {
					tip += " (还原语序: " + r.restore + ")"
				}
				tok.HoverTip = tip
				break
			}
		}

		tokens = append(tokens, tok)
	}

	return tokens
}

// renderGrammarSpans builds the visual tokens from annotations with clean clause chips and hover tooltips
func (a *app) renderGrammarSpans(c *ui.Context, t *ui.Theme, origText string, res *SentenceAnalysisResult) {
	tokens := buildGrammarTokens(origText, res)

	ui.Row(c).FillWidth().Wrap().Gap(4).AlignItems(ui.Center).Children(func() {
		for _, tok := range tokens {
			item := tok

			// Dim non-core elements when onlyTrunk is active
			if a.onlyTrunk && !item.IsCore {
				ui.Text(c, item.Word+" ").FontSize(14).TextColor(t.TextMuted)
				continue
			}

			// Render break slash if showSlashes is enabled and clause opens
			if a.showSlashes && item.OpenClause != "" {
				ui.Box(c).Padding(0, 3).Children(func() {
					ui.Text(c, "/").FontSize(14).FontWeight(700).TextColor(ui.Hex("#818CF8"))
				})
			}

			// Render clause badge ONCE at clause entrance
			if item.OpenClause != "" {
				clauseTip := "从句类别: " + item.OpenClause
				if exp, ok := grammarExplanations[item.OpenClause]; ok {
					clauseTip = exp
				}
				clauseBadge := ui.Box(c).Padding(1, 6).Radius(4).
					Background(ui.Hex("#DCFCE7")).Border(1, ui.Hex("#86EFAC")).
					Tooltip(clauseTip)
				if clauseBadge.Hovered() {
					if exp, ok := grammarExplanations[item.OpenClause]; ok {
						a.grammarHoverKnowledge = exp
					}
				}
				clauseBadge.Children(func() {
					ui.Row(c).Gap(2).AlignItems(ui.Center).Children(func() {
						ui.Text(c, item.OpenClause).FontSize(10).FontWeight(600).TextColor(ui.Hex("#166534"))
						ui.Text(c, "▾").FontSize(9).TextColor(ui.Hex("#166534"))
					})
				})
			}

			tipText := item.HoverTip
			if tipText == "" {
				tipText = item.Word
			}

			// Role-based highlight glow when legend button is clicked
			isHighlightedRole := (a.filterRole != "" && a.filterRole == item.Role)

			switch item.Role {
			case "connector":
				// Connector: green capsule (like), (so), (that)
				connBox := ui.Box(c).Padding(1, 6).Radius(10).
					Background(ui.Hex("#F0FDF4")).Border(1, ui.Hex("#16A34A")).
					Tooltip("连接词: " + item.CleanWord + " (引导从句并起衔接作用)")
				if connBox.Hovered() {
					a.grammarHoverKnowledge = grammarExplanations["connector"]
				}
				connBox.Children(func() {
					ui.Text(c, item.Word).FontSize(13).FontWeight(600).TextColor(ui.Hex("#15803D"))
				})

			case "predicate":
				// Predicate: deep blue wavy underline + optional obstacle star
				predBg := ui.Transparent
				if isHighlightedRole {
					predBg = ui.Hex("#DBEAFE")
				}
				predBox := ui.Box(c).Padding(0, 2).Radius(3).Background(predBg).Tooltip(tipText)
				if predBox.Hovered() {
					a.grammarHoverKnowledge = grammarExplanations["predicate"]
				}
				predBox.Children(func() {
					ui.Row(c).AlignItems(ui.Center).Children(func() {
						ui.Text(c, item.Word+" ").FontSize(14).FontWeight(700).
							WavyUnderline().DecorationColor(ui.Hex("#2563EB")).TextColor(ui.Hex("#2563EB"))
						if item.Obstacle != "" {
							ui.Text(c, "★"+item.Obstacle).FontSize(9).FontWeight(600).TextColor(ui.Hex("#DC2626"))
						}
					})
				})

			case "subject":
				// Subject: bold solid underline
				subjBg := ui.Transparent
				if isHighlightedRole {
					subjBg = ui.Hex("#FEF08A")
				}
				subjBox := ui.Box(c).Padding(0, 2).Radius(3).Background(subjBg).Tooltip(tipText)
				if subjBox.Hovered() {
					a.grammarHoverKnowledge = grammarExplanations["subject"]
				}
				subjBox.Children(func() {
					ui.Text(c, item.Word+" ").FontSize(14).FontWeight(700).
						Underline().DecorationColor(ui.Hex("#64748B")).TextColor(ui.Hex("#0F172A"))
				})

			case "object":
				// Object: bold solid underline
				objBg := ui.Transparent
				if isHighlightedRole {
					objBg = ui.Hex("#DCFCE7")
				}
				objBox := ui.Box(c).Padding(0, 2).Radius(3).Background(objBg).Tooltip(tipText)
				if objBox.Hovered() {
					a.grammarHoverKnowledge = grammarExplanations["object"]
				}
				objBox.Children(func() {
					ui.Text(c, item.Word+" ").FontSize(14).FontWeight(700).
						Underline().DecorationColor(ui.Hex("#0F172A")).TextColor(ui.Hex("#0F172A"))
				})

			default:
				// Normal word
				ui.Text(c, item.Word+" ").FontSize(14).TextColor(t.Text)
			}
		}
	})
}
