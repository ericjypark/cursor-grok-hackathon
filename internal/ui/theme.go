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
func Banner(width int) string {
	mark := Gradient("◆", true)
	word := Gradient("F I E L D N O T E", true)
	return Box(width, "", "  "+mark+"  "+word, "")
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
