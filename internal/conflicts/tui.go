// Copyright (C) 2026 DaemonHound Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package conflicts

import (
	"fmt"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	docStyle   = lipgloss.NewStyle().Margin(1, 2)
	titleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("230")).
			Background(lipgloss.Color("63")).
			Padding(0, 1)
	selectedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("170")).
			Bold(true)
	localStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))  // Green
	remoteStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("208")) // Orange
	descStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("243"))
)

// ConflictItem wraps a Conflict for list display.
type ConflictItem struct {
	conflict *Conflict
}

func (i ConflictItem) FilterValue() string {
	return i.conflict.FilePath
}

func (i ConflictItem) Title() string {
	status := "⚠️  PENDING"
	if i.conflict.ResolvedAt != nil {
		status = "✓ RESOLVED"
	}
	return fmt.Sprintf("%s - %s", i.conflict.FilePath, status)
}

func (i ConflictItem) Description() string {
	if i.conflict.ResolvedAt != nil {
		return fmt.Sprintf("Resolved with '%s' strategy on %s", i.conflict.ResolutionStrategy, i.conflict.ResolvedAt.Format("2006-01-02 15:04"))
	}
	return fmt.Sprintf("Detected on %s", i.conflict.DetectedAt.Format("2006-01-02 15:04"))
}

// ListModel displays a list of conflicts.
type ListModel struct {
	list list.Model
}

// NewListModel creates a conflict list UI.
func NewListModel(conflicts []*Conflict) *ListModel {
	items := make([]list.Item, len(conflicts))
	for i, c := range conflicts {
		items[i] = ConflictItem{conflict: c}
	}

	l := list.New(items, list.NewDefaultDelegate(), 0, 0)
	l.Title = "Conflicts"
	l.SetShowStatusBar(true)
	l.SetFilteringEnabled(true)
	l.SetShowPagination(true)

	return &ListModel{list: l}
}

func (m *ListModel) Init() tea.Cmd {
	return nil
}

func (m *ListModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		}

	case tea.WindowSizeMsg:
		h, v := docStyle.GetFrameSize()
		m.list.SetSize(msg.Width-h, msg.Height-v)
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m *ListModel) View() string {
	return docStyle.Render(m.list.View())
}

// DetailModel shows conflict details with resolution options.
type DetailModel struct {
	conflict *Conflict
	selected int // 0 = local, 1 = remote, 2 = show hashes
}

// NewDetailModel creates a conflict detail viewer.
func NewDetailModel(conflict *Conflict) *DetailModel {
	return &DetailModel{
		conflict: conflict,
		selected: 0,
	}
}

func (m *DetailModel) Init() tea.Cmd {
	return nil
}

func (m *DetailModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "up", "k":
			if m.selected > 0 {
				m.selected--
			}
		case "down", "j":
			if m.selected < 2 {
				m.selected++
			}
		case "enter":
			// Return selected option
			switch m.selected {
			case 0:
				return m, func() tea.Msg { return ResolutionMsg{strategy: "local"} }
			case 1:
				return m, func() tea.Msg { return ResolutionMsg{strategy: "remote"} }
			}
		}
	case tea.WindowSizeMsg:
		// Handle window resize
	}

	return m, nil
}

func (m *DetailModel) View() string {
	status := "⚠️  PENDING RESOLUTION"
	if m.conflict.ResolvedAt != nil {
		status = fmt.Sprintf("✓ RESOLVED with '%s' strategy", m.conflict.ResolutionStrategy)
	}

	header := titleStyle.Render(fmt.Sprintf("Conflict: %s", m.conflict.FilePath))
	subheader := descStyle.Render(status)

	// Local version
	localLabel := localStyle.Render("► KEEP LOCAL (this machine)")
	if m.selected == 0 && m.conflict.ResolvedAt == nil {
		localLabel = selectedStyle.Render(localLabel)
	}
	localContent := descStyle.Render(fmt.Sprintf("Hash: %s\nContent:\n%s", m.conflict.LocalHash, m.conflict.LocalContent))

	// Remote version
	remoteLabel := remoteStyle.Render("► USE REMOTE (other machine)")
	if m.selected == 1 && m.conflict.ResolvedAt == nil {
		remoteLabel = selectedStyle.Render(remoteLabel)
	}
	remoteContent := descStyle.Render(fmt.Sprintf("Hash: %s\nContent:\n%s", m.conflict.RemoteHash, m.conflict.RemoteContent))

	// Help text
	help := descStyle.Render("\n[↑↓] Navigate | [Enter] Select | [q] Back")
	if m.conflict.ResolvedAt != nil {
		help = descStyle.Render("\nAlready resolved. Press [q] to go back.")
	}

	return docStyle.Render(
		fmt.Sprintf("%s\n%s\n\n%s\n%s\n\n%s\n%s\n%s",
			header, subheader,
			localLabel, localContent,
			remoteLabel, remoteContent,
			help,
		),
	)
}

// ResolutionMsg is sent when user selects a resolution strategy.
type ResolutionMsg struct {
	strategy string
}
