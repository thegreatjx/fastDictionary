package main

import (
	"strings"

	"github.com/egoist/mygo/ui"
)

// AI 结果只用一个按钮表达状态：待处理、处理中、已生效。引擎标题只用强调色，
// 不再另放一个「…已生效」徽标，避免和按钮重复。
func isAIResultEngine(engine string) bool {
	return strings.HasPrefix(engine, "AI ")
}

type aiRefineViewState struct {
	label  string
	tip    string
	active bool
	busy   bool
}

func aiRefineViewFor(s snapshot) aiRefineViewState {
	if s.aiRefining {
		tip := "AI 正在重新翻译，请稍候"
		if s.isScreenshotMode {
			tip = "AI 正在看图核对原文并翻译，请稍候"
		}
		return aiRefineViewState{
			label:  "AI 处理中…",
			tip:    tip,
			active: true,
			busy:   true,
		}
	}
	if isAIResultEngine(s.engine) {
		tip := "当前译文已由 AI 精翻生成 · 点击重新生成"
		if s.isScreenshotMode {
			tip = "当前原文和译文已由 AI 看图精翻生成 · 点击重新生成"
		}
		return aiRefineViewState{
			label:  "已 AI 精翻",
			tip:    tip,
			active: true,
		}
	}
	if s.isScreenshotMode {
		return aiRefineViewState{label: "AI 精翻", tip: "把截图发给 AI 核对原文并翻译，避免 OCR 错字"}
	}
	return aiRefineViewState{label: "AI 精翻", tip: "AI 精翻：术语与长句更准"}
}

// refineCurrentTranslation 是浮窗里唯一的 AI 动作入口。截图直接发图，
// 让模型以像素为准纠正 OCR；划词没有图片，只发文字。
func (a *app) refineCurrentTranslation(s snapshot) {
	if !s.isScreenshotMode {
		a.refineWithAI()
		return
	}
	a.mu.Lock()
	hasImage := a.ocrPath != ""
	a.mu.Unlock()
	if hasImage {
		a.visionRefine()
		return
	}
	a.refineWithAI()
}

func renderAIRefineControls(c *ui.Context, t *ui.Theme, a *app, s snapshot) {
	if s.source == "" || (s.isWord && !s.isScreenshotMode) {
		return
	}
	at := appTheme(t)
	state := aiRefineViewFor(s)
	btn := ui.ButtonBase(c).Tooltip(state.tip).Height(HeightCompact).
		Padding(0, Space2+2).Radius(RadiusSM).Center()
	bg := at.Surface
	border := at.BorderCtrl
	iconCol := at.Accent
	labelCol := at.Text
	if state.active {
		bg = at.AccentSoft
		border = at.AccentRing
		labelCol = at.Accent
	} else if btn.Hovered() {
		bg = at.TintSurface
		border = at.TintBorder
	}
	btn.Background(bg).Border(1, border)
	btn.Children(func() {
		ui.Row(c).Gap(Space1).AlignItems(ui.Center).Children(func() {
			if state.busy {
				ui.Spinner(c).Size(12, 12)
			} else {
				ui.Icon(c, iconSparkles).Size(14, 14).TextColor(iconCol)
			}
			ui.Text(c, state.label).Font(fontUI).FontSize(FontHelper).
				FontWeight(500).TextColor(labelCol).SingleLine()
		})
	})
	if btn.Clicked() && !state.busy {
		a.refineCurrentTranslation(s)
	}
	if s.aiRefineErr != "" {
		ui.Row(c).Gap(Space2).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "精翻失败："+s.aiRefineErr).Font(fontUI).
				FontSize(FontHelper).TextColor(at.Danger).SingleLine()
			if GhostButton(c, t, "重试").Height(HeightIconSm).Radius(RadiusXS).Clicked() {
				a.refineCurrentTranslation(s)
			}
			if GhostButton(c, t, "打开 AI 设置").Height(HeightIconSm).Radius(RadiusXS).Clicked() {
				a.openAISettings()
			}
		})
	}
}

func renderEngineCaptionWithAIState(c *ui.Context, t *ui.Theme, s snapshot, cfg Config) {
	if !cfg.ShowEngineTag || s.engine == "" {
		return
	}
	at := appTheme(t)
	color := at.TextMuted
	if isAIResultEngine(s.engine) || s.aiRefining {
		color = at.Accent
	}
	ui.Text(c, translationEngineCaption(s, cfg.TargetLang)).
		Font(fontUI).FontFeatures("tnum").FontSize(FontMicro).
		TextColor(color).SingleLine()
}
