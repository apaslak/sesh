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
		return "○ idle", lipgloss.Color("#9CA3AF")
	case agentstatus.Working:
		return "● working", lipgloss.Color("#FBBF24")
	case agentstatus.Attention:
		return "! attention", lipgloss.Color("#F87171")
	case agentstatus.Finished:
		if status.Read {
			return "✓ read", lipgloss.Color("#86EFAC")
		}
		return "◆ unread", lipgloss.Color("#67E8F9")
	default:
		return "? unavailable", lipgloss.Color("#9CA3AF")
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

// renderTreeRow reserves independent status/title/path columns on wide screens,
// and puts the path on a second line when those columns would crowd the title.
func renderTreeRow(row treeRow, width int, home string, selected bool) []string {
	width = max(0, width)
	lines := []string{}
	if row.agent == nil {
		lines = append(lines, ansi.Truncate("  "+row.label, width, "…"))
	} else {
		indicator, color := agentIndicator(row.agent.status)
		badge := lipgloss.NewStyle().Foreground(color).Width(15).Render(indicator)
		path := agentDisplayPath(row.agent.directory, home)
		if width >= 80 {
			pathWidth := min(max(20, ansi.StringWidth(path)), width/3)
			nameWidth := max(1, width-2-15-2-pathWidth-2)
			name := lipgloss.NewStyle().Width(nameWidth).Render(ansi.Truncate(row.label, nameWidth, "…"))
			lines = append(lines, "  "+badge+"  "+name+"  "+ansi.Truncate(path, pathWidth, "…"))
		} else {
			lines = append(lines, ansi.Truncate("  "+badge+"  "+row.label, width, "…"))
			lines = append(lines, ansi.Truncate("  "+path, width, "…"))
		}
	}
	if selected {
		for i := range lines {
			lines[i] = lipgloss.NewStyle().Reverse(true).Render(lines[i])
		}
	}
	return lines
}
