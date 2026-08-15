package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

// The progress view owns the alternate screen, so it must land on exactly as
// many lines as the window has and never push a line past its right edge —
// either mistake makes the terminal scroll and tears the frame apart.
func TestViewFillsTheWindowExactly(t *testing.T) {
	// Both stage plans: T0 alone is 7 rows, T0+T1 is 11. The taller one is what
	// a short window has to survive.
	for _, withT1 := range []bool{false, true} {
		for _, sz := range [][2]int{{48, 18}, {60, 20}, {80, 24}, {100, 30}, {180, 45}, {240, 60}} {
			cols, rows := sz[0], sz[1]
			m := newModel(nil, withT1)
			m.cols, m.rows, m.width = cols, rows, Fit(cols)
			m.set("map", "done", "31 pages")
			m.set("scrape_site", "running", "")
			m.set("scrape_reddit", "running", "184s elapsed")

			lines := strings.Split(m.View(), "\n")
			if len(lines) != rows {
				t.Errorf("t1=%v %dx%d: view is %d lines, want %d", withT1, cols, rows, len(lines), rows)
			}
			for i, ln := range lines {
				if w := lipgloss.Width(ln); w > cols {
					t.Errorf("t1=%v %dx%d: line %d is %d wide, past the right edge", withT1, cols, rows, i, w)
				}
			}
		}
	}
}

// A --no-scrape run must not display rows it will never reach, or its meter
// could never fill.
func TestT0OnlyRunsShowNoScrapeStages(t *testing.T) {
	m := newModel(nil, false)
	for _, s := range m.stages {
		if strings.HasPrefix(s.key, "scrape_reddit") || s.key == "select_sources" {
			t.Errorf("t0-only plan still lists %q", s.key)
		}
	}
	if got := len(newModel(nil, true).stages) - len(m.stages); got != len(t1Stages) {
		t.Errorf("T1 added %d rows, want %d", got, len(t1Stages))
	}
}

// A window too small for the layout still may not draw past its right edge —
// it degrades, it does not wrap.
func TestViewStaysInsideNarrowWindows(t *testing.T) {
	for _, sz := range [][2]int{{20, 10}, {32, 12}, {44, 14}} {
		cols, rows := sz[0], sz[1]
		m := newModel(nil, true)
		m.cols, m.rows, m.width = cols, rows, Fit(cols)
		for i, ln := range strings.Split(m.View(), "\n") {
			if w := lipgloss.Width(ln); w > cols {
				t.Errorf("%dx%d: line %d is %d wide, past the right edge", cols, rows, i, w)
			}
		}
	}
}
