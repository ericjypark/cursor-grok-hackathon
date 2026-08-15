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
	for _, sz := range [][2]int{{48, 18}, {60, 20}, {80, 24}, {100, 30}, {180, 45}, {240, 60}} {
		cols, rows := sz[0], sz[1]
		m := newModel(nil)
		m.cols, m.rows, m.width = cols, rows, Fit(cols)
		m.set("map", "done", "31 pages")
		m.set("scrape_site", "running", "")

		lines := strings.Split(m.View(), "\n")
		if len(lines) != rows {
			t.Errorf("%dx%d: view is %d lines, want %d", cols, rows, len(lines), rows)
		}
		for i, ln := range lines {
			if w := lipgloss.Width(ln); w > cols {
				t.Errorf("%dx%d: line %d is %d wide, past the right edge", cols, rows, i, w)
			}
		}
	}
}

// A window too small for the layout still may not draw past its right edge —
// it degrades, it does not wrap.
func TestViewStaysInsideNarrowWindows(t *testing.T) {
	for _, sz := range [][2]int{{20, 10}, {32, 12}, {44, 14}} {
		cols, rows := sz[0], sz[1]
		m := newModel(nil)
		m.cols, m.rows, m.width = cols, rows, Fit(cols)
		for i, ln := range strings.Split(m.View(), "\n") {
			if w := lipgloss.Width(ln); w > cols {
				t.Errorf("%dx%d: line %d is %d wide, past the right edge", cols, rows, i, w)
			}
		}
	}
}
