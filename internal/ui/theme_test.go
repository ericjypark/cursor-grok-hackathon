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

// The banner draws itself in over the run's first two seconds, on top of a
// stage list that is already rendering under it. A reveal that changed the
// block's size on any frame would reflow every row below it twelve times a
// second.
func TestBannerRevealHoldsItsBlockAtEveryFrame(t *testing.T) {
	for _, w := range []int{40, 76, 120, 200} {
		for frame := 0; frame <= revealFrames+5; frame++ {
			lines := strings.Split(BannerAt(w, frame), "\n")
			if len(lines) != 5 {
				t.Fatalf("BannerAt(%d, %d) is %d lines, want 5", w, frame, len(lines))
			}
			for i, ln := range lines {
				if got := lipgloss.Width(ln); got != w {
					t.Fatalf("BannerAt(%d, %d) line %d is %d wide", w, frame, i, got)
				}
			}
		}
	}
}

// The reveal has to actually be a reveal: empty at the first frame, complete
// by the last, and never going backwards in between.
func TestBannerRevealAdvancesAndSettles(t *testing.T) {
	// The wordmark row, counted between its own two rails: everything not yet
	// revealed is held back as a space.
	lit := func(frame int) int {
		row := []rune(stripANSI(strings.Split(BannerAt(60, frame), "\n")[2]))
		n := 0
		for _, r := range row[1 : len(row)-1] {
			if r != ' ' {
				n++
			}
		}
		return n
	}
	if lit(0) != 0 {
		t.Errorf("the wordmark was already on screen at frame 0")
	}
	prev := 0
	for frame := 0; frame <= revealFrames; frame++ {
		if n := lit(frame); n < prev {
			t.Fatalf("frame %d un-revealed part of the wordmark", frame)
		} else {
			prev = n
		}
	}
	// Once revealed it is the banner, exactly — a settled frame that differs
	// from the static wordmark would flicker at the end of the reveal.
	if BannerAt(60, revealFrames+30) != Banner(60) {
		t.Error("the settled reveal is not the banner")
	}
}

func TestGaugeKeepsItsWidthAndPlacesTheNeedle(t *testing.T) {
	for _, pct := range []float64{0, 0.25, 0.75, 1} {
		g := Gauge(16, pct, Warn)
		if got := lipgloss.Width(g); got != 16 {
			t.Errorf("Gauge(16, %v) is %d wide, want 16", pct, got)
		}
		want := int(pct*16 + 0.5)
		if got := strings.Count(g, "▰"); got != want {
			t.Errorf("Gauge(16, %v) filled %d cells, want %d", pct, got, want)
		}
	}
	// An empty gauge has nothing to point at, and must not borrow the last
	// cell of the track to say so.
	if strings.Contains(Gauge(8, 0, Warn), "▰") {
		t.Error("an empty gauge drew a needle")
	}
}
