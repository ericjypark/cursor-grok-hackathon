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

// A narrow window used to drop the running row's whole trailing column: the
// note moved into it, made it too wide for the guard, and took the stopwatch
// down with it. A running stage with no elapsed time reads as a hang, so the
// clock is the one thing the row may never surrender.
func TestRunningRowKeepsItsStopwatchOnNarrowWindows(t *testing.T) {
	for _, cols := range []int{40, 48, 60, 72} {
		m := newModel(nil, true)
		m.cols, m.rows, m.width = cols, 18, Fit(cols)
		m.set("search_collisions", "running", `searching "cursor" with no category context`)
		m.stages[3].startedAt = time.Now().Add(-42 * time.Second)

		row := stripANSI(m.stageLine(m.stages[3], m.width-2, false))
		if !strings.Contains(row, "42.0s") {
			t.Errorf("%d cols: running row lost its stopwatch: %q", cols, row)
		}
		if w := lipgloss.Width(row); w > m.width-2 {
			t.Errorf("%d cols: running row is %d wide, past the frame's %d", cols, w, m.width-2)
		}
	}
}

// The note is clipped to what is left beside the clock rather than being
// dropped outright — a partial note still says what the backend is on.
func TestNarrowRunningRowTruncatesTheNoteRatherThanLosingIt(t *testing.T) {
	m := newModel(nil, true)
	m.cols, m.rows, m.width = 60, 18, Fit(60)
	m.set("search_collisions", "running", `searching "cursor" with no category context`)
	m.stages[3].startedAt = time.Now().Add(-42 * time.Second)

	row := stripANSI(m.stageLine(m.stages[3], m.width-2, false))
	if !strings.Contains(row, "…") {
		t.Errorf("over-long note was not clipped: %q", row)
	}
	if !strings.Contains(row, `searching "curs`) {
		t.Errorf("clipped note kept nothing readable: %q", row)
	}
}

// Vertical slack goes into the list's own spacing before it opens gaps around
// it: a 100x30 window once split ten spare lines into two five-line canyons
// with a solid block of stages between them, which reads as a rendering fault.
func TestSlackGoesIntoTheListBeforeTheOuterGaps(t *testing.T) {
	m := newModel(nil, true)
	m.cols, m.rows, m.width = 100, 30, Fit(100)
	m.set("map", "done", "31 pages")
	m.set("search_collisions", "running", "searching for collisions")

	lines := strings.Split(stripANSI(m.View()), "\n")
	blank := func(i int) bool { return strings.TrimSpace(lines[i]) == "" }

	first, last := 0, len(lines)-1
	for first < len(lines) && !strings.Contains(lines[first], "Mapping the site") {
		first++
	}
	for last >= 0 && !strings.Contains(lines[last], "Normalizing posts") {
		last--
	}
	if first >= len(lines) || last < 0 {
		t.Fatalf("could not find the stage list:\n%s", strings.Join(lines, "\n"))
	}

	above, below := 0, 0
	for i := first - 1; i >= 0 && blank(i); i-- {
		above++
	}
	for i := last + 1; i < len(lines) && blank(i); i++ {
		below++
	}
	if above > 2 || below > 2 {
		t.Errorf("outer gaps are %d above and %d below, want no more than 2 each:\n%s",
			above, below, strings.Join(lines, "\n"))
	}
	inner := 0
	for i := first; i <= last; i++ {
		if blank(i) {
			inner++
		}
	}
	if inner == 0 {
		t.Errorf("spare lines never reached the list's own spacing:\n%s", strings.Join(lines, "\n"))
	}
}

// The view owns the alternate screen at every size it can be given, not only
// the handful the layout was tuned on.
func TestViewNeverOutgrowsItsWindow(t *testing.T) {
	for _, withT1 := range []bool{false, true} {
		for cols := 40; cols <= 140; cols += 4 {
			for rows := 10; rows <= 50; rows++ {
				m := newModel(nil, withT1)
				m.cols, m.rows, m.width = cols, rows, Fit(cols)
				m.set("map", "done", "31 pages")
				m.set("scrape_site", "failed", "")
				m.set("search_collisions", "running", `searching "cursor" with no category context`)

				lines := strings.Split(m.View(), "\n")
				if len(lines) > rows {
					t.Fatalf("t1=%v %dx%d: view is %d lines, past the window's %d",
						withT1, cols, rows, len(lines), rows)
				}
				for i, ln := range lines {
					if w := lipgloss.Width(ln); w > cols {
						t.Fatalf("t1=%v %dx%d: line %d is %d wide, past the right edge",
							withT1, cols, rows, i, w)
					}
				}
			}
		}
	}
}

// Eleven rows each reading "queued" is eleven rows of noise. Only the front of
// the queue is worth a word.
func TestOnlyTheNextQueuedStageIsLabelled(t *testing.T) {
	m := newModel(nil, true)
	m.cols, m.rows, m.width = 100, 40, Fit(100)
	m.set("map", "done", "31 pages")
	m.set("scrape_site", "running", "")

	view := stripANSI(m.View())
	if strings.Contains(view, "queued") {
		t.Errorf("the queue is still shouting:\n%s", view)
	}
	if n := strings.Count(view, "next"); n != 1 {
		t.Errorf("%d rows claim to be next, want exactly 1:\n%s", n, view)
	}
	// It is the first pending stage, not just any of them.
	for _, ln := range strings.Split(view, "\n") {
		if strings.Contains(ln, "next") && !strings.Contains(ln, "Reading the repository") {
			t.Errorf("the wrong row is marked next: %q", ln)
		}
	}
}

// The completion flash swaps the row's trailing column for a full bar for half
// a second. It runs on rows of every width, so it has to clear the same guard
// the pulse does rather than pushing the row past the frame.
func TestCompletionFlashNeverOutgrowsTheRow(t *testing.T) {
	for cols := 40; cols <= 160; cols += 4 {
		m := newModel(nil, true)
		m.cols, m.rows, m.width = cols, 30, Fit(cols)
		m.set("map", "done", "31 pages across 4 hosts, 2 of them redirects")
		m.stages[0].elapsed = 4200 * time.Millisecond
		for _, at := range []time.Duration{0, 600 * time.Millisecond, 2 * time.Second} {
			m.stages[0].settledAt = time.Now().Add(-at)
			row := m.stageLine(m.stages[0], m.width-2, true)
			if w := lipgloss.Width(row); w > m.width-2 {
				t.Errorf("%d cols, %s after settling: row is %d wide, past the frame's %d",
					cols, at, w, m.width-2)
			}
			// The elapsed time is the row's payload once it is done; the flash
			// may replace the note, never the clock.
			if !strings.Contains(stripANSI(row), "4.2s") {
				t.Errorf("%d cols, %s after settling: row lost its elapsed time: %q", cols, at, row)
			}
		}
	}
}

// The flash is the whole point of settledAt: a stage that flips from a
// sweeping bar to a tick between two frames is a change nobody watching from
// across a room catches.
func TestAFinishedStageHoldsAFullBarBeforeItSettles(t *testing.T) {
	m := newModel(nil, true)
	m.cols, m.rows, m.width = 120, 30, Fit(120)
	m.set("map", "done", "31 pages")
	m.stages[0].elapsed = 4200 * time.Millisecond

	if !strings.Contains(m.stageLine(m.stages[0], m.width-2, true), "▰▰▰") {
		t.Error("a stage that just finished did not flash a full bar")
	}
	m.stages[0].settledAt = time.Now().Add(-2 * time.Second)
	settled := stripANSI(m.stageLine(m.stages[0], m.width-2, true))
	if strings.Contains(settled, "▰") {
		t.Errorf("the bar never collapsed back into the readout: %q", settled)
	}
	if !strings.Contains(settled, "31 pages") {
		t.Errorf("the settled row lost its note: %q", settled)
	}
}

// Phase headers are hierarchy, not information: they are worth a line only
// while every stage still has one. A window that had to drop a stage row must
// not be spending lines on headings.
func TestPhaseHeadersAreNeverBoughtWithAStageRow(t *testing.T) {
	for _, withT1 := range []bool{false, true} {
		for cols := 40; cols <= 140; cols += 4 {
			for rows := 10; rows <= 50; rows++ {
				m := newModel(nil, withT1)
				m.cols, m.rows, m.width = cols, rows, Fit(cols)
				m.set("map", "done", "31 pages")
				m.set("search_collisions", "running", "searching")

				view := stripANSI(m.View())
				if !strings.Contains(view, phaseNames[0][1]) {
					continue
				}
				for _, s := range m.stages {
					if !strings.Contains(view, s.label) {
						t.Fatalf("t1=%v %dx%d: headers are on screen but %q is not:\n%s",
							withT1, cols, rows, s.label, view)
					}
				}
			}
		}
	}
}

// A header with its group's first row pushed a blank line below it reads as a
// heading for nothing.
func TestAPhaseHeaderStaysGluedToItsFirstStage(t *testing.T) {
	m := newModel(nil, true)
	m.cols, m.rows, m.width = 110, 34, Fit(110)
	lines := strings.Split(stripANSI(m.View()), "\n")
	for i, ln := range lines {
		if !strings.Contains(ln, phaseNames[0][1]) && !strings.Contains(ln, phaseNames[1][1]) {
			continue
		}
		if i+1 >= len(lines) || strings.TrimSpace(lines[i+1]) == "" {
			t.Errorf("a phase header was left hanging over a blank line:\n%s", strings.Join(lines, "\n"))
		}
	}
}
