package repopicker

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/dlvhdr/gh-dash/v4/internal/tui/context"
)

// RepoOption represents a selectable repository option
type RepoOption struct {
	Label string // Display label (e.g., "My Fork", "Upstream", "All Repos")
	Value string // The repo value (e.g., "owner/repo" or empty for no filter)
	Desc  string // Optional description
}

// Model is the repo picker component
type Model struct {
	ctx           *context.ProgramContext
	options       []RepoOption
	cursor        int
	customInput   textinput.Model
	isCustomMode  bool
	width         int
	focused       bool
	selectedValue string
}

// KeyMap defines keybindings for the picker
type KeyMap struct {
	Up       key.Binding
	Down     key.Binding
	Select   key.Binding
	Cancel   key.Binding
	Custom   key.Binding
}

// DefaultKeyMap returns the default keybindings
func DefaultKeyMap() KeyMap {
	return KeyMap{
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑/k", "up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("↓/j", "down"),
		),
		Select: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "select"),
		),
		Cancel: key.NewBinding(
			key.WithKeys("esc", "ctrl+c"),
			key.WithHelp("esc", "cancel"),
		),
		Custom: key.NewBinding(
			key.WithKeys("c"),
			key.WithHelp("c", "custom repo"),
		),
	}
}

var Keys = DefaultKeyMap()

// RepoSelectedMsg is sent when a repo is selected
type RepoSelectedMsg struct {
	Value    string // The selected repo (e.g., "owner/repo") or empty for no filter
	IsCustom bool   // True if this was a custom entry
}

// RepoCancelledMsg is sent when the picker is cancelled
type RepoCancelledMsg struct{}

// NewModel creates a new repo picker model
func NewModel(ctx *context.ProgramContext) Model {
	ti := textinput.New()
	ti.Placeholder = "owner/repo"
	ti.CharLimit = 100
	ti.Width = 40

	return Model{
		ctx:          ctx,
		options:      []RepoOption{},
		cursor:       0,
		customInput:  ti,
		isCustomMode: false,
		width:        50,
		focused:      false,
	}
}

// SetOptions sets the available repo options
func (m *Model) SetOptions(options []RepoOption) {
	m.options = options
	m.cursor = 0
}

// SetWidth sets the picker width
func (m *Model) SetWidth(w int) {
	m.width = w
	m.customInput.Width = w - 10
}

// Focus focuses the picker
func (m *Model) Focus() {
	m.focused = true
	m.isCustomMode = false
	m.cursor = 0
}

// Blur blurs the picker
func (m *Model) Blur() {
	m.focused = false
	m.isCustomMode = false
	m.customInput.Blur()
}

// Focused returns whether the picker is focused
func (m Model) Focused() bool {
	return m.focused
}

// SetSelectedValue sets the currently selected value (for highlighting)
func (m *Model) SetSelectedValue(value string) {
	m.selectedValue = value
	// Try to find and select the matching option
	for i, opt := range m.options {
		if opt.Value == value {
			m.cursor = i
			break
		}
	}
}

// Update handles messages
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	if !m.focused {
		return m, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.isCustomMode {
			switch {
			case key.Matches(msg, Keys.Cancel):
				m.isCustomMode = false
				m.customInput.Blur()
				m.customInput.SetValue("")
				return m, nil
			case key.Matches(msg, Keys.Select):
				value := strings.TrimSpace(m.customInput.Value())
				if value != "" {
					m.focused = false
					m.isCustomMode = false
					return m, func() tea.Msg {
						return RepoSelectedMsg{Value: value, IsCustom: true}
					}
				}
				return m, nil
			default:
				var cmd tea.Cmd
				m.customInput, cmd = m.customInput.Update(msg)
				return m, cmd
			}
		}

		switch {
		case key.Matches(msg, Keys.Up):
			if m.cursor > 0 {
				m.cursor--
			}
		case key.Matches(msg, Keys.Down):
			if m.cursor < len(m.options)-1 {
				m.cursor++
			}
		case key.Matches(msg, Keys.Select):
			if len(m.options) > 0 {
				selected := m.options[m.cursor]
				m.focused = false
				return m, func() tea.Msg {
					return RepoSelectedMsg{Value: selected.Value, IsCustom: false}
				}
			}
		case key.Matches(msg, Keys.Cancel):
			m.focused = false
			return m, func() tea.Msg {
				return RepoCancelledMsg{}
			}
		case key.Matches(msg, Keys.Custom):
			m.isCustomMode = true
			m.customInput.Focus()
			return m, textinput.Blink
		}
	}

	return m, nil
}

// View renders the picker
func (m Model) View() string {
	if !m.focused {
		return ""
	}

	var b strings.Builder

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(m.ctx.Theme.PrimaryText).
		MarginBottom(1)

	b.WriteString(titleStyle.Render("Select Repository Filter"))
	b.WriteString("\n\n")

	if m.isCustomMode {
		b.WriteString("Enter custom repo (owner/repo):\n")
		b.WriteString(m.customInput.View())
		b.WriteString("\n\n")
		b.WriteString(lipgloss.NewStyle().Faint(true).Render("Press Enter to confirm, Esc to cancel"))
	} else {
		for i, opt := range m.options {
			cursor := "  "
			style := lipgloss.NewStyle().Foreground(m.ctx.Theme.FaintText)

			if i == m.cursor {
				cursor = "> "
				style = lipgloss.NewStyle().
					Foreground(m.ctx.Theme.PrimaryText).
					Bold(true)
			}

			// Mark the currently active option
			marker := ""
			if opt.Value == m.selectedValue {
				marker = " (current)"
			}

			line := fmt.Sprintf("%s%s%s", cursor, opt.Label, marker)
			b.WriteString(style.Render(line))

			if opt.Desc != "" {
				descStyle := lipgloss.NewStyle().
					Foreground(m.ctx.Theme.FaintText).
					Italic(true)
				b.WriteString(descStyle.Render(fmt.Sprintf(" - %s", opt.Desc)))
			}
			b.WriteString("\n")
		}

		b.WriteString("\n")
		helpStyle := lipgloss.NewStyle().Faint(true)
		b.WriteString(helpStyle.Render("↑/↓: navigate • Enter: select • c: custom • Esc: cancel"))
	}

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(m.ctx.Theme.PrimaryBorder).
		Padding(1, 2).
		Width(m.width)

	return boxStyle.Render(b.String())
}

// UpdateProgramContext updates the context
func (m *Model) UpdateProgramContext(ctx *context.ProgramContext) {
	m.ctx = ctx
}
