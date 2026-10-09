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

// renderTreeRow groups each agent's title and path into two lines, applying the
// same selection background to every segment so nested colors cannot erase it.
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
	first := prefix + segment(badge+"  ", statusColor) + segment(row.label, lipgloss.Color("#cad3f5"))
	second := segment("   "+agentDisplayPath(row.agent.directory, home), lipgloss.Color("#a5adcb"))
	return []string{finish(first), finish(second)}
}
