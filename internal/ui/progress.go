// Package ui renders live pipeline progress while the backend works.
package ui

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ericjypark/cursor-grok-hackathon/internal/client"
)

var (
	dimStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	doneStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	warnStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	errStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	titleStyle = lipgloss.NewStyle().Bold(true)
)

type stage struct {
	key, label, detail, status string
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
}

func newModel(events <-chan client.Event) model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))

	stages := make([]stage, len(stageOrder))
	copy(stages, stageOrder)
	for i := range stages {
		stages[i].status = "pending"
	}
	return model{stages: stages, spin: s, events: events}
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
		if m.stages[i].key == key {
			m.stages[i].status = status
			if detail != "" {
				m.stages[i].detail = detail
			}
			return
		}
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.Type == tea.KeyCtrlC || msg.String() == "q" {
			m.quitting = true
			m.fatal = errors.New("cancelled")
			return m, tea.Quit
		}
	case spinner.TickMsg:
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

func (m model) View() string {
	var sb strings.Builder
	sb.WriteString(titleStyle.Render("Understanding the product") + "\n\n")

	for _, s := range m.stages {
		var icon, label string
		switch s.status {
		case "done":
			icon, label = doneStyle.Render("✓"), s.label
		case "running":
			icon, label = m.spin.View(), s.label
		case "failed":
			icon, label = warnStyle.Render("!"), s.label
		default:
			icon, label = dimStyle.Render("·"), dimStyle.Render(s.label)
		}
		line := fmt.Sprintf("  %s %s", icon, label)
		if s.detail != "" && s.status != "pending" {
			line += dimStyle.Render("  " + s.detail)
		}
		sb.WriteString(line + "\n")
	}

	for _, w := range m.warnings {
		sb.WriteString(warnStyle.Render("  ! "+w) + "\n")
	}
	if m.fatal != nil {
		sb.WriteString("\n" + errStyle.Render("  ✗ "+m.fatal.Error()) + "\n")
	}
	return sb.String() + "\n"
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
