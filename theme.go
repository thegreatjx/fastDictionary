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
		danger := ui.Hex("#EF4444")
		success := ui.Hex("#22C55E")
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
			AccentSoft:    accent.Alpha(0.09),
			AccentRing:    accent.Alpha(0.45),
			AccentText:    white,
			Danger:        danger,
			DangerHoverBg: danger.Alpha(0.09),
			Success:       success,
			SuccessSoft:   success.Alpha(0.10),
			Warning:       ui.Hex("#F59E0B"),
			ShadowColor:   ui.RGBA(0, 0, 0, 0.35),
			InsetColor:    ui.RGBA(255, 255, 255, 0.14),
		}
	}
	danger := ui.Hex("#DC2626")
	success := ui.Hex("#15803D")
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
		AccentSoft:    accent.Alpha(0.09),
		AccentRing:    accent.Alpha(0.45),
		AccentText:    white,
		Danger:        danger,
		DangerHoverBg: danger.Alpha(0.09),
		Success:       success,
		SuccessSoft:   success.Alpha(0.10),
		Warning:       ui.Hex("#B45309"),
		ShadowColor:   ui.RGBA(15, 23, 42, 0.12),
		InsetColor:    ui.Transparent,
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
	if b.Hovered() {
		b.Background(at.SurfaceHover)
	}
	b.Children(func() {
		ui.Icon(c, icon).Size(IconSize, IconSize).TextColor(at.TextSec)
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
	at := appTheme(c.Theme())
	parts := ui.SegmentedBase(c, selected, len(labels))
	track := parts.Track.Radius(RadiusTrack).Padding(2).Gap(2).
		Background(at.SurfaceSubtle).Border(1, at.BorderSubtle)
	track.Children(func() {
		for i, label := range labels {
			seg := parts.Segment(i).Radius(RadiusSM).
				Padding(Space1, Space3).Height(HeightCompact)
			ink := at.TextSec
			if i == *selected {
				ink = at.Text
				seg.Background(at.SurfaceElev).
					Shadow(0, 1, 2, 0, ui.RGBA(0, 0, 0, 0.12))
				if at.Dark {
					seg.InsetShadow(0, 1, 0, 0, at.InsetColor)
				}
			}
			seg.TextColor(ink)
			seg.Children(func() {
				ui.Text(c, label).Font(fontUI).FontSize(FontControl).
					FontWeight(500).TextColor(ink).SingleLine()
			})
		}
	})
	return track
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

// Notice colours: the "why did the shortcut open this" banner. Amber in
// both themes, because it explains rather than reports an error.
func noticeBg(t *ui.Theme) ui.Color {
	if t.Dark {
		return ui.RGBA(251, 191, 36, 0.14)
	}
	return ui.RGBA(251, 191, 36, 0.12)
}

func noticeBorder(t *ui.Theme) ui.Color {
	if t.Dark {
		return ui.RGBA(251, 191, 36, 0.45)
	}
	return ui.RGBA(217, 119, 6, 0.35)
}

func noticeText(t *ui.Theme) ui.Color {
	if t.Dark {
		return ui.Hex("#FCD34D")
	}
	return ui.Hex("#92400E")
}

// cefrColors tints the CEFR level badge by difficulty band. Both colours come
// from the theme so the badge stays readable in dark mode, where the fixed
// pastels it used to use turned into unreadable pale blocks.
func cefrColors(t *ui.Theme, level string) (bg, fg ui.Color) {
	switch {
	case strings.HasPrefix(level, "C"):
		// Hardest: the danger-ish end of the scale.
		if t.Dark {
			return ui.RGBA(168, 85, 247, 0.22), ui.Hex("#D8B4FE")
		}
		return ui.RGBA(168, 85, 247, 0.12), ui.Hex("#7E22CE")
	case strings.HasPrefix(level, "A"):
		// Easiest: the calm end.
		if t.Dark {
			return ui.RGBA(74, 222, 128, 0.18), ui.Hex("#86EFAC")
		}
		return ui.RGBA(22, 163, 74, 0.10), ui.Hex("#15803D")
	}
	if t.Dark {
		return ui.RGBA(56, 189, 248, 0.18), ui.Hex("#7DD3FC")
	}
	return ui.RGBA(37, 99, 235, 0.10), ui.Hex("#1D4ED8")
}

// memoryTipColors tints the "memory tips" card: amber in light mode, a dim
// amber wash in dark mode, where the light version was a glaring block.
func memoryTipColors(t *ui.Theme) (bg, border, title, body ui.Color) {
	if t.Dark {
		return ui.RGBA(251, 191, 36, 0.12), ui.RGBA(251, 191, 36, 0.35), ui.Hex("#FCD34D"), ui.Hex("#FDE68A")
	}
	return ui.Hex("#FEFCE8"), ui.Hex("#FEF08A"), ui.Hex("#854D0E"), ui.Hex("#713F12")
}

// saveButtonColor is the amber of the "save to notebook" action, brightened in
// dark mode so it keeps its contrast against the dark surface.
func saveButtonColor(t *ui.Theme) ui.Color {
	if t.Dark {
		return ui.Hex("#CA8A04")
	}
	return ui.Hex("#EAB308")
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
	chipSurfaceLight = "#FBFCFD"
	chipSurfaceDark  = "#1B2027"
	chipBorderLight  = "#E3E8EE"
	chipBorderDark   = "#2E3641"
	chipInkLight     = "#3F4A5A"
	chipInkDark      = "#AEBAC8"
	chipInkHoverL    = "#1E2A38"
	chipInkHoverD    = "#E4EAF1"
	chipAccentLight  = "#4A6E8A"
	chipAccentDark   = "#7FA6C4"
	chipTipBgLight   = "#26303B"
	chipTipBgDark    = "#E8EDF2"
	chipTipFgLight   = "#EAF0F6"
	chipTipFgDark    = "#1A2028"
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
			return ui.Hex(chipAccentDark), ui.RGBA(127, 166, 196, 0.16)
		}
		return ui.Hex(chipInkDark), ui.RGBA(174, 186, 200, 0.12)
	}
	if tone == chipTonePrimary {
		return ui.Hex(chipAccentLight), ui.RGBA(74, 110, 138, 0.10)
	}
	return ui.Hex(chipInkLight), ui.RGBA(63, 74, 90, 0.07)
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
