package ui

import (
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
)

// FormTheme dresses the huh prompts in the same ramp the progress view uses:
// a gradient-anchored bar marks the focused field, blurred fields recede to
// the dim gutter color.
func FormTheme() *huh.Theme {
	t := huh.ThemeBase()

	t.Focused.Base = t.Focused.Base.
		BorderStyle(lipgloss.ThickBorder()).BorderLeft(true).
		BorderForeground(RampAt(0)).PaddingLeft(1)
	t.Focused.Title = Title
	t.Focused.Description = Dim
	t.Focused.ErrorIndicator = Bad.SetString(" ▲")
	t.Focused.ErrorMessage = Bad
	t.Focused.TextInput.Prompt = Accent
	t.Focused.TextInput.Cursor = lipgloss.NewStyle().Foreground(RampAt(1))
	t.Focused.TextInput.Placeholder = Dim
	t.Focused.TextInput.Text = Body

	t.Blurred.Base = t.Focused.Base.BorderForeground(lipgloss.Color("#3F3F46"))
	t.Blurred.Title = Muted
	t.Blurred.Description = Dim
	t.Blurred.TextInput.Prompt = Dim
	t.Blurred.TextInput.Placeholder = Dim
	t.Blurred.TextInput.Text = Muted

	return t
}

// FormHeader is the slim lead-in above the prompts. The full wordmark is saved
// for the progress view, so the two never compete in one scrollback.
func FormHeader() string {
	return "\n  " + Gradient("◆", true) + "  " + Title.Render("fieldnote") +
		"  " + Dim.Render("· tell us what to look at") + "\n  " + Rule(frameWidth) + "\n"
}
