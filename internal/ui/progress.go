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
	// settledAt is when the row stopped running. It exists only for the
	// completion flash: a stage that flips from a sweeping bar to a tick with
	// no moment in between is a change nobody in an audience catches.
	settledAt time.Time
	elapsed   time.Duration
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
	nextKey string // the queued stage nearest the front, the only one labelled
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
		if settled(status) && !settled(s.status) {
			s.settledAt = time.Now()
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
	head := BannerAt(m.width, m.frame)

	done := 0
	for _, s := range m.stages {
		if s.status == "done" {
			done++
		}
	}
	// Only the stage about to start is labelled, so the queue reads as one
	// waiting list with a front rather than as six rows of "queued".
	for _, s := range m.stages {
		if s.status == "pending" {
			m.nextKey = s.key
			break
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

	// Phase headers are the one thing here that is pure hierarchy, so they are
	// taken only out of slack: they appear when every stage row already fits
	// with their own cost to spare, and never at the price of a stage.
	cost := 2*len(phases(m.stages)) - 1
	useHeads := budget >= len(m.stages)+cost
	room := budget
	if useHeads {
		room -= cost
	}

	shown := fit(m.stages, room)
	rows, glued := m.stageRows(shown, true, useHeads)
	// Sub-lines are the first thing a short window gives up: dropping one
	// costs a note, dropping a row costs a whole stage.
	if height(rows) > budget {
		rows, glued = m.stageRows(shown, false, useHeads)
	}

	// free is the blank-line budget between the three blocks. Spend it on the
	// list's own spacing first and cap what is left over each outer gap: an
	// all-or-nothing double-space left a window with almost enough slack
	// looking like a rendering fault — two five-line canyons around a list
	// still packed solid.
	const outerGap = 2
	free := m.rows - headH - height(rows) - footH
	inner := 0
	if free > 0 {
		inner = min(max(free-2*outerGap, 0), len(rows)-1)
	}
	list := openGaps(rows, inner, glued)

	// Whatever the list could not absorb splits above and below it, which is
	// what lands the header and footer on the window's own first and last row.
	// Measuring the spaced list rather than trusting inner keeps that split
	// honest: the view has to come out exactly m.rows tall or it scrolls.
	above, below := 0, 0
	if free = m.rows - headH - lipgloss.Height(list) - footH; free > 0 {
		above, below = free/2, free-free/2
	}

	body := head +
		strings.Repeat("\n", above+1) + list +
		strings.Repeat("\n", below+1) + foot.String()
	return rail(body, m.width)
}

// stageRows draws one row per stage, optionally prefixed by the phase header
// its group opens. It also reports which gaps are glued shut: a header and the
// first row under it are one object, and a blank line driven between them
// reads as a heading for nothing.
func (m model) stageRows(stages []stage, withSubs, withHeads bool) (rows []string, glued []bool) {
	open := -1
	for _, s := range stages {
		if withHeads {
			if p := phaseOf(s); p != open {
				head := m.phaseHeader(p, m.width-2)
				if open >= 0 {
					// The two phases are different work. One blank line is
					// what says so even on a window with no slack to spend.
					head = "\n" + head
				}
				open = p
				rows, glued = append(rows, head), append(glued, false)
				rows, glued = append(rows, " "+m.stageLine(s, m.width-2, withSubs)), append(glued, true)
				continue
			}
		}
		rows, glued = append(rows, " "+m.stageLine(s, m.width-2, withSubs)), append(glued, false)
	}
	return rows, glued
}

// The pipeline is two pieces of work, not eleven steps: T0 decides what the
// product is, T1 goes and finds people complaining about it. Naming them is
// what makes a four-minute wait legible to someone who has never seen the CLI.
var phaseNames = [][2]string{
	{"T0", "UNDERSTANDING THE PRODUCT"},
	{"T1", "FINDING THE COMPLAINTS"},
}

func phaseOf(s stage) int {
	for _, t := range t1Stages {
		if t.key == s.key {
			return 1
		}
	}
	return 0
}

// phases reports which phases a stage plan actually contains, so a --no-scrape
// run is not charged for a header it will never draw.
func phases(stages []stage) []int {
	seen := map[int]bool{}
	out := []int{}
	for _, s := range stages {
		if p := phaseOf(s); !seen[p] {
			seen[p], out = true, append(out, p)
		}
	}
	return out
}

// phaseHeader is a rule with the phase's name on it and its own count on the
// right. The rule is dim: it is a section marker, and a gradient here would
// compete with the banner and the footer for the same eye.
func (m model) phaseHeader(p, width int) string {
	done, total := 0, 0
	for _, s := range m.stages {
		if phaseOf(s) != p {
			continue
		}
		total++
		if s.status == "done" {
			done++
		}
	}
	name := phaseNames[p]
	left := lipgloss.NewStyle().Foreground(RampAt(float64(p))).Render("▌") +
		" " + Muted.Render(name[0]) + "  " + Body.Render(name[1])
	right := Title.Render(fmt.Sprintf("%d", done)) + Dim.Render(fmt.Sprintf("/%d", total))
	fill := width - lipgloss.Width(left) - lipgloss.Width(right) - 2
	if fill < 1 {
		return ansi.Truncate(Spread(width, left, right), width, "")
	}
	return left + " " + Dim.Render(strings.Repeat("─", fill)) + " " + right
}

// openGaps joins the rows, opening extra of the gaps between them. The chosen
// gaps are spaced on half-steps so a partial spend lands centred in the list
// instead of packed against one end, where a single stray blank line reads as
// a seam rather than as breathing room.
func openGaps(rows []string, extra int, glued []bool) string {
	gaps := len(rows) - 1
	if gaps < 1 || extra < 1 {
		return strings.Join(rows, "\n")
	}
	open := make(map[int]bool, extra)
	for k := 0; k < extra; k++ {
		i := (2*k+1)*gaps/(2*extra) + 1
		// Slide past gaps that are already open or glued shut rather than
		// spending the line twice or splitting a phase off its first stage.
		for i < len(rows) && (open[i] || (i < len(glued) && glued[i])) {
			i++
		}
		if i >= len(rows) {
			continue
		}
		open[i] = true
	}
	var sb strings.Builder
	for i, r := range rows {
		if i > 0 {
			sb.WriteString("\n")
			if open[i] {
				sb.WriteString("\n")
			}
		}
		sb.WriteString(r)
	}
	return sb.String()
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
	// spare is what the trailing column falls back to when the full readout
	// will not fit beside the aligned label column: for a finished stage the
	// note is a nicety and the elapsed time is the record of the work, so the
	// row gives up the note first and the clock only if it has to.
	var icon, label, trail, sub, spare string
	switch s.status {
	case "done":
		// The completion moment. A stage that swaps a sweeping bar for a tick
		// between two frames is a change nobody watching from a room away
		// catches, so a finished row holds a full gradient bar for half a
		// second, then lands lit, then settles. The row never changes width
		// while it does it.
		lit := m.flash(s)
		icon, label = Good.Render("✔"), Body.Render(s.label)
		clk := Dim.Render(dur(s.elapsed))
		if lit > 0 {
			icon, label, clk = Title.Render("✔"), Title.Render(s.label), Muted.Render(dur(s.elapsed))
		}
		spare = clk
		if s.detail != "" && clk != "" {
			trail = Muted.Render(s.detail) + "  "
		} else if s.detail != "" {
			trail = Muted.Render(s.detail)
		}
		trail += clk
		if lit > 1 {
			// The bar snaps full where the pulse was sweeping, on exactly the
			// budget the pulse had to clear — so the fill lands in place
			// rather than jumping to a different column.
			if bar := meterWidth(width); width-labelCol-4 >= bar+2+lipgloss.Width(clk) {
				trail = Meter(bar, 1)
				if clk != "" {
					trail += "  " + clk
				}
			}
		}
	case "running":
		icon, label = m.spin.View(), Title.Render(s.label)
		// The stopwatch runs off startedAt, so it ticks with the spinner
		// rather than waiting on the backend to say something. It is the one
		// thing this row never gives up: a running stage with no elapsed time
		// is indistinguishable from a hang, which is the whole reason the
		// trailing column exists.
		watch := Dim.Render(stopwatch(m.since(s)))
		// room is what the trailing column may occupy without crowding the
		// aligned label column — the same budget the guard below enforces.
		room := width - labelCol - 4
		trail = watch
		switch {
		case s.detail == "":
		case withSub:
			sub = Dim.Render("↳ ") + Muted.Render(s.detail)
		default:
			// No room for a second line: the note is worth more than the
			// pulse, so it takes the trailing column back — clipped to what
			// is left beside the clock. A note wider than the row used to
			// blow the whole column past the guard and take the clock with
			// it, leaving a row that said nothing at all.
			if n := room - lipgloss.Width(watch) - 2; n > 0 {
				if d := ansi.Truncate(s.detail, n, "…"); d != "" {
					trail = Muted.Render(d) + "  " + watch
				}
			}
		}
		// The pulse is the first thing surrendered: it is decoration, so it
		// only appears once the clock has a bar's width to spare beside it.
		if s.detail == "" || withSub {
			if bar := meterWidth(width); room >= bar+2+lipgloss.Width(watch) {
				trail = Pulse(bar, m.frame) + "  " + watch
			}
		}
	case "failed":
		icon, label = Warn.Render("▲"), Body.Render(s.label)
		trail = Warn.Render("degraded")
	default:
		// Six rows all reading "queued" is six rows of noise. The queue says
		// what it is by being dim; only its front is worth a word.
		icon, label = Dim.Render("·"), Dim.Render(s.label)
		if s.key == m.nextKey {
			icon, label, trail = Muted.Render("·"), Muted.Render(s.label), Dim.Render("next")
		}
	}

	left := icon + "  " + label
	if trail == "" {
		return left
	}
	row := Spread(width, left, trail)
	switch {
	case s.status == "running":
		// The clock outranks the label's tail. A clipped label still reads as
		// the stage it names; a running row with nothing after it reads as a
		// hang, so this row clips the left rather than dropping the right.
		if over := lipgloss.Width(left) + 2 + lipgloss.Width(trail) - width; over > 0 {
			left = ansi.Truncate(left, max(lipgloss.Width(left)-over, 0), "…")
			row = Spread(width, left, trail)
		}
	case width < labelCol+lipgloss.Width(trail)+4:
		// Below the aligned label column the row would overlap, so the
		// trailing readout gives way — to the shorter one it kept in reserve
		// if there is one, and to nothing if even that will not fit.
		row = left
		if spare != "" && width >= labelCol+lipgloss.Width(spare)+4 {
			row = Spread(width, left, spare)
		}
	}
	if sub != "" {
		row += "\n   " + sub
	}
	return row
}

// settled reports whether a status is one a stage stops moving in.
func settled(status string) bool { return status == "done" || status == "failed" }

// flash reports how far into the completion transition a settled row is: 2
// while its bar holds full, 1 while the row is still lit, 0 once it has cooled
// into the finished list. Driven off the clock rather than a frame count, so
// the transition is the same length however the terminal is repainting.
func (m model) flash(s stage) int {
	switch d := time.Since(s.settledAt); {
	case s.settledAt.IsZero():
		return 0
	case d < 450*time.Millisecond:
		return 2
	case d < 900*time.Millisecond:
		return 1
	}
	return 0
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
