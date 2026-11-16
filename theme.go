package main

import (
	"strings"

	"github.com/egoist/mygo/ui"
)

// 唯一几何标尺：4px 栅格，整数 DIP。组件文件里不再出现散落的像素数字。
const (
	Space1 float32 = 4
	Space2 float32 = 8
	Space3 float32 = 12
	Space4 float32 = 16
	Space6 float32 = 24
	Space8 float32 = 32

	// 同心圆角：外圆角 = 内圆角 + 内边距。
	RadiusXS     float32 = 4
	RadiusSM     float32 = 6
	RadiusTrack  float32 = 8
	RadiusWindow float32 = 10

	// 双密度控件高度。
	HeightCompact float32 = 28
	HeightControl float32 = 32
	HeightSearch  float32 = 34
	HeightIconSm  float32 = 24
	HeightIconMd  float32 = 28
)

// 字阶。LineHeight 是字号倍数；字重最多 600。
const (
	FontPageTitle float32 = 20
	FontWordHead  float32 = 18
	FontSection   float32 = 14
	FontControl   float32 = 13
	FontHelper    float32 = 12
	FontMicro     float32 = 11
	IconSize      float32 = 16

	// 布局宽度。
	LabelColumn     float32 = 112
	ControlColumn   float32 = 480 // 长说明的阅读上限
	InputWidth      float32 = 240
	ContentMaxWidth float32 = 600
	NavWidth                = 188 // 无类型常量：窗口选项用 int，布局用 float32
	TrayWidth               = 300
	TrayHeight              = 248

	fieldGap      float32 = 16
	fieldRowGap   float32 = 24
	controlGap    float32 = 8
	buttonPadding float32 = 12
)

// 字体栈。Font 接受逗号分隔的候选列表。
const (
	fontUI   = "Segoe UI Variable Text, Segoe UI, Microsoft YaHei UI"
	fontIPA  = "Segoe UI, Lucida Sans Unicode, Arial Unicode MS"
	fontMono = "Cascadia Mono, Consolas"
)

// AppTheme 是项目自己的语义色板。框架控件读 ui.Theme，自定义组件读这里；
// 两者由 applyTheme 一次同步，颜色定义只在本文件出现。
type AppTheme struct {
	Dark bool

	Canvas        ui.Color
	Surface       ui.Color
	SurfaceSubtle ui.Color
	SurfaceElev   ui.Color
	SurfaceHover  ui.Color
	SurfacePress  ui.Color

	Text         ui.Color
	TextSec      ui.Color
	TextMuted    ui.Color
	TextDisabled ui.Color

	BorderSubtle ui.Color
	BorderCtrl   ui.Color
	BorderHover  ui.Color
	BorderWindow ui.Color

	Accent        ui.Color
	AccentHover   ui.Color
	AccentPress   ui.Color
	AccentSoft    ui.Color
	AccentRing    ui.Color
	AccentText    ui.Color
	Danger        ui.Color
	DangerHoverBg ui.Color
	Success       ui.Color
	SuccessSoft   ui.Color
	Warning       ui.Color

	ShadowColor ui.Color
	InsetColor  ui.Color
	TintSurface ui.Color // 清冷淡蓝微底（浮窗原文条 / 轻量强调底）
	TintBorder  ui.Color // 淡蓝微边框
}

func defaultAccent(dark bool) ui.Color {
	if dark {
		return ui.Hex("#9DBBD1")
	}
	return ui.Hex("#34566F")
}

// accentChoice is one user-selectable accent. Every choice is a low-saturation
// ink that already works on the paper surfaces; the theme does not change hue
// anywhere else, so switching accent cannot bring the old green/blue blocks
// back.
type accentChoice struct {
	Value string
	Label string
	Light ui.Color
	Dark  ui.Color
}

var accentChoices = []accentChoice{
	{Value: "slate", Label: "黛青", Light: ui.Hex("#34566F"), Dark: ui.Hex("#9DBBD1")},
	{Value: "indigo", Label: "靛蓝", Light: ui.Hex("#3D4E86"), Dark: ui.Hex("#A7B6E8")},
	{Value: "plum", Label: "紫檀", Light: ui.Hex("#5C3F5E"), Dark: ui.Hex("#C6A9C9")},
	{Value: "graphite", Label: "石墨", Light: ui.Hex("#3A3D42"), Dark: ui.Hex("#C2C6CC")},
}

// accentForTheme resolves a saved value; an empty or unknown value falls back
// to the default slate, so configs written before this setting existed do not
// open with a zero colour.
func accentForTheme(value string, dark bool) ui.Color {
	for _, choice := range accentChoices {
		if choice.Value == value {
			if dark {
				return choice.Dark
			}
			return choice.Light
		}
	}
	return defaultAccent(dark)
}

// newAppTheme 是纸墨系统的唯一定义处。
//
// 规则只有四条：底是中性纸面，字是墨，线是发丝；全局只留一个低饱和黛青
// 作为交互色；成功不占用色相，用图标和文字本身表达，避免绿色块再次出现。
func newAppTheme(dark bool, accent ui.Color) AppTheme {
	white := ui.Hex("#FFFFFF")
	black := ui.Hex("#000000")
	if dark {
		danger := ui.Hex("#D98B80")
		return AppTheme{
			Dark:          true,
			Canvas:        ui.Hex("#121316"),
			Surface:       ui.Hex("#191B1E"),
			SurfaceSubtle: ui.Hex("#222528"),
			SurfaceElev:   ui.Hex("#25282C"),
			SurfaceHover:  ui.RGBA(255, 255, 255, 0.06),
			SurfacePress:  ui.RGBA(255, 255, 255, 0.10),
			Text:          ui.Hex("#E8E9EB"),
			TextSec:       ui.Hex("#A6ABB2"),
			TextMuted:     ui.Hex("#8E949C"),
			TextDisabled:  ui.RGBA(232, 233, 235, 0.34),
			BorderSubtle:  ui.RGBA(255, 255, 255, 0.08),
			BorderCtrl:    ui.RGBA(255, 255, 255, 0.15),
			BorderHover:   ui.RGBA(255, 255, 255, 0.26),
			BorderWindow:  ui.RGBA(255, 255, 255, 0.12),
			Accent:        accent,
			AccentHover:   accent.Mix(white, 0.16),
			AccentPress:   accent.Mix(white, 0.30),
			AccentSoft:    accent.Alpha(0.14),
			AccentRing:    accent.Alpha(0.40),
			AccentText:    ui.Hex("#12181D"),
			Danger:        danger,
			DangerHoverBg: danger.Alpha(0.09),
			Success:       accent,
			SuccessSoft:   accent.Alpha(0.12),
			Warning:       ui.Hex("#D2B078"),
			ShadowColor:   ui.RGBA(0, 0, 0, 0.45),
			InsetColor:    ui.RGBA(255, 255, 255, 0.10),
			TintSurface:   ui.Hex("#1D2023"),
			TintBorder:    ui.RGBA(255, 255, 255, 0.10),
		}
	}
	danger := ui.Hex("#A34A3E")
	return AppTheme{
		Dark:          false,
		Canvas:        ui.Hex("#F2F2EF"),
		Surface:       ui.Hex("#FFFFFF"),
		SurfaceSubtle: ui.Hex("#EAEAE6"),
		SurfaceElev:   ui.Hex("#FFFFFF"),
		SurfaceHover:  ui.RGBA(26, 28, 30, 0.045),
		SurfacePress:  ui.RGBA(26, 28, 30, 0.09),
		Text:          ui.Hex("#1A1C1E"),
		TextSec:       ui.Hex("#555A61"),
		TextMuted:     ui.Hex("#666C74"),
		TextDisabled:  ui.RGBA(26, 28, 30, 0.34),
		BorderSubtle:  ui.RGBA(26, 28, 30, 0.09),
		BorderCtrl:    ui.RGBA(26, 28, 30, 0.16),
		BorderHover:   ui.RGBA(26, 28, 30, 0.28),
		BorderWindow:  ui.RGBA(26, 28, 30, 0.12),
		Accent:        accent,
		AccentHover:   accent.Mix(black, 0.14),
		AccentPress:   accent.Mix(black, 0.26),
		AccentSoft:    accent.Alpha(0.10),
		AccentRing:    accent.Alpha(0.38),
		AccentText:    white,
		Danger:        danger,
		DangerHoverBg: danger.Alpha(0.08),
		Success:       accent,
		SuccessSoft:   accent.Alpha(0.10),
		Warning:       ui.Hex("#8A6A3A"),
		ShadowColor:   ui.RGBA(20, 24, 28, 0.10),
		InsetColor:    ui.Transparent,
		TintSurface:   ui.Hex("#F7F7F4"),
		TintBorder:    ui.RGBA(26, 28, 30, 0.10),
	}
}

// appTheme 从当前框架主题派生完整色板，组件不必再自己判断明暗。
func appTheme(t *ui.Theme) AppTheme {
	return newAppTheme(t.Dark, t.Accent)
}

// applyTheme 同时喂给框架控件（ui.Theme）和自定义组件（appTheme）。
// 强调色固定为纸墨系统的黛青，不再跟随系统强调色：Windows 主题可能是
// 荧光绿或高饱和蓝，一旦接管就会破坏整套中性纸面。
func applyTheme(c *ui.Context, mode string, dark bool, accentTheme string) {
	curr := c.Theme()
	t := *curr
	if mode == "light" {
		dark = false
	} else if mode == "dark" {
		dark = true
	}
	accent := accentForTheme(accentTheme, dark)
	at := newAppTheme(dark, accent)

	t.Dark = at.Dark
	t.Background = at.Canvas
	t.Surface = at.Surface
	t.SurfaceHover = at.SurfaceHover
	t.SurfacePressed = at.SurfacePress
	t.Border = at.BorderCtrl
	t.Text = at.Text
	t.TextMuted = at.TextMuted
	t.Accent = at.Accent
	t.AccentHover = at.AccentHover
	t.AccentPressed = at.AccentPress
	t.AccentText = at.AccentText
	t.Selection = at.AccentSoft
	t.Focus = at.Accent
	t.Danger = at.Danger
	t.Warning = at.Warning
	t.Success = at.Success
	t.Scrollbar = at.BorderCtrl.Alpha(0.6)
	t.Radius = RadiusSM
	t.Spacing = Space1
	t.Font = fontUI
	// 框架把 Field 的说明画成 FontSize-1；13 正好得到 12px helper。
	t.FontSize = FontControl
	// 拖拽浮窗时每一帧都会重画，而 SetTheme 会让整棵树失效重排：色板、圆角、
	// 间距与字号一个字段都没变也走一遍，就是拖拽时的闪烁和卡顿。
	if curr.Dark == t.Dark && curr.Background == t.Background && curr.Surface == t.Surface &&
		curr.SurfaceHover == t.SurfaceHover && curr.SurfacePressed == t.SurfacePressed &&
		curr.Border == t.Border && curr.Text == t.Text && curr.TextMuted == t.TextMuted &&
		curr.Accent == t.Accent && curr.AccentHover == t.AccentHover &&
		curr.AccentPressed == t.AccentPressed && curr.AccentText == t.AccentText &&
		curr.Selection == t.Selection && curr.Focus == t.Focus && curr.Danger == t.Danger &&
		curr.Warning == t.Warning && curr.Success == t.Success && curr.Scrollbar == t.Scrollbar &&
		curr.Radius == t.Radius && curr.Spacing == t.Spacing && curr.Font == t.Font &&
		curr.FontSize == t.FontSize {
		return
	}
	c.SetTheme(&t)
}

// ---- 共享文本 ----

func HelperText(c *ui.Context, t *ui.Theme, text string) ui.Element {
	at := appTheme(t)
	return ui.Text(c, text).Font(fontUI).FontSize(FontHelper).FontWeight(400).
		LineHeight(1.5).TextColor(at.TextMuted).Wrap().MaxWidth(ControlColumn)
}

func PageTitle(c *ui.Context, t *ui.Theme, title, sub string) {
	at := appTheme(t)
	ui.Text(c, title).Font(fontUI).FontSize(FontPageTitle).FontWeight(600).
		LineHeight(1.4).TextColor(at.Text).Balance()
	if sub != "" {
		HelperText(c, t, sub)
	}
}

func SectionTitle(c *ui.Context, t *ui.Theme, text string) ui.Element {
	at := appTheme(t)
	return ui.Text(c, text).Font(fontUI).FontSize(FontSection).FontWeight(600).
		LineHeight(1.45).TextColor(at.Text)
}

func KeycapBadge(c *ui.Context, t *ui.Theme, text string) ui.Element {
	at := appTheme(t)
	badge := ui.Box(c).Radius(RadiusXS).Padding(2, 6).
		Background(at.SurfaceSubtle).Border(1, at.BorderCtrl)
	badge.Children(func() {
		ui.Text(c, text).Font(fontMono).FontFeatures("tnum").
			FontSize(FontMicro).FontWeight(500).LineHeight(1.3).
			TextColor(at.TextSec).SingleLine()
	})
	return badge
}

// ---- 四级按钮 ----
// 全部基于 ui.ButtonBase：保留 Focusable 与框架焦点环，不用容器 Opacity。
// 不挂 ElementTransition{Colors:true}：设置页切页和托盘开合都会销毁重建
// 按钮，按 md 的提醒，挂了会在首帧闪一下强调色。

func buttonLabel(c *ui.Context, at AppTheme, label string, color ui.Color) {
	ui.Text(c, label).Font(fontUI).FontSize(FontControl).FontWeight(500).
		TextColor(color).SingleLine()
}

func PrimaryButton(c *ui.Context, t *ui.Theme, label string) ui.Element {
	at := appTheme(t)
	b := ui.ButtonBase(c).Height(HeightControl).Radius(RadiusSM).
		Padding(0, Space3).Gap(Space2).Center().
		Background(at.Accent).BorderWidth(0).TextColor(at.AccentText)
	if b.Pressed() {
		b.Background(at.AccentPress)
	} else if b.Hovered() {
		b.Background(at.AccentHover)
	}
	b.Children(func() { buttonLabel(c, at, label, at.AccentText) })
	return b
}

func SecondaryButton(c *ui.Context, t *ui.Theme, label string, icon ...*ui.SVG) ui.Element {
	at := appTheme(t)
	b := ui.ButtonBase(c).Height(HeightControl).Radius(RadiusSM).
		Padding(0, Space3).Gap(Space2).Center().
		Background(at.Surface).Border(1, at.BorderCtrl).TextColor(at.Text)
	withIcon := len(icon) > 0 && icon[0] != nil
	if withIcon {
		// 左图标按钮的光学修正：左内边距比右内边距少 2DIP。
		b.PaddingStart(Space3 - 2).PaddingEnd(Space3)
	}
	if b.Pressed() {
		b.Background(at.SurfacePress)
	} else if b.Hovered() {
		b.Background(at.SurfaceHover).Border(1, at.BorderHover)
	}
	b.Children(func() {
		if withIcon {
			ui.Icon(c, icon[0]).Size(IconSize, IconSize).TextColor(at.Text)
		}
		buttonLabel(c, at, label, at.Text)
	})
	return b
}

func GhostButton(c *ui.Context, t *ui.Theme, label string) ui.Element {
	at := appTheme(t)
	b := ui.ButtonBase(c).Height(HeightControl).Radius(RadiusSM).
		Padding(0, Space2).Gap(Space2).Center().
		Background(ui.Transparent).BorderWidth(0).TextColor(at.TextSec)
	if b.Hovered() {
		b.Background(at.SurfaceHover)
	}
	if label != "" {
		b.Children(func() { buttonLabel(c, at, label, at.TextSec) })
	}
	return b
}

func DangerGhostButton(c *ui.Context, t *ui.Theme, label string) ui.Element {
	at := appTheme(t)
	fg := at.TextMuted
	bg := ui.Transparent
	b := ui.ButtonBase(c).Height(HeightCompact).Radius(RadiusSM).
		Padding(0, Space3).Gap(Space2).Center().BorderWidth(0)
	hovered := b.Hovered()
	if hovered {
		fg, bg = at.Danger, at.DangerHoverBg
	}
	b.Background(bg).TextColor(fg)
	b.Children(func() { buttonLabel(c, at, label, fg) })
	return b
}

func IconButton(c *ui.Context, t *ui.Theme, icon *ui.SVG, tip string) ui.Element {
	at := appTheme(t)
	b := ui.ButtonBase(c).Height(HeightIconMd).Width(HeightIconMd).
		Radius(RadiusSM).Center().Background(ui.Transparent).BorderWidth(0).
		Tooltip(tip)
	fg := at.TextSec
	if b.Hovered() {
		b.Background(at.SurfaceHover)
		fg = at.Text
	}
	if b.Pressed() {
		b.Background(at.SurfacePress)
		fg = at.Text
	}
	b.Children(func() {
		ui.Icon(c, icon).Size(IconSize, IconSize).TextColor(fg)
	})
	return b
}

// ---- 设置页组件 ----

func SurfaceCard(c *ui.Context, t *ui.Theme) ui.Element {
	at := appTheme(t)
	return ui.Box(c).FillWidth().Padding(Space3, Space4).Radius(RadiusTrack).
		Background(at.Surface).Border(1, at.BorderSubtle)
}

func SettingCard(c *ui.Context, t *ui.Theme, title string, fn func()) {
	if title != "" {
		SectionTitle(c, t, title)
	}
	SurfaceCard(c, t).Padding(0, 0).Children(func() {
		ui.Column(c).FillWidth().Children(fn)
	})
}

func SettingRow(c *ui.Context, t *ui.Theme, label, desc string, control func()) {
	at := appTheme(t)
	ui.Box(c).FillWidth().Padding(Space4, Space4).
		BorderWidth(0, 0, 1, 0).BorderColor(at.BorderSubtle).Children(func() {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(Space4).Children(func() {
			ui.Column(c).Grow(1).Gap(Space1).Children(func() {
				ui.Text(c, label).Font(fontUI).FontSize(FontControl).FontWeight(500).
					TextColor(at.Text)
				if desc != "" {
					HelperText(c, t, desc).MaxWidth(320)
				}
			})
			control()
		})
	})
}

// SettingSliderRow 是设置页滑块的统一行：左侧标签与说明，右侧滑块 + 等宽数值。
func SettingSliderRow(c *ui.Context, t *ui.Theme, label, desc, valueText string, buildSlider func()) {
	at := appTheme(t)
	ui.Box(c).FillWidth().Padding(Space4, Space4).
		BorderWidth(0, 0, 1, 0).BorderColor(at.BorderSubtle).Children(func() {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(Space4).Children(func() {
			ui.Column(c).Grow(1).Gap(Space1).Children(func() {
				ui.Text(c, label).Font(fontUI).FontSize(FontControl).FontWeight(500).TextColor(at.Text)
				if desc != "" {
					HelperText(c, t, desc).MaxWidth(320)
				}
			})
			ui.Row(c).AlignItems(ui.Center).Gap(Space3).Shrink(0).Children(func() {
				buildSlider()
				ui.Box(c).Width(52).AlignItems(ui.End).Children(func() {
					ui.Text(c, valueText).Font(fontMono).FontFeatures("tnum").
						FontSize(FontHelper).FontWeight(500).TextColor(at.TextSec).SingleLine()
				})
			})
		})
	})
}

// LongField 是脱离 ui.Form 的独立字段：标签在控件正上方左对齐。
func LongField(c *ui.Context, t *ui.Theme, label, desc string, control func() ui.Element) {
	at := appTheme(t)
	ui.Box(c).FillWidth().Padding(Space4).BorderWidth(0, 0, 1, 0).
		BorderColor(at.BorderSubtle).Children(func() {
		ui.Column(c).FillWidth().MaxWidth(ContentMaxWidth).Gap(Space2).Children(func() {
			labelEl := ui.Text(c, label).Font(fontUI).FontSize(FontControl).
				FontWeight(500).TextColor(at.TextSec)
			ctrl := control()
			var captionEl ui.Element
			if desc != "" {
				captionEl = HelperText(c, t, desc)
			}
			ctrl.LabelledBy(labelEl)
			if captionEl.Valid() {
				ctrl.DescribedBy(captionEl)
			}
		})
	})
}

// SegmentedControl 基于 ui.SegmentedBase，宽度随选项内容自适应。
func SegmentedControl(c *ui.Context, selected *int, labels ...string) ui.Element {
	return segmentedControlWithHeight(c, selected, HeightControl, labels...)
}

// SegmentedControlCompact 用于托盘与浮窗 Tabs 的紧凑态。
func SegmentedControlCompact(c *ui.Context, selected *int, labels ...string) ui.Element {
	return segmentedControlWithHeight(c, selected, HeightCompact, labels...)
}

func segmentedControlWithHeight(c *ui.Context, selected *int, height float32, labels ...string) ui.Element {
	at := appTheme(c.Theme())
	pad := float32(3)
	if height <= HeightCompact {
		pad = 2
	}
	// 精确扣除上下 1px Border 与上下 Padding，滑块不再压住底边框。
	segH := height - 2 - pad*2
	parts := ui.SegmentedBase(c, selected, len(labels))
	track := parts.Track.Height(height).Radius(RadiusTrack).Padding(pad).Gap(2).
		AlignItems(ui.Center).AlignSelf(ui.Start).Shrink(0).
		Background(at.SurfaceSubtle).Border(1, at.BorderSubtle)
	track.Children(func() {
		for i, label := range labels {
			seg := parts.Segment(i).Height(segH).Radius(RadiusSM).
				Padding(0, Space3).Center().Shrink(0)
			if height <= HeightCompact {
				seg.Padding(0, Space2+2)
			}
			ink := at.TextSec
			weight := 500
			if i == *selected {
				ink = at.Text
				weight = 600
				seg.Background(at.SurfaceElev).Border(1, at.BorderSubtle).
					Shadow(0, 1, 1, 0, ui.RGBA(15, 23, 42, 0.06))
				if at.Dark {
					seg.InsetShadow(0, 1, 0, 0, at.InsetColor)
				}
			}
			seg.TextColor(ink)
			seg.Children(func() {
				ui.Text(c, label).Font(fontUI).FontSize(FontControl).
					FontWeight(weight).TextColor(ink).SingleLine()
			})
		}
	})
	return track
}

// SegmentedOption 与 SegmentedControlMapped 用显式值映射代替魔法下标 0/1/2。
type SegmentedOption[T comparable] struct {
	Value T
	Label string
}

func SegmentedControlMapped[T comparable](c *ui.Context, compact bool, current T, onSelect func(T), opts ...SegmentedOption[T]) {
	idx := 0
	labels := make([]string, len(opts))
	for i, opt := range opts {
		labels[i] = opt.Label
		if opt.Value == current {
			idx = i
		}
	}
	var seg ui.Element
	if compact {
		seg = SegmentedControlCompact(c, &idx, labels...)
	} else {
		seg = SegmentedControl(c, &idx, labels...)
	}
	seg.OnChange(func() {
		if idx >= 0 && idx < len(opts) {
			onSelect(opts[idx].Value)
		}
	})
}

// ---- 过渡辅助（P1 推广完设置页后删除）----

func field(c *ui.Context, t *ui.Theme, label, desc string, build func() ui.Element) {
	at := appTheme(t)
	ui.Row(c).FillWidth().Gap(fieldGap).AlignItems(ui.Start).Children(func() {
		labelEl := ui.Text(c, label).Font(fontUI).FontSize(FontSection).
			FontWeight(600).TextColor(at.Text).Width(LabelColumn).Wrap()
		var captionEl ui.Element
		ui.Column(c).FillWidth().MaxWidth(ControlColumn).Gap(Space2).Children(func() {
			ctrl := build()
			if desc != "" {
				captionEl = HelperText(c, t, desc)
			}
			ctrl.LabelledBy(labelEl)
			if captionEl.Valid() {
				ctrl.DescribedBy(captionEl)
			}
		})
	})
}

func fieldGroup(c *ui.Context, fn func()) {
	ui.Column(c).FillWidth().Gap(fieldRowGap).Children(fn)
}

// 兼容旧组件名的颜色派生，全部走 AppTheme。
func accentSoft(t *ui.Theme) ui.Color { return appTheme(t).AccentSoft }
func accentLine(t *ui.Theme) ui.Color { return appTheme(t).AccentRing }
func dangerSoft(t *ui.Theme) ui.Color { return appTheme(t).DangerHoverBg }
func dangerLine(t *ui.Theme) ui.Color { return appTheme(t).Danger.Alpha(0.35) }

// inkRuleWidth is the 2px accent rule marking the hero translation.
const inkRuleWidth float32 = 3

// Notice colours: the "why did the shortcut open this" banner. It explains
// rather than reports an error, so it uses the plain paper tint and neutral
// ink instead of spending a warning hue.
func noticeBg(t *ui.Theme) ui.Color {
	return appTheme(t).TintSurface
}

func noticeBorder(t *ui.Theme) ui.Color {
	return appTheme(t).TintBorder
}

func noticeText(t *ui.Theme) ui.Color {
	return appTheme(t).TextSec
}

// cefrColors gives the CEFR badge one neutral surface and encodes difficulty
// by ink weight, not hue. The letter already says A1/B2/C1; a second colour
// code on top of it was decoration, and the old green/violet pair was the
// brightest thing in the card.
func cefrColors(t *ui.Theme, level string) (bg, fg ui.Color) {
	at := appTheme(t)
	fg = at.TextMuted
	switch {
	case strings.HasPrefix(level, "C"):
		fg = at.Text
	case strings.HasPrefix(level, "B"):
		fg = at.TextSec
	}
	return at.SurfaceSubtle, fg
}

// memoryTipColors tints the "memory tips" card. Guidance is not a warning, so
// it sits on the same neutral paper tint as the source strip; the heading's
// icon carries the only emphasis.
func memoryTipColors(t *ui.Theme) (bg, border, title, body ui.Color) {
	at := appTheme(t)
	return at.TintSurface, at.TintBorder, at.TextSec, at.Text
}

// The save action is the one commit in a card, so it gets a neutral ink fill
// instead of a semantic hue. In light mode it is near-black; in dark mode it
// inverts to near-white. Both carry their own foreground, so the label never
// depends on the system accent's brightness.
func saveButtonColors(t *ui.Theme) (bg, fg, hover, press ui.Color) {
	if t != nil && t.Dark {
		return ui.Hex("#E8E9EB"), ui.Hex("#121316"), ui.Hex("#F4F4F1"), ui.Hex("#D3D5D8")
	}
	return ui.Hex("#1A1C1E"), ui.Hex("#FFFFFF"), ui.Hex("#2C2F33"), ui.Hex("#0F1113")
}

// saveButtonStyle paints the button and returns the foreground its icon and
// label must use.
func saveButtonStyle(b ui.Element, t *ui.Theme) ui.Color {
	bg, fg, hover, press := saveButtonColors(t)
	switch {
	case b.Pressed():
		bg = press
	case b.Hovered():
		bg = hover
	}
	b.Background(bg).TextColor(fg)
	return fg
}

// saveButtonColor is the ink used by the writing card's tone table.
func saveButtonColor(t *ui.Theme) ui.Color {
	bg, _, _, _ := saveButtonColors(t)
	return bg
}

func saveButtonTextColor(t *ui.Theme) ui.Color {
	_, fg, _, _ := saveButtonColors(t)
	return fg
}

// chipTone names what one selection-chip button does. The tone picks one of
// two inks, not one of seven hues.
//
// The chip used to colour every action differently: blue, teal, violet,
// amber, green, magenta in one 36px row. Six saturated hues side by side read
// as a toy, and they fought the quiet grey the rest of the app is built from.
// Colour is now spent once — on the primary action — and the other buttons
// share a cool slate ink that differs from its neighbours by glyph and by
// position, which is how a real toolbar separates its tools.
type chipTone int

const (
	// chipTonePrimary marks the chip's main action (translate). It is the one
	// button allowed an accent colour, so the row has a single place to look
	// first.
	chipTonePrimary chipTone = iota
	// chipToneInk is every other action: speech, screenshot, the AI actions
	// and the utility menu.
	chipToneInk
)

// The chip's palette. A floating tool should look like a piece of paper laid
// on the page: warm-neutral white, a hairline, and two inks. The old icy blue
// surface fought every page it floated over, and the blue primary made the
// whole bar read as a web widget instead of a desktop tool.
const (
	chipSurfaceLight = "#FCFCFA"
	chipSurfaceDark  = "#1C1D1F"
	chipBorderLight  = "#E2E2DD"
	chipBorderDark   = "#2E3136"
	chipInkLight     = "#4B5057"
	chipInkDark      = "#B4B9C0"
	chipInkHoverL    = "#1A1C1E"
	chipInkHoverD    = "#E8E9EB"
	chipTipBgLight   = "#1B1D20"
	chipTipBgDark    = "#EDEEE9"
	chipTipFgLight   = "#F4F4F1"
	chipTipFgDark    = "#1A1C1E"
)

// chipSurface is the pill the buttons sit on.
func chipSurface(t *ui.Theme) ui.Color {
	if t.Dark {
		return ui.Hex(chipSurfaceDark)
	}
	return ui.Hex(chipSurfaceLight)
}

// chipBorderColor is the hairline around the pill and the rule that separates
// the tools from the menu.
func chipBorderColor(t *ui.Theme) ui.Color {
	if t.Dark {
		return ui.Hex(chipBorderDark)
	}
	return ui.Hex(chipBorderLight)
}

// chipShadows 返回贴合阴影与扩散提亮阴影。颜色留在 theme.go，
// 悬浮条渲染不再写散落色值。
func chipShadows(t *ui.Theme) (contact, lift ui.Color) {
	if t.Dark {
		return ui.RGBA(0, 0, 0, 0.30), ui.RGBA(0, 0, 0, 0.42)
	}
	return ui.RGBA(20, 24, 28, 0.06), ui.RGBA(20, 24, 28, 0.10)
}

// chipToneColors returns the icon colour of a chip action and the wash its
// button takes on hover.
//
// Both inks sit at or above the 3:1 non-text contrast floor against
// chipSurface in both themes, which is what keeps a glyph legible when colour
// is no longer carrying the meaning. The wash is the same slate for every
// button: a hover that changes hue per button was what made the row look like
// six unrelated tools.
func chipToneColors(t *ui.Theme, tone chipTone) (fg, wash ui.Color) {
	if tone == chipTonePrimary {
		if t.Dark {
			return t.Accent, t.Accent.Alpha(0.16)
		}
		return t.Accent, t.Accent.Alpha(0.10)
	}
	if t.Dark {
		return ui.Hex(chipInkDark), ui.RGBA(255, 255, 255, 0.08)
	}
	return ui.Hex(chipInkLight), ui.RGBA(26, 28, 30, 0.06)
}

// chipIconHover is the icon colour while the pointer is on the button: the ink
// deepens instead of turning into another hue, so hover reads as emphasis and
// not as a change of identity.
func chipIconHover(t *ui.Theme) ui.Color {
	if t.Dark {
		return ui.Hex(chipInkHoverD)
	}
	return ui.Hex(chipInkHoverL)
}

// chipTipColors returns the fill and text of the hover label.
//
// It is a dark slate plate in the light theme and a pale one in the dark theme
// — inverted from the chip, as a tooltip should be, but drawn from the same
// cool family rather than from the theme's neutral Inverse, which is nearly
// black and reads as a different product.
func chipTipColors(t *ui.Theme) (bg, fg ui.Color) {
	if t.Dark {
		return ui.Hex(chipTipBgDark), ui.Hex(chipTipFgDark)
	}
	return ui.Hex(chipTipBgLight), ui.Hex(chipTipFgLight)
}

// ---- 内容语义色板：从 grammar.go / popup_view.go 收口到 theme.go ----

// grammarInkFor 是长难句标注的明暗色板。颜色只在这里定义。
//
// This card is the one place where hue carries information: six syntactic
// roles must be separable at a glance, so these inks stay moderately saturated
// while every surrounding surface is neutral paper. Line shape (solid, wave,
// double, dotted) carries the second channel, so no role depends on hue alone.
func grammarInkFor(t *ui.Theme) grammarInk {
	if t != nil && t.Dark {
		return grammarInk{
			subjectLine:    ui.Hex("#6FB3BF"),
			predicateLine:  ui.Hex("#8AAEFF"),
			objectLine:     ui.Hex("#C98BAE"),
			complementLine: ui.Hex("#C3A0E8"),
			connector:      ui.Hex("#A8C79A"),
			clauseTint:     ui.RGBA(168, 199, 154, 0.10),
			clauseTint2:    ui.RGBA(168, 199, 154, 0.16),
			clauseText:     ui.Hex("#A6ABB2"),
			obstacle:       ui.Hex("#E5A171"),
			obstacleBg:     ui.RGBA(229, 161, 113, 0.12),
			body:           ui.Hex("#E8E9EB"),
			muted:          ui.Hex("#8E949C"),
		}
	}
	return grammarInk{
		subjectLine:    ui.Hex("#2F6F7A"),
		predicateLine:  ui.Hex("#2A57C4"),
		objectLine:     ui.Hex("#8A4B6B"),
		complementLine: ui.Hex("#6F4AA8"),
		connector:      ui.Hex("#4C7A3F"),
		clauseTint:     ui.RGBA(76, 122, 63, 0.06),
		clauseTint2:    ui.RGBA(76, 122, 63, 0.10),
		clauseText:     ui.Hex("#555A61"),
		obstacle:       ui.Hex("#B05A3C"),
		obstacleBg:     ui.RGBA(176, 90, 60, 0.08),
		body:           ui.Hex("#1A1C1E"),
		muted:          ui.Hex("#666C74"),
	}
}

// grammarHeaderColors 是长难句卡片顶部「拆解」标签的底、边、字。
func grammarHeaderColors(t *ui.Theme) (bg, border, text ui.Color) {
	at := appTheme(t)
	return at.SurfaceSubtle, at.BorderSubtle, at.TextSec
}

// grammarObstacleColors 是「句式难点」卡片的明暗色板。It used to be a warm
// brown card; the surface is now neutral and only the title/badge keep a muted
// clay accent, so it no longer reads as a second warning panel.
func grammarObstacleColors(t *ui.Theme, ink grammarInk) (cardBg, cardBorder, cardTitle, badgeBg, badgeBorder, badgeText, obsText, restoreLabel, restoreText, noteLabel, noteText ui.Color) {
	if t != nil && t.Dark {
		return ui.Hex("#1D2023"),
			ui.Hex("#2E3136"),
			ink.obstacle,
			ink.obstacleBg,
			withAlpha(ink.obstacle, 0.32),
			ink.obstacle,
			ink.body,
			ink.muted,
			ink.subjectLine,
			ink.muted,
			ink.body
	}
	return ui.Hex("#F7F7F4"), ui.Hex("#E6E6E1"), ui.Hex("#8A5145"),
		ui.Hex("#EAEAE6"), ui.Hex("#DCDCD6"), ui.Hex("#8A5145"),
		ui.Hex("#1A1C1E"), ui.Hex("#666C74"), ui.Hex("#34566F"),
		ui.Hex("#666C74"), ui.Hex("#3A3E44")
}

// grammarToggleOnColors 是长难句卡片开关的选中态。
func grammarToggleOnColors(t *ui.Theme) (bg, text, border ui.Color) {
	at := appTheme(t)
	return at.AccentSoft, at.Accent, at.AccentRing
}
