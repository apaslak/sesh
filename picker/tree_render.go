package picker

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/joshmedeski/sesh/v2/agentstatus"
)

// agentIndicator keeps every state distinguishable without relying on color.
func agentIndicator(status agentstatus.Status) (string, color.Color) {
	switch status := status.(type) {
	case agentstatus.Idle:
		return "○ idle", lipgloss.Color("#a5adcb")
	case agentstatus.Working:
		return "● working", lipgloss.Color("#eed49f")
	case agentstatus.Attention:
		return "! attention", lipgloss.Color("#ed8796")
	case agentstatus.Finished:
		if status.Read {
			return "✓ read", lipgloss.Color("#a6da95")
		}
		return "◆ unread", lipgloss.Color("#8bd5ca")
	default:
		return "? unavailable", lipgloss.Color("#a5adcb")
	}
}

func agentDisplayPath(directory, home string) string {
	if directory == "" {
		return "path unavailable"
	}
	if home != "" {
		if directory == home {
			return "~"
		}
		if strings.HasPrefix(directory, home+"/") {
			return "~" + strings.TrimPrefix(directory, home)
		}
	}
	return directory
}

// renderTreeRow keeps status, title, and a muted right-aligned path on one line.
// Every segment shares the selection background, including padding and path.
func renderTreeRow(row treeRow, width int, home string, selected bool) []string {
	width = max(0, width)
	segment := func(text string, foreground color.Color) string {
		style := lipgloss.NewStyle().Foreground(foreground)
		if selected {
			style = style.Background(lipgloss.Color("#363a4f"))
		}
		return style.Render(text)
	}
	finish := func(line string) string {
		line = ansi.Truncate(line, width, "…")
		return line + segment(strings.Repeat(" ", max(0, width-ansi.StringWidth(line))), lipgloss.Color("#cad3f5"))
	}
	marker := "  "
	if selected {
		marker = "› "
	}
	prefix := segment(" "+marker, lipgloss.Color("#c6a0f6"))
	if row.agent == nil {
		return []string{finish(prefix + segment(row.label, lipgloss.Color("#cad3f5")))}
	}
	indicator, statusColor := agentIndicator(row.agent.status)
	badge := indicator + strings.Repeat(" ", max(0, 13-ansi.StringWidth(indicator)))
	prefix += segment(badge+"  ", statusColor)
	rightInset := min(1, width)
	room := max(0, width-ansi.StringWidth(prefix)-rightInset)
	// Split tight space between title/path; a short title lends its unused
	// budget to the path, and a short path leaves more room for the title.
	reservedTitle := min(ansi.StringWidth(row.label), max(1, (room-2)/2))
	path := ansi.Truncate(agentDisplayPath(row.agent.directory, home), max(0, room-2-reservedTitle), "…")
	gap := 0
	if path != "" {
		gap = 2
	}
	title := ansi.Truncate(row.label, max(0, room-ansi.StringWidth(path)-gap), "…")
	line := prefix + segment(title, lipgloss.Color("#cad3f5"))
	padding := strings.Repeat(" ", max(0, width-ansi.StringWidth(line)-ansi.StringWidth(path)-rightInset))
	line += segment(padding+path+strings.Repeat(" ", rightInset), lipgloss.Color("#a5adcb"))
	return []string{finish(line)}
}
