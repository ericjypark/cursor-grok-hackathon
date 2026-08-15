package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// The color profile is global, so the suite pins it to Ascii: assertions are
// then about geometry, which is what can actually break.
func TestMain(m *testing.M) {
	lipgloss.SetColorProfile(termenv.Ascii)
	m.Run()
}

func TestRampAtClampsToEndpoints(t *testing.T) {
	for _, tc := range []struct {
		in   float64
		want string
	}{{-1, rampHex[0]}, {0, rampHex[0]}, {1, rampHex[len(rampHex)-1]}, {2, rampHex[len(rampHex)-1]}} {
		if got := string(RampAt(tc.in)); got != tc.want {
			t.Errorf("RampAt(%v) = %s, want %s", tc.in, got, tc.want)
		}
	}
}

func TestRampAtStaysInsideTheRamp(t *testing.T) {
	// The interior of the curve must never fall off the end of the stop list.
	for i := 0; i <= 100; i++ {
		if c := string(RampAt(float64(i) / 100)); !strings.HasPrefix(c, "#") || len(c) != 7 {
			t.Fatalf("RampAt(%v) produced %q", float64(i)/100, c)
		}
	}
}

func TestMeterFillsProportionally(t *testing.T) {
	for _, tc := range []struct {
		pct  float64
		want int
	}{{0, 0}, {0.5, 5}, {1, 10}, {1.5, 10}} {
		got := strings.Count(Meter(10, tc.pct), "▰")
		if got != tc.want {
			t.Errorf("Meter(10, %v) filled %d cells, want %d", tc.pct, got, tc.want)
		}
		if total := len([]rune(Meter(10, tc.pct))); total != 10 {
			t.Errorf("Meter(10, %v) is %d cells wide, want 10", tc.pct, total)
		}
	}
}

func TestPulseKeepsWidthAcrossTheWholeCycle(t *testing.T) {
	// The band sweeps out and back; at no frame may it change the bar's width
	// or run past either end.
	for frame := 0; frame < 200; frame++ {
		bar := []rune(Pulse(12, frame))
		if len(bar) != 12 {
			t.Fatalf("frame %d: width %d, want 12", frame, len(bar))
		}
		if lit := strings.Count(string(bar), "▰"); lit > 4 {
			t.Fatalf("frame %d: %d lit cells, want at most 4", frame, lit)
		}
	}
}

func TestBoxIsRectangular(t *testing.T) {
	lines := strings.Split(Box(20, "hi", "there"), "\n")
	if len(lines) != 4 {
		t.Fatalf("got %d lines, want 4", len(lines))
	}
	for i, ln := range lines {
		if w := lipgloss.Width(ln); w != 20 {
			t.Errorf("line %d is %d wide, want 20", i, w)
		}
	}
}

func TestSpacedLetterSpacesAndFallsBack(t *testing.T) {
	if got := spaced(" cursor "); got != "C U R S O R" {
		t.Errorf("spaced = %q", got)
	}
	if got := spaced(""); got != "UNKNOWN" {
		t.Errorf("spaced(empty) = %q, want UNKNOWN", got)
	}
}

func TestFitStaysInsideTheWindow(t *testing.T) {
	for _, cols := range []int{1, 20, 44, 80, 200, 400} {
		got := Fit(cols)
		if got > cols && cols > 0 {
			t.Errorf("Fit(%d) = %d, wider than the window", cols, got)
		}
		if got < 1 {
			t.Errorf("Fit(%d) = %d, want at least 1", cols, got)
		}
		// Room for the rail on both sides, or the frame overflows the window.
		if cols >= 2*margin+1 && got > cols-2*margin {
			t.Errorf("Fit(%d) = %d, leaves no rail", cols, got)
		}
	}
}

func TestSpreadFillsExactlyTheWidth(t *testing.T) {
	if got := lipgloss.Width(Spread(30, "left", "right")); got != 30 {
		t.Errorf("Spread width = %d, want 30", got)
	}
	// Overflowing fragments keep a single separating space rather than
	// silently overlapping.
	if got := Spread(4, "left", "right"); got != "left right" {
		t.Errorf("Spread(overflow) = %q", got)
	}
}

func TestBannerSpansTheFrameAtEveryWidth(t *testing.T) {
	for _, w := range []int{40, 60, 80, 200} {
		for i, ln := range strings.Split(Banner(w), "\n") {
			if got := lipgloss.Width(ln); got != w {
				t.Errorf("Banner(%d) line %d is %d wide", w, i, got)
			}
		}
	}
}
