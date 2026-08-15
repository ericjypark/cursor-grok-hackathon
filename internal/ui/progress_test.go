package ui

import (
	"strings"
	"testing"
	"time"

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

// The longest stage in the run is one opaque model call, so the row has to say
// what the backend is on and how long it has been on it — otherwise a working
// minute is indistinguishable from a hang.
func TestRunningStageShowsItsNoteAndStopwatch(t *testing.T) {
	m := newModel(nil, false)
	m.cols, m.rows, m.width = 120, 40, Fit(120)
	m.set("synthesize", "running", "grok-4.6 · drafting from 12 evidence blocks")
	m.stages[6].startedAt = time.Now().Add(-90 * time.Second)

	view := stripANSI(m.View())
	if !strings.Contains(view, "↳ grok-4.6 · drafting from 12 evidence blocks") {
		t.Errorf("running stage drew no note line:\n%s", view)
	}
	if !strings.Contains(view, "01:30") {
		t.Errorf("running stage drew no stopwatch:\n%s", view)
	}
	// The note belongs under the label, not beside it.
	for _, ln := range strings.Split(view, "\n") {
		if strings.Contains(ln, "Synthesizing the dossier") && strings.Contains(ln, "grok-4.6") {
			t.Errorf("note collided with the label row: %q", ln)
		}
	}
}

// A note is worth more than the pulse, so a window with no room for a second
// line surrenders the pulse rather than the note.
func TestNoteSurvivesAWindowTooShortForSubLines(t *testing.T) {
	m := newModel(nil, true)
	m.cols, m.rows, m.width = 120, 16, Fit(120)
	m.set("scrape_reddit", "running", "r/cursor · 12 threads")

	view := stripANSI(m.View())
	if strings.Contains(view, "↳") {
		t.Errorf("short window still drew a sub-line:\n%s", view)
	}
	if !strings.Contains(view, "r/cursor · 12 threads") {
		t.Errorf("short window dropped the note entirely:\n%s", view)
	}
}

func TestStopwatchDropsMillisecondsAndRollsOverToMinutes(t *testing.T) {
	for _, c := range []struct {
		d    time.Duration
		want string
	}{
		{400 * time.Millisecond, "0.4s"},
		{12300 * time.Millisecond, "12.3s"},
		{95 * time.Second, "01:35"},
	} {
		if got := stopwatch(c.d); got != c.want {
			t.Errorf("stopwatch(%s) = %q, want %q", c.d, got, c.want)
		}
	}
}

func stripANSI(s string) string {
	var sb strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == 0x1b {
			for i < len(s) && s[i] != 'm' {
				i++
			}
			continue
		}
		sb.WriteByte(s[i])
	}
	return sb.String()
}

// The running stage is the last row a cramped window gives up, not the first:
// clipping the tail once dropped the only stage doing any work.
func TestShortWindowKeepsTheRunningStage(t *testing.T) {
	m := newModel(nil, true)
	m.cols, m.rows, m.width = 120, 16, Fit(120)
	m.set("map", "done", "150 pages")
	m.set("scrape_reddit", "running", "r/cursor")

	var running bool
	for _, s := range fit(m.stages, 6) {
		running = running || s.key == "scrape_reddit"
	}
	if !running {
		t.Error("a short window dropped the stage that was actually working")
	}
}
