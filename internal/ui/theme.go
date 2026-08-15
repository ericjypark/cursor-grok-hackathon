package ui

// This file holds the visual language shared by every surface the CLI draws:
// the brand gradient, the palette sampled from it, and the box/rule/meter
// primitives the progress view and the summary both build on.

import (
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/term"
	"github.com/lucasb-eyer/go-colorful"
	"github.com/muesli/termenv"
)

// Layout. The CLI claims the whole terminal: every surface spans the window's
// own width less a margin rail on each side. Nothing is capped — a wide window
// gets a wide frame, which is the point.
const margin = 2

// Fit turns a raw terminal width into the frame width surfaces draw at. There
// is no floor: a frame wider than its window wraps every line it draws, which
// is worse at any size than a cramped one.
func Fit(cols int) int {
	return max(cols-2*margin, 1)
}

// TermSize reports the terminal geometry for surfaces drawn outside bubbletea,
// which never receive a WindowSizeMsg. Falls back to a conventional 80x24.
func TermSize() (int, int) {
	w, h, err := term.GetSize(os.Stdout.Fd())
	if err != nil || w <= 0 {
		return 80, 24
	}
	return w, h
}

// The brand ramp: violet through indigo into cyan. Every accent in the CLI is
// sampled from this one curve, so unrelated surfaces still look related.
var rampHex = []string{"#A78BFA", "#7C5CFF", "#4F7DF9", "#22D3EE"}

var ramp = func() []colorful.Color {
	out := make([]colorful.Color, len(rampHex))
	for i, h := range rampHex {
		c, _ := colorful.Hex(h)
		out[i] = c
	}
	return out
}()

// Palette. Anything not sampled from the ramp is a status color, kept
// desaturated enough to sit beside it without fighting.
var (
	Title  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#F4F4F5"))
	Body   = lipgloss.NewStyle().Foreground(lipgloss.Color("#D4D4D8"))
	Dim    = lipgloss.NewStyle().Foreground(lipgloss.Color("#71717A"))
	Muted  = lipgloss.NewStyle().Foreground(lipgloss.Color("#A1A1AA"))
	Accent = lipgloss.NewStyle().Foreground(lipgloss.Color("#7C5CFF"))
	Good   = lipgloss.NewStyle().Foreground(lipgloss.Color("#34D399"))
	Warn   = lipgloss.NewStyle().Foreground(lipgloss.Color("#FBBF24"))
	Bad    = lipgloss.NewStyle().Foreground(lipgloss.Color("#FB7185"))
)

// hasTrueColor reports whether per-character gradients will actually render.
// On a 256-color or monochrome terminal a gradient degrades into noise, so
// every gradient helper falls back to a flat accent instead.
func hasTrueColor() bool { return lipgloss.ColorProfile() == termenv.TrueColor }

// RampAt samples the gradient at t in [0,1], clamped.
func RampAt(t float64) lipgloss.Color {
	switch {
	case t <= 0:
		return lipgloss.Color(rampHex[0])
	case t >= 1:
		return lipgloss.Color(rampHex[len(rampHex)-1])
	}
	span := 1.0 / float64(len(ramp)-1)
	i := int(t / span)
	if i >= len(ramp)-1 {
		i = len(ramp) - 2
	}
	local := (t - float64(i)*span) / span
	return lipgloss.Color(ramp[i].BlendLuv(ramp[i+1], local).Clamped().Hex())
}

// Gradient paints s across the full ramp, one step per rune.
func Gradient(s string, bold bool) string {
	base := lipgloss.NewStyle().Bold(bold)
	if !hasTrueColor() {
		return base.Foreground(lipgloss.Color("#7C5CFF")).Render(s)
	}
	runes := []rune(s)
	if len(runes) == 0 {
		return ""
	}
	var sb strings.Builder
	den := float64(len(runes) - 1)
	for i, r := range runes {
		t := 0.0
		if den > 0 {
			t = float64(i) / den
		}
		sb.WriteString(base.Foreground(RampAt(t)).Render(string(r)))
	}
	return sb.String()
}

// Rule draws a gradient horizontal divider.
func Rule(width int) string {
	if width < 1 {
		return ""
	}
	return Gradient(strings.Repeat("─", width), false)
}

// Box wraps body lines in a rounded frame whose top and bottom edges carry the
// gradient and whose sides pick up the ramp's two endpoints.
func Box(width int, lines ...string) string {
	inner := width - 2
	if inner < 1 {
		inner = 1
	}
	left := lipgloss.NewStyle().Foreground(RampAt(0))
	right := lipgloss.NewStyle().Foreground(RampAt(1))

	var sb strings.Builder
	sb.WriteString(Gradient("╭"+strings.Repeat("─", inner)+"╮", false) + "\n")
	for _, ln := range lines {
		pad := inner - lipgloss.Width(ln)
		if pad < 0 {
			pad = 0
		}
		sb.WriteString(left.Render("│") + ln + strings.Repeat(" ", pad) + right.Render("│") + "\n")
	}
	sb.WriteString(Gradient("╰"+strings.Repeat("─", inner)+"╯", false))
	return sb.String()
}

// Banner is the wordmark that opens every run. It spans the frame, with the
// wordmark on the left rail.
func Banner(width int) string { return BannerAt(width, revealFrames) }

// revealFrames is how long the wordmark takes to draw itself in, counted in
// spinner ticks (12/sec). Long enough to read as a reveal from the back of a
// room, short enough that it is over before the first stage reports.
const revealFrames = 22

// BannerAt draws the banner mid-reveal: the frame's rule and the wordmark wipe
// in left to right, lit cells carrying the ramp and the rest sitting dim. The
// block is the same size at every frame — a banner that grows would reflow the
// stage list under it on every tick.
func BannerAt(width, frame int) string {
	t := 1.0
	if frame < revealFrames {
		t = float64(frame) / float64(revealFrames)
	}
	mark := " "
	if t > 0 {
		mark = Gradient("◆", true)
	}
	// The wordmark trails the rule slightly, so the frame reads as drawing
	// itself and the name as arriving inside it rather than the two racing.
	word := wipe("F I E L D N O T E", (t-0.15)/0.85)
	inner := max(width-2, 1)
	edge := func(l, r string) string {
		return wipeRule(l+strings.Repeat("─", inner)+r, t)
	}
	body := "  " + mark + "  " + word
	pad := max(inner-lipgloss.Width(body), 0)
	// The sides light with the sweep that passes them: the left rail as the
	// wipe leaves it, the right one only once the rule has reached the corner.
	l, r := Dim, Dim
	if t > 0.05 {
		l = lipgloss.NewStyle().Foreground(RampAt(0))
	}
	if t >= 1 {
		r = lipgloss.NewStyle().Foreground(RampAt(1))
	}
	rail := func(mid string) string { return l.Render("│") + mid + r.Render("│") }
	return edge("╭", "╮") + "\n" +
		rail(strings.Repeat(" ", inner)) + "\n" +
		rail(body+strings.Repeat(" ", pad)) + "\n" +
		rail(strings.Repeat(" ", inner)) + "\n" +
		edge("╰", "╯")
}

// litAt is RampAt for the reveal, which paints one rune at a time and so has
// the same problem Gradient does: on a terminal that cannot render the curve,
// per-rune sampling is banding, not a gradient. The wipe still reads, because
// what carries it is the lit edge against the dim remainder, not the hue.
func litAt(t float64) lipgloss.Color {
	if !hasTrueColor() {
		return lipgloss.Color("#7C5CFF")
	}
	return RampAt(t)
}

// wipe reveals the first t of s in the ramp and holds the rest back as spaces,
// with a bright head on the boundary so the reveal has a leading edge rather
// than a soft edge nobody notices.
func wipe(s string, t float64) string {
	runes := []rune(s)
	if t >= 1 {
		return Gradient(s, true)
	}
	cut := int(t * float64(len(runes)))
	var sb strings.Builder
	den := float64(len(runes) - 1)
	for i, r := range runes {
		switch {
		case i < cut-1:
			sb.WriteString(lipgloss.NewStyle().Bold(true).Foreground(litAt(float64(i) / den)).Render(string(r)))
		case i == cut-1:
			sb.WriteString(Title.Render(string(r)))
		default:
			sb.WriteString(" ")
		}
	}
	return sb.String()
}

// wipeRule is wipe for the frame's own edges, where the unlit remainder stays
// drawn — a half-drawn box reads as a rendering fault, a half-lit one reads as
// the box switching on.
func wipeRule(s string, t float64) string {
	if t >= 1 {
		return Gradient(s, false)
	}
	runes := []rune(s)
	cut := int(t * float64(len(runes)))
	den := float64(len(runes) - 1)
	var sb strings.Builder
	for i, r := range runes {
		if i < cut {
			sb.WriteString(lipgloss.NewStyle().Foreground(litAt(float64(i) / den)).Render(string(r)))
		} else {
			sb.WriteString(Dim.Render(string(r)))
		}
	}
	return sb.String()
}

// Spread lays a left and a right fragment against the two edges of a width,
// filling the gap with spaces. Fragments are already-styled strings, so the
// padding is measured with lipgloss.Width rather than len.
func Spread(width int, left, right string) string {
	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + right
}

// Meter renders a determinate gradient bar: filled cells sample the ramp at
// their own position, so a full bar shows the whole brand curve.
func Meter(width int, pct float64) string {
	if width < 1 {
		return ""
	}
	filled := int(pct*float64(width) + 0.5)
	if filled > width {
		filled = width
	}
	var sb strings.Builder
	den := float64(width - 1)
	for i := 0; i < width; i++ {
		if i < filled {
			t := 0.0
			if den > 0 {
				t = float64(i) / den
			}
			sb.WriteString(lipgloss.NewStyle().Foreground(RampAt(t)).Render("▰"))
		} else {
			sb.WriteString(Dim.Render("▱"))
		}
	}
	return sb.String()
}

// Gauge is Meter with a needle: the leading filled cell is drawn in the tint
// the caller's verdict picked. A plain meter says how full it is; a needle in
// the verdict's own color says how full is too full, which is the only reason
// the number is on screen.
func Gauge(width int, pct float64, tint lipgloss.Style) string {
	if width < 1 {
		return ""
	}
	filled := min(int(pct*float64(width)+0.5), width)
	var sb strings.Builder
	den := float64(width - 1)
	for i := 0; i < width; i++ {
		switch {
		case i == filled-1:
			sb.WriteString(tint.Bold(true).Render("▰"))
		case i < filled:
			t := 0.0
			if den > 0 {
				t = float64(i) / den
			}
			sb.WriteString(lipgloss.NewStyle().Foreground(RampAt(t)).Render("▰"))
		default:
			sb.WriteString(Dim.Render("▱"))
		}
	}
	return sb.String()
}

// HeavyRule is the divider under the summary's wordmark. A second weight of
// line is what separates the headline from the closing rule; two identical
// rules read as two halves of one list.
func HeavyRule(width int) string {
	if width < 1 {
		return ""
	}
	return Gradient(strings.Repeat("━", width), false)
}

// Pulse renders an indeterminate bar: a lit band sweeps back and forth, its
// color tracking the band's position along the ramp.
func Pulse(width, frame int) string {
	if width < 1 {
		return ""
	}
	// The lit band scales with the bar so a wide terminal gets a sweep rather
	// than a speck crossing a long track.
	band := width / 6
	if band < 3 {
		band = 3
	}
	period := 2 * (width + band)
	pos := frame % period
	if pos >= width+band {
		pos = period - pos
	}
	head := pos - band
	var sb strings.Builder
	den := float64(width - 1)
	for i := 0; i < width; i++ {
		if i > head && i <= pos {
			t := 0.0
			if den > 0 {
				t = float64(i) / den
			}
			sb.WriteString(lipgloss.NewStyle().Foreground(RampAt(t)).Render("▰"))
		} else {
			sb.WriteString(Dim.Render("▱"))
		}
	}
	return sb.String()
}
