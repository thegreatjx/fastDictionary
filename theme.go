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
		return ui.Hex("#60A5FA")
	}
	return ui.Hex("#2563EB")
}

// accentChoice is one user-selectable theme. The soft background and its
// border belong to the choice, so switching themes moves the source strip,
// selected pills and focus borders together instead of only recolouring the
// primary button.
type accentChoice struct {
	Value           string
	Label           string
	Light           ui.Color
	LightHover      ui.Color
	LightPress      ui.Color
	LightSoft       ui.Color
	LightSoftBorder ui.Color
	Dark            ui.Color
	DarkSoft        ui.Color
	DarkSoftBorder  ui.Color
}

var accentChoices = []accentChoice{
	{
		Value: "sky", Label: "晴空蓝",
		Light: ui.Hex("#2563EB"), LightHover: ui.Hex("#1D4ED8"), LightPress: ui.Hex("#1E40AF"),
		LightSoft: ui.Hex("#EFF6FF"), LightSoftBorder: ui.Hex("#BFDBFE"),
		Dark: ui.Hex("#60A5FA"), DarkSoft: ui.Hex("#1B2A4A"), DarkSoftBorder: ui.Hex("#35558A"),
	},
	{
		Value: "mint", Label: "薄荷青",
		Light: ui.Hex("#0D9488"), LightHover: ui.Hex("#0F766E"), LightPress: ui.Hex("#115E59"),
		LightSoft: ui.Hex("#F0FDFA"), LightSoftBorder: ui.Hex("#99F6E4"),
		Dark: ui.Hex("#2DD4BF"), DarkSoft: ui.Hex("#133332"), DarkSoftBorder: ui.Hex("#1F5A54"),
	},
	{
		Value: "violet", Label: "薰衣紫",
		Light: ui.Hex("#7C3AED"), LightHover: ui.Hex("#6D28D9"), LightPress: ui.Hex("#5B21B6"),
		LightSoft: ui.Hex("#F5F3FF"), LightSoftBorder: ui.Hex("#DDD6FE"),
		Dark: ui.Hex("#A78BFA"), DarkSoft: ui.Hex("#262044"), DarkSoftBorder: ui.Hex("#453672"),
	},
	{
		Value: "coral", Label: "暖珊瑚",
		Light: ui.Hex("#EA580C"), LightHover: ui.Hex("#C2410C"), LightPress: ui.Hex("#9A3412"),
		LightSoft: ui.Hex("#FFF7ED"), LightSoftBorder: ui.Hex("#FED7AA"),
		Dark: ui.Hex("#FB923C"), DarkSoft: ui.Hex("#36211A"), DarkSoftBorder: ui.Hex("#6B3B22"),
	},
}

// accentThemeChoice maps a saved value to a palette. Values from the retired
// paper theme are folded into the nearest new hue so old configs keep a valid
// colour instead of opening on a zero value.
func accentThemeChoice(value string) accentChoice {
	switch value {
	case "mint":
		return accentChoices[1]
	case "violet", "plum":
		return accentChoices[2]
	case "coral":
		return accentChoices[3]
	case "sky", "slate", "indigo", "graphite", "":
		return accentChoices[0]
	}
	return accentChoices[0]
}

// accentForTheme resolves a saved value; an empty or unknown value falls back
// to the default sky blue.
func accentForTheme(value string, dark bool) ui.Color {
	choice := accentThemeChoice(value)
	if dark {
		return choice.Dark
	}
	return choice.Light
}

func accentHoverFor(accent ui.Color, dark bool) ui.Color {
	for _, choice := range accentChoices {
		if dark && choice.Dark == accent {
			return accent.Mix(ui.Hex("#FFFFFF"), 0.16)
		}
		if !dark && choice.Light == accent {
			return choice.LightHover
		}
	}
	if dark {
		return accent.Mix(ui.Hex("#FFFFFF"), 0.16)
	}
	return accent.Mix(ui.Hex("#000000"), 0.14)
}

func accentPressFor(accent ui.Color, dark bool) ui.Color {
	for _, choice := range accentChoices {
		if dark && choice.Dark == accent {
			return accent.Mix(ui.Hex("#FFFFFF"), 0.28)
		}
		if !dark && choice.Light == accent {
			return choice.LightPress
		}
	}
	if dark {
		return accent.Mix(ui.Hex("#FFFFFF"), 0.28)
	}
	return accent.Mix(ui.Hex("#000000"), 0.26)
}

// accentSoftColors returns the themed source-strip wash and its soft border.
// Values are explicit per palette so the selected pill, source strip and
// focus border always move together with the accent.
func accentSoftColors(accent ui.Color, dark bool) (soft, border ui.Color) {
	for _, choice := range accentChoices {
		if dark && choice.Dark == accent {
			return choice.DarkSoft, choice.DarkSoftBorder
		}
		if !dark && choice.Light == accent {
			return choice.LightSoft, choice.LightSoftBorder
		}
	}
	if dark {
		return accent.Alpha(0.20), accent.Alpha(0.45)
	}
	return accent.Alpha(0.12), accent.Alpha(0.45)
}

// newAppTheme 是晴空微蓝系统的唯一定义处。
//
// 冷白底、白色卡片、一条淡色氛围底和一个主题色构成整套表面；成功只在
// 瞬时勾选时借用青绿，不出现整块绿色。所有背景与文字色值集中在这里。
func newAppTheme(dark bool, accent ui.Color) AppTheme {
	white := ui.Hex("#FFFFFF")
	soft, ring := accentSoftColors(accent, dark)
	if dark {
		danger := ui.Hex("#F87171")
		return AppTheme{
			Dark:          true,
			Canvas:        ui.Hex("#111622"),
			Surface:       ui.Hex("#192030"),
			SurfaceSubtle: soft,
			SurfaceElev:   ui.Hex("#232C40"),
			SurfaceHover:  ui.Hex("#1E2738"),
			SurfacePress:  ui.Hex("#253047"),
			Text:          ui.Hex("#F1F5F9"),
			TextSec:       ui.Hex("#94A3B8"),
			TextMuted:     ui.Hex("#64748B"),
			TextDisabled:  ui.RGBA(241, 245, 249, 0.34),
			BorderSubtle:  ui.Hex("#263147"),
			BorderCtrl:    ui.Hex("#33415C"),
			BorderHover:   ui.Hex("#4A5C7A"),
			BorderWindow:  ui.Hex("#2B3850"),
			Accent:        accent,
			AccentHover:   accentHoverFor(accent, true),
			AccentPress:   accentPressFor(accent, true),
			AccentSoft:    soft,
			AccentRing:    ring,
			AccentText:    ui.Hex("#0B1220"),
			Danger:        danger,
			DangerHoverBg: danger.Alpha(0.10),
			Success:       ui.Hex("#10B981"),
			SuccessSoft:   ui.Hex("#10B981").Alpha(0.14),
			Warning:       ui.Hex("#FBBF24"),
			ShadowColor:   ui.RGBA(0, 0, 0, 0.48),
			InsetColor:    ui.RGBA(255, 255, 255, 0.10),
			TintSurface:   soft,
			TintBorder:    ring,
		}
	}
	danger := ui.Hex("#DC2626")
	return AppTheme{
		Dark:          false,
		Canvas:        ui.Hex("#F7F9FC"),
		Surface:       ui.Hex("#FFFFFF"),
		SurfaceSubtle: soft,
		SurfaceElev:   ui.Hex("#FFFFFF"),
		SurfaceHover:  ui.Hex("#F1F5F9"),
		SurfacePress:  ui.Hex("#E2E8F0"),
		Text:          ui.Hex("#0F172A"),
		TextSec:       ui.Hex("#475569"),
		TextMuted:     ui.Hex("#64748B"),
		TextDisabled:  ui.RGBA(15, 23, 42, 0.34),
		BorderSubtle:  ui.Hex("#E5EAF2"),
		BorderCtrl:    ui.Hex("#D3DCE8"),
		BorderHover:   ui.Hex("#B8C6D9"),
		BorderWindow:  ui.Hex("#DCE4EF"),
		Accent:        accent,
		AccentHover:   accentHoverFor(accent, false),
		AccentPress:   accentPressFor(accent, false),
		AccentSoft:    soft,
		AccentRing:    ring,
		AccentText:    white,
		Danger:        danger,
		DangerHoverBg: danger.Alpha(0.08),
		Success:       ui.Hex("#10B981"),
		SuccessSoft:   ui.Hex("#10B981").Alpha(0.12),
		Warning:       ui.Hex("#B45309"),
		ShadowColor:   ui.RGBA(15, 23, 42, 0.08),
		InsetColor:    ui.Transparent,
		TintSurface:   soft,
		TintBorder:    ring,
	}
}

// appTheme 从当前框架主题派生完整色板，组件不必再自己判断明暗。
func appTheme(t *ui.Theme) AppTheme {
	return newAppTheme(t.Dark, t.Accent)
}

// applyTheme 同时喂给框架控件（ui.Theme）和自定义组件（appTheme）。
// 强调色固定为晴空微蓝的当前主题色，不再跟随系统强调色：Windows 主题
// 可能是荧光绿或高饱和蓝，一旦接管就会破坏整套冷白表面。
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
		Background(at.SurfaceHover).Border(1, at.BorderSubtle)
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
				ink = at.Accent
				weight = 600
				seg.Background(at.AccentSoft).Border(1, at.AccentRing)
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

// The save action is the one commit in a card, so it takes the theme accent
// and its own foreground. The foreground is dark on the bright dark-theme
// accents and white on the deeper light-theme accents.
func saveButtonColors(t *ui.Theme) (bg, fg, hover, press ui.Color) {
	at := appTheme(t)
	return at.Accent, at.AccentText, at.AccentHover, at.AccentPress
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

// saveButtonColor is the accent used by the writing card's tone table.
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

// The chip's palette. A floating tool sits on the page like a clean white
// card: pure white in light mode, deep sea blue in dark mode, a hairline and
// two inks. Only the primary action carries the theme accent.
const (
	chipSurfaceLight = "#FFFFFF"
	chipSurfaceDark  = "#192030"
	chipBorderLight  = "#E5EAF2"
	chipBorderDark   = "#263147"
	chipInkLight     = "#475569"
	chipInkDark      = "#94A3B8"
	chipInkHoverL    = "#0F172A"
	chipInkHoverD    = "#F1F5F9"
	chipTipBgLight   = "#0F172A"
	chipTipBgDark    = "#F1F5F9"
	chipTipFgLight   = "#F8FAFC"
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
		return ui.RGBA(0, 0, 0, 0.32), ui.RGBA(0, 0, 0, 0.44)
	}
	return ui.RGBA(15, 23, 42, 0.06), ui.RGBA(15, 23, 42, 0.10)
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
	return ui.Hex(chipInkLight), ui.RGBA(15, 23, 42, 0.06)
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

// grammarInkFor 是长难句标注的色板。颜色只在这里定义。
//
// The card no longer spends six hues on six roles. The trunk is bold body ink,
// the predicate is the one element that takes the theme accent and a single
// fine underline, modifiers fall back to muted grey, and clauses are told
// apart by a 10% accent wash plus a micro badge.
func grammarInkFor(t *ui.Theme) grammarInk {
	at := appTheme(t)
	return grammarInk{
		body:          at.Text,
		predicate:     at.Accent,
		predicateLine: at.AccentRing,
		muted:         at.TextMuted,
		connector:     at.TextSec,
		clauseTint:    at.Accent.Alpha(0.10),
		clauseText:    at.TextSec,
		obstacle:      at.Warning,
		obstacleBg:    at.Warning.Alpha(0.10),
	}
}

// grammarHeaderColors 是长难句卡片顶部「拆解」标签的底、边、字。
func grammarHeaderColors(t *ui.Theme) (bg, border, text ui.Color) {
	at := appTheme(t)
	return at.AccentSoft, at.AccentRing, at.Accent
}

// grammarObstacleColors 是「句式难点」卡片的色板：中性卡面，只有标题与
// 徽章借用警告色，不再出现整块陶土底。
func grammarObstacleColors(t *ui.Theme, ink grammarInk) (cardBg, cardBorder, cardTitle, badgeBg, badgeBorder, badgeText, obsText, restoreLabel, restoreText, noteLabel, noteText ui.Color) {
	at := appTheme(t)
	return at.SurfaceHover,
		at.BorderSubtle,
		at.Warning,
		ink.obstacleBg,
		at.Warning.Alpha(0.32),
		at.Warning,
		at.Text,
		at.TextMuted,
		at.TextSec,
		at.TextMuted,
		at.TextSec
}

// grammarToggleOnColors 是长难句卡片开关的选中态。
func grammarToggleOnColors(t *ui.Theme) (bg, text, border ui.Color) {
	at := appTheme(t)
	return at.AccentSoft, at.Accent, at.AccentRing
}
