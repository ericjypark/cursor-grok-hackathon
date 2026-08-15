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

	"github.com/ericjypark/cursor-grok-hackathon/internal/client"
)

// Layout constants. labelCol keeps every stage's trailing column aligned no
// matter which label is longest.
const (
	frameWidth = 60
	labelCol   = 26
	barWidth   = 14
)

type stage struct {
	key, label, detail, status string
	startedAt                  time.Time
	elapsed                    time.Duration
}

// Display order, which is also roughly execution order.
var stageOrder = []stage{
	{key: "map", label: "Mapping the site"},
	{key: "scrape_site", label: "Reading key pages"},
	{key: "scrape_repo", label: "Reading the repository"},
	{key: "search_collisions", label: "Hunting name collisions"},
	{key: "find_similar", label: "Finding adjacent products"},
	{key: "search_context", label: "Confirming vocabulary"},
	{key: "synthesize", label: "Synthesizing the dossier"},
}

type eventMsg client.Event
type closedMsg struct{}

type model struct {
	stages   []stage
	spin     spinner.Model
	events   <-chan client.Event
	result   *client.ProductDossier
	warnings []string
	fatal    error
	quitting bool

	frame   int
	startAt time.Time
	width   int
}

func newModel(events <-chan client.Event) model {
	s := spinner.New()
	// Frames without the trailing space bubbles ships on Dot, so a running row
	// occupies exactly the same width as a settled one and nothing jitters.
	s.Spinner = spinner.Spinner{
		Frames: []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"},
		FPS:    time.Second / 12,
	}

	stages := make([]stage, len(stageOrder))
	copy(stages, stageOrder)
	for i := range stages {
		stages[i].status = "pending"
	}
	return model{stages: stages, spin: s, events: events, startAt: time.Now(), width: frameWidth}
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
		m.width = min(frameWidth, max(32, msg.Width-4))
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
		case "result":
			if d, err := ev.Result(); err == nil {
				m.result = &d
			} else {
				m.fatal = fmt.Errorf("malformed result event: %w", err)
			}
			return m, tea.Quit
		}
		return m, waitFor(m.events)
	case closedMsg:
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
	var sb strings.Builder
	sb.WriteString("\n" + indent(Banner(m.width)) + "\n\n")

	done := 0
	for _, s := range m.stages {
		if s.status == "done" {
			done++
		}
		sb.WriteString("   " + m.stageLine(s) + "\n")
	}

	sb.WriteString("\n  " + Rule(m.width) + "\n")
	sb.WriteString("   " + Meter(barWidth, float64(done)/float64(len(m.stages))))
	sb.WriteString("  " + Muted.Render(fmt.Sprintf("%d/%d", done, len(m.stages))))
	sb.WriteString("  " + Dim.Render(clock(time.Since(m.startAt))) + "\n")

	for _, w := range m.warnings {
		sb.WriteString("\n   " + Warn.Render("▲ "+w))
	}
	if m.fatal != nil {
		sb.WriteString("\n   " + Bad.Render("✗ "+m.fatal.Error()) + "\n")
	}
	return sb.String() + "\n"
}

// stageLine draws one row: status glyph, label, then a trailing column that is
// a live pulse while running and a timing/detail readout once settled.
func (m model) stageLine(s stage) string {
	var icon, label, trail string
	switch s.status {
	case "done":
		icon, label = Good.Render("✔"), Body.Render(s.label)
		trail = Dim.Render(dur(s.elapsed))
		if s.detail != "" {
			trail += "  " + Muted.Render(s.detail)
		}
	case "running":
		icon, label = m.spin.View(), Title.Render(s.label)
		if s.detail != "" {
			trail = Muted.Render(s.detail)
		} else {
			trail = Pulse(barWidth, m.frame)
		}
	case "failed":
		icon, label = Warn.Render("▲"), Body.Render(s.label)
		trail = Warn.Render("degraded")
	default:
		icon, label = Dim.Render("·"), Dim.Render(s.label)
	}

	line := icon + "  " + label
	if trail == "" {
		return line
	}
	pad := labelCol - lipgloss.Width(s.label)
	if pad < 1 {
		pad = 1
	}
	return line + strings.Repeat(" ", pad) + trail
}

func indent(block string) string {
	lines := strings.Split(block, "\n")
	for i := range lines {
		lines[i] = "  " + lines[i]
	}
	return strings.Join(lines, "\n")
}

func dur(d time.Duration) string {
	if d == 0 {
		return ""
	}
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return fmt.Sprintf("%.1fs", d.Seconds())
}

func clock(d time.Duration) string {
	return fmt.Sprintf("%02d:%02d", int(d.Minutes()), int(d.Seconds())%60)
}

// Run drives the progress display until the stream ends, returning the dossier.
func Run(ctx context.Context, events <-chan client.Event) (client.ProductDossier, error) {
	final, err := tea.NewProgram(newModel(events), tea.WithContext(ctx)).Run()
	if err != nil {
		return client.ProductDossier{}, err
	}
	m, ok := final.(model)
	if !ok {
		return client.ProductDossier{}, errors.New("unexpected terminal model")
	}
	if m.fatal != nil {
		return client.ProductDossier{}, m.fatal
	}
	if m.result == nil {
		return client.ProductDossier{}, errors.New("no dossier was produced")
	}
	return *m.result, nil
}
