// Package ui renders live pipeline progress while the backend works.
package ui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/ericjypark/cursor-grok-hackathon/internal/client"
)

// labelCol keeps every stage's trailing column aligned no matter which label
// is longest; the trailing column itself is right-aligned to the frame edge.
const labelCol = 26

// meterWidth scales the progress bars with the frame so a wide terminal gets a
// wide bar instead of a stub floating in whitespace.
func meterWidth(frame int) int {
	w := frame / 3
	if w < 10 {
		w = 10
	}
	if w > 28 {
		w = 28
	}
	return w
}

type stage struct {
	key, label, detail, status string
	startedAt                  time.Time
	elapsed                    time.Duration
}

// Display order, which is also roughly execution order.
var stageOrder = []stage{
	// T0 — product understanding
	{key: "map", label: "Mapping the site"},
	{key: "scrape_site", label: "Reading key pages"},
	{key: "scrape_repo", label: "Reading the repository"},
	{key: "search_collisions", label: "Hunting name collisions"},
	{key: "find_similar", label: "Finding adjacent products"},
	{key: "search_context", label: "Confirming vocabulary"},
	{key: "synthesize", label: "Synthesizing the dossier"},
}

// T1 — sources and scraping. Kept separate so a --stop-after t0 run does not
// show four rows it will never reach, and a meter that can never fill.
var t1Stages = []stage{
	{key: "select_sources", label: "Choosing where to look"},
	{key: "scrape_reddit", label: "Scraping Reddit"},
	{key: "scrape_hackernews", label: "Scraping HackerNews"},
	{key: "map_posts", label: "Normalizing posts"},
}

type eventMsg client.Event
type closedMsg struct{}

type model struct {
	stages   []stage
	spin     spinner.Model
	events   <-chan client.Event
	dossier  *client.ProductDossier
	result   *client.FieldNote
	warnings []string
	fatal    error
	quitting bool

	frame   int
	startAt time.Time
	width   int // frame width content is drawn at
	cols    int // full terminal width
	rows    int // full terminal height
}

func newModel(events <-chan client.Event, withT1 bool) model {
	s := spinner.New()
	// Frames without the trailing space bubbles ships on Dot, so a running row
	// occupies exactly the same width as a settled one and nothing jitters.
	s.Spinner = spinner.Spinner{
		Frames: []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"},
		FPS:    time.Second / 12,
	}

	planned := stageOrder
	if withT1 {
		planned = append(append([]stage{}, stageOrder...), t1Stages...)
	}
	stages := make([]stage, len(planned))
	copy(stages, planned)
	for i := range stages {
		stages[i].status = "pending"
	}
	cols, rows := TermSize()
	return model{
		stages:  stages,
		spin:    s,
		events:  events,
		startAt: time.Now(),
		cols:    cols,
		rows:    rows,
		width:   Fit(cols),
	}
}

func waitFor(ch <-chan client.Event) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return closedMsg{}
		}
		return eventMsg(ev)
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(m.spin.Tick, waitFor(m.events))
}

func (m *model) set(key, status, detail string) {
	for i := range m.stages {
		if m.stages[i].key != key {
			continue
		}
		s := &m.stages[i]
		switch {
		case status == "running" && s.startedAt.IsZero():
			s.startedAt = time.Now()
		case status != "running" && !s.startedAt.IsZero() && s.elapsed == 0:
			s.elapsed = time.Since(s.startedAt)
		}
		s.status = status
		if detail != "" {
			s.detail = detail
		}
		return
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.cols, m.rows = msg.Width, msg.Height
		m.width = Fit(msg.Width)
		return m, nil
	case tea.KeyMsg:
		if msg.Type == tea.KeyCtrlC || msg.String() == "q" {
			m.quitting = true
			m.fatal = errors.New("cancelled")
			return m, tea.Quit
		}
	case spinner.TickMsg:
		m.frame++
		// Cycling the spinner's own color along the ramp is what gives the
		// active row its glow.
		m.spin.Style = lipgloss.NewStyle().Foreground(RampAt(wave(m.frame)))
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd
	case eventMsg:
		ev := client.Event(msg)
		switch ev.Kind {
		case "stage":
			if s, err := ev.Stage(); err == nil {
				detail := ""
				if s.Detail != nil {
					detail = *s.Detail
				}
				m.set(string(s.Stage), string(s.Status), detail)
			}
		case "error":
			if e, err := ev.Err(); err == nil {
				if e.Fatal != nil && *e.Fatal {
					m.fatal = errors.New(e.Detail)
					return m, tea.Quit
				}
				label := "warning"
				if e.Stage != nil {
					label = string(*e.Stage)
					m.set(label, "failed", "")
				}
				m.warnings = append(m.warnings, fmt.Sprintf("%s: %s", label, e.Detail))
			}
		case "dossier":
			// T0 is done but the run is not: T1 still has to scrape. Hold the
			// dossier and keep rendering rather than tearing the UI down.
			if d, err := ev.Dossier(); err == nil {
				m.dossier = &d
			}
		case "harvest":
			if h, err := ev.Harvest(); err == nil && !h.Live {
				m.warnings = append(m.warnings, "scrape: "+h.SourceNote)
			}
		case "heartbeat":
			// Nothing to draw. Its job is done by having arrived: it keeps the
			// connection warm through the minutes Reddit takes.
		case "result":
			if n, err := ev.Result(); err == nil {
				m.result = &n
			} else {
				m.fatal = fmt.Errorf("malformed result event: %w", err)
			}
			return m, tea.Quit
		}
		return m, waitFor(m.events)
	case closedMsg:
		// A stream that died after T0 still has a usable dossier, so salvage it
		// rather than throwing away a minute of finished work.
		if m.result == nil && m.dossier != nil {
			m.result = &client.FieldNote{Dossier: *m.dossier}
			m.warnings = append(m.warnings, "backend closed the stream early — the dossier is complete, the scrape is not")
		}
		if m.result == nil && m.fatal == nil {
			m.fatal = errors.New("backend closed the stream without returning a dossier")
		}
		return m, tea.Quit
	}
	return m, nil
}

// wave maps a frame counter onto a 0..1..0 triangle, so ramp-sampled colors
// breathe rather than jumping at the loop point.
func wave(frame int) float64 {
	const period = 40
	p := frame % period
	if p >= period/2 {
		p = period - p
	}
	return float64(p) / float64(period/2)
}

func (m model) View() string {
	head := Banner(m.width)

	done := 0
	for _, s := range m.stages {
		if s.status == "done" {
			done++
		}
	}

	var foot strings.Builder
	foot.WriteString(Rule(m.width) + "\n")
	bar := Meter(meterWidth(m.width), float64(done)/float64(len(m.stages)))
	status := " " + bar +
		"  " + Title.Render(fmt.Sprintf("%d", done)) + Dim.Render(fmt.Sprintf("/%d", len(m.stages))) +
		"  " + Dim.Render(clock(time.Since(m.startAt)))
	foot.WriteString(Spread(m.width, status, Dim.Render("q")+" "+Dim.Render("cancel")))
	for _, w := range m.warnings {
		foot.WriteString("\n " + Warn.Render("▲ "+w))
	}
	if m.fatal != nil {
		foot.WriteString("\n " + Bad.Render("✗ "+m.fatal.Error()))
	}

	// Vertical layout: header on the top rail, footer on the bottom one, the
	// stage list floating in the space between them. A tall window spends its
	// slack double-spacing the list before it opens gaps around it.
	headH, footH := lipgloss.Height(head), lipgloss.Height(foot.String())

	// Two separator lines always sit between the three blocks, so the list can
	// only have what is left after them. Eleven stages do not fit a short
	// window, and a view taller than its window scrolls the terminal and tears
	// the frame apart.
	budget := m.rows - headH - footH - 2
	shown := fit(m.stages, budget)
	rows := m.stageRows(shown, true)
	// Sub-lines are the first thing a short window gives up: dropping one
	// costs a note, dropping a row costs a whole stage.
	if height(rows) > budget {
		rows = m.stageRows(shown, false)
	}

	sep, listH := "\n", height(rows)
	if m.rows-headH-listH-footH >= listH {
		sep, listH = "\n\n", listH+len(rows)-1
	}

	// free is the blank-line budget; splitting it above and below the list
	// makes the header and footer land on the window's own first and last row.
	above, below := 0, 0
	if free := m.rows - headH - listH - footH; free > 0 {
		above, below = free/2, free-free/2
	}

	body := head +
		strings.Repeat("\n", above+1) + strings.Join(rows, sep) +
		strings.Repeat("\n", below+1) + foot.String()
	return rail(body, m.width)
}

func (m model) stageRows(stages []stage, withSubs bool) []string {
	rows := make([]string, 0, len(stages))
	for _, s := range stages {
		rows = append(rows, " "+m.stageLine(s, m.width-2, withSubs))
	}
	return rows
}

func height(rows []string) int {
	n := 0
	for _, r := range rows {
		n += lipgloss.Height(r)
	}
	return n
}

// fit picks which stage rows to draw when the window cannot hold them all.
// Finished stages go first — the footer already reports how many are done —
// then queued ones from the tail back. Running stages are surrendered last:
// what a watcher needs to see is the work happening right now.
func fit(stages []stage, budget int) []stage {
	if budget >= len(stages) {
		return stages
	}
	if budget < 1 {
		budget = 1
	}
	keep := make([]bool, len(stages))
	for i := range keep {
		keep[i] = true
	}
	drop := len(stages) - budget
	for i := 0; i < len(stages) && drop > 0; i++ {
		if stages[i].status == "done" {
			keep[i], drop = false, drop-1
		}
	}
	for i := len(stages) - 1; i >= 0 && drop > 0; i-- {
		if keep[i] && stages[i].status == "pending" {
			keep[i], drop = false, drop-1
		}
	}
	out := make([]stage, 0, budget)
	for i, s := range stages {
		if keep[i] {
			out = append(out, s)
		}
	}
	if len(out) > budget {
		out = out[:budget]
	}
	return out
}

// rail insets every line by the side margin and clips it to the frame. The
// clip is the last line of defence: on a window too small for its own content
// a wrapped line would shift every row below it and tear the frame apart.
func rail(block string, width int) string {
	pad := strings.Repeat(" ", margin)
	lines := strings.Split(block, "\n")
	for i := range lines {
		lines[i] = pad + ansi.Truncate(lines[i], width, "")
	}
	return strings.Join(lines, "\n")
}

// stageLine draws one row: status glyph, label, then a trailing column that is
// a live pulse while running and a timing/detail readout once settled. A
// running stage with a note gets a second line under it — synthesis and the
// scrapes are minutes long, and a row that only says "running" for that long
// is indistinguishable from a hang.
func (m model) stageLine(s stage, width int, withSub bool) string {
	var icon, label, trail, sub string
	switch s.status {
	case "done":
		icon, label = Good.Render("✔"), Body.Render(s.label)
		if s.detail != "" {
			trail = Muted.Render(s.detail) + "  "
		}
		trail += Dim.Render(dur(s.elapsed))
	case "running":
		icon, label = m.spin.View(), Title.Render(s.label)
		// The stopwatch runs off startedAt, so it ticks with the spinner
		// rather than waiting on the backend to say something.
		trail = Pulse(meterWidth(width), m.frame) + "  " + Dim.Render(stopwatch(m.since(s)))
		switch {
		case s.detail == "":
		case withSub:
			sub = Dim.Render("↳ ") + Muted.Render(s.detail)
		default:
			// No room for a second line: the note is worth more than the
			// pulse, so it takes the trailing column back.
			trail = Muted.Render(s.detail) + "  " + Dim.Render(stopwatch(m.since(s)))
		}
	case "failed":
		icon, label = Warn.Render("▲"), Body.Render(s.label)
		trail = Warn.Render("degraded")
	default:
		icon, label = Dim.Render("·"), Dim.Render(s.label)
		trail = Dim.Render("queued")
	}

	left := icon + "  " + label
	// Below the aligned label column the row would overlap, so the trailing
	// readout is dropped rather than wrapped.
	row := Spread(width, left, trail)
	if width < labelCol+lipgloss.Width(trail)+4 {
		row = left
	}
	if sub != "" {
		row += "\n   " + sub
	}
	return row
}

// since reports how long a stage has been running, and nothing for a stage
// that has not started or has already settled.
func (m model) since(s stage) time.Duration {
	if s.startedAt.IsZero() || s.status != "running" {
		return 0
	}
	return time.Since(s.startedAt)
}

// stopwatch formats a still-running stage's elapsed time. Unlike dur it never
// renders milliseconds: a figure repainting twelve times a second is motion,
// not information, and the spinner already carries that job.
func stopwatch(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	return clock(d)
}

func dur(d time.Duration) string {
	if d == 0 {
		return ""
	}
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	return clock(d)
}

func clock(d time.Duration) string {
	return fmt.Sprintf("%02d:%02d", int(d.Minutes()), int(d.Seconds())%60)
}

// Run drives the progress display until the stream ends, returning everything
// the run produced.
func Run(ctx context.Context, events <-chan client.Event, withT1 bool) (client.FieldNote, error) {
	final, err := tea.NewProgram(newModel(events, withT1),
		tea.WithContext(ctx), tea.WithAltScreen()).Run()
	if err != nil {
		return client.FieldNote{}, err
	}
	m, ok := final.(model)
	if !ok {
		return client.FieldNote{}, errors.New("unexpected terminal model")
	}
	if m.fatal != nil {
		return client.FieldNote{}, m.fatal
	}
	if m.result == nil {
		return client.FieldNote{}, errors.New("no dossier was produced")
	}
	return *m.result, nil
}
