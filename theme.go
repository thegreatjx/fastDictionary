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
		return ui.Hex("#3B82F6")
	}
	return ui.Hex("#2563EB")
}

// newAppTheme 是明暗两套色板的唯一定义处。
func newAppTheme(dark bool, accent ui.Color) AppTheme {
	white := ui.Hex("#FFFFFF")
	if dark {
		danger := ui.Hex("#E39A93")
		success := ui.Hex("#9BC7B5")
		return AppTheme{
			Dark:          true,
			Canvas:        ui.Hex("#111317"),
			Surface:       ui.Hex("#181B21"),
			SurfaceSubtle: ui.Hex("#22262E"),
			SurfaceElev:   ui.Hex("#262B35"),
			SurfaceHover:  ui.RGBA(255, 255, 255, 0.06),
			SurfacePress:  ui.RGBA(255, 255, 255, 0.10),
			Text:          ui.Hex("#F1F5F9"),
			TextSec:       ui.Hex("#94A3B8"),
			TextMuted:     ui.Hex("#64748B"),
			TextDisabled:  ui.RGBA(241, 245, 249, 0.35),
			BorderSubtle:  ui.RGBA(255, 255, 255, 0.08),
			BorderCtrl:    ui.RGBA(255, 255, 255, 0.14),
			BorderHover:   ui.RGBA(255, 255, 255, 0.24),
			BorderWindow:  ui.RGBA(255, 255, 255, 0.12),
			Accent:        accent,
			AccentHover:   ui.Hex("#2563EB"),
			AccentPress:   ui.Hex("#1D4ED8"),
			AccentSoft:    accent.Alpha(0.16),
			AccentRing:    accent.Alpha(0.45),
			AccentText:    white,
			Danger:        danger,
			DangerHoverBg: danger.Alpha(0.09),
			Success:       success,
			SuccessSoft:   success.Alpha(0.10),
			Warning:       ui.Hex("#D6B07A"),
			ShadowColor:   ui.RGBA(0, 0, 0, 0.35),
			InsetColor:    ui.RGBA(255, 255, 255, 0.14),
			TintSurface:   ui.Hex("#171D28"),
			TintBorder:    ui.RGBA(96, 165, 250, 0.20),
		}
	}
	danger := ui.Hex("#A33A34")
	success := ui.Hex("#2F6F5E")
	return AppTheme{
		Dark:          false,
		Canvas:        ui.Hex("#F5F7FA"),
		Surface:       ui.Hex("#FFFFFF"),
		SurfaceSubtle: ui.Hex("#EEF2F6"),
		SurfaceElev:   ui.Hex("#FFFFFF"),
		SurfaceHover:  ui.RGBA(15, 23, 42, 0.045),
		SurfacePress:  ui.RGBA(15, 23, 42, 0.08),
		Text:          ui.Hex("#0F172A"),
		TextSec:       ui.Hex("#475569"),
		TextMuted:     ui.Hex("#64748B"),
		TextDisabled:  ui.RGBA(15, 23, 42, 0.36),
		BorderSubtle:  ui.RGBA(15, 23, 42, 0.08),
		BorderCtrl:    ui.RGBA(15, 23, 42, 0.14),
		BorderHover:   ui.RGBA(15, 23, 42, 0.24),
		BorderWindow:  ui.RGBA(15, 23, 42, 0.10),
		Accent:        accent,
		AccentHover:   ui.Hex("#1D4ED8"),
		AccentPress:   ui.Hex("#1E40AF"),
		AccentSoft:    accent.Alpha(0.11),
		AccentRing:    accent.Alpha(0.45),
		AccentText:    white,
		Danger:        danger,
		DangerHoverBg: danger.Alpha(0.09),
		Success:       success,
		SuccessSoft:   success.Alpha(0.10),
		Warning:       ui.Hex("#8A6435"),
		ShadowColor:   ui.RGBA(15, 23, 42, 0.12),
		InsetColor:    ui.Transparent,
		TintSurface:   ui.Hex("#F3F7FF"),
		TintBorder:    ui.RGBA(37, 99, 235, 0.16),
	}
}

// appTheme 从当前框架主题派生完整色板，组件不必再自己判断明暗。
func appTheme(t *ui.Theme) AppTheme {
	return newAppTheme(t.Dark, t.Accent)
}

// applyTheme 同时喂给框架控件（ui.Theme）和自定义组件（appTheme）。
// 系统 accent 仍优先，保证 Windows 上跟随用户主题色。
func applyTheme(c *ui.Context, mode string, dark bool) {
	curr := c.Theme()
	t := *curr
	if mode == "light" {
		dark = false
	} else if mode == "dark" {
		dark = true
	}
	sysAccent := curr.Accent
	hasSysAccent := sysAccent.A > 0 && !(sysAccent.R == 0 && sysAccent.G == 0 && sysAccent.B == 0)
	accent := defaultAccent(dark)
	if hasSysAccent {
		accent = sysAccent
	}
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
// rather than reports an error, so it borrows the cool tint the source strip
// uses instead of spending a second warning hue.
func noticeBg(t *ui.Theme) ui.Color {
	return appTheme(t).TintSurface
}

func noticeBorder(t *ui.Theme) ui.Color {
	return appTheme(t).TintBorder
}

func noticeText(t *ui.Theme) ui.Color {
	return appTheme(t).TextSec
}

// cefrColors tints the CEFR level badge by difficulty band. Both colours come
// from the theme so the badge stays readable in dark mode, where the fixed
// pastels it used to use turned into unreadable pale blocks.
func cefrColors(t *ui.Theme, level string) (bg, fg ui.Color) {
	at := appTheme(t)
	switch {
	case strings.HasPrefix(level, "C"):
		// Hardest: a dusty violet, not a warning colour.
		if t.Dark {
			return ui.RGBA(179, 167, 230, 0.16), ui.Hex("#C3B2E8")
		}
		return ui.RGBA(122, 78, 158, 0.10), ui.Hex("#6B4A8A")
	case strings.HasPrefix(level, "A"):
		// Easiest: a quiet pine, the same family as the success ink.
		if t.Dark {
			return at.Success.Alpha(0.14), at.Success
		}
		return at.Success.Alpha(0.08), at.Success
	}
	if t.Dark {
		return at.Accent.Alpha(0.16), ui.Hex("#93B4F5")
	}
	return at.Accent.Alpha(0.09), at.Accent
}

// memoryTipColors tints the "memory tips" card. It used to be amber in light
// mode; the note is guidance, not a warning, so it now uses the same cool tint
// as the source strip and leaves colour to the heading's icon.
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
		return ui.Hex("#E2E8F0"), ui.Hex("#0F172A"), ui.Hex("#F1F5F9"), ui.Hex("#CBD5E1")
	}
	return ui.Hex("#1E293B"), ui.Hex("#FFFFFF"), ui.Hex("#111827"), ui.Hex("#0B1220")
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

// The chip's palette. One cool, low-saturation family: a blue-grey slate for
// the ink and a single steel blue for the primary action. The surface is not
// pure white or pure black — a trace of blue in it keeps the bar from
// looking pasted on top of the page.
const (
	chipSurfaceLight = "#F4F8FF" // 95% 白 + 5% 冰川蓝：通透不艳俗
	chipSurfaceDark  = "#171D28"
	chipBorderLight  = "#D4E2FA" // 极细冰蓝框，不再灰蒙蒙
	chipBorderDark   = "#2B394F"
	chipInkLight     = "#3A4960"
	chipInkDark      = "#B6C5DA"
	chipInkHoverL    = "#1D4ED8"
	chipInkHoverD    = "#E2ECFA"
	chipAccentLight  = "#2563EB" // 主图标恢复明快系统蓝
	chipAccentDark   = "#60A5FA"
	chipTipBgLight   = "#1E293B"
	chipTipBgDark    = "#E8EDF5"
	chipTipFgLight   = "#F1F5F9"
	chipTipFgDark    = "#0F172A"
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
		return ui.RGBA(0, 0, 0, 0.28), ui.RGBA(0, 0, 0, 0.40)
	}
	return ui.RGBA(15, 23, 42, 0.06), ui.RGBA(30, 58, 138, 0.10)
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
	if t.Dark {
		if tone == chipTonePrimary {
			return ui.Hex(chipAccentDark), ui.RGBA(96, 165, 250, 0.16)
		}
		return ui.Hex(chipInkDark), ui.RGBA(96, 165, 250, 0.12)
	}
	if tone == chipTonePrimary {
		return ui.Hex(chipAccentLight), ui.RGBA(37, 99, 235, 0.11)
	}
	return ui.Hex(chipInkLight), ui.RGBA(37, 99, 235, 0.08)
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
// The roles still need to be told apart at a glance, but the old palette spent
// eight saturated hues and turned the sentence into sticky notes. This one is
// an editorial family: muted teal, blue, plum, violet, olive and clay.
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
			clauseText:     ui.Hex("#94A3B8"),
			obstacle:       ui.Hex("#E5A171"),
			obstacleBg:     ui.RGBA(229, 161, 113, 0.12),
			body:           ui.Hex("#E4EAF1"),
			muted:          ui.Hex("#7D8595"),
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
		clauseText:     ui.Hex("#475569"),
		obstacle:       ui.Hex("#B05A3C"),
		obstacleBg:     ui.RGBA(176, 90, 60, 0.08),
		body:           ui.Hex("#1E293B"),
		muted:          ui.Hex("#8C94A3"),
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
		return ui.Hex("#1C222B"),
			ui.Hex("#2B3442"),
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
	return ui.Hex("#F7F8FA"), ui.Hex("#E3E8EF"), ui.Hex("#9A5A44"),
		ui.Hex("#F1F5F9"), ui.Hex("#E2E8F0"), ui.Hex("#9A5A44"),
		ui.Hex("#1E293B"), ui.Hex("#64748B"), ui.Hex("#2F6F7A"),
		ui.Hex("#64748B"), ui.Hex("#334155")
}

// grammarToggleOnColors 是长难句卡片开关的选中态。
func grammarToggleOnColors(t *ui.Theme) (bg, text, border ui.Color) {
	at := appTheme(t)
	return at.AccentSoft, at.Accent, at.AccentRing
}
