package picker

import (
	"strings"
	"unicode"

	"github.com/charmbracelet/x/ansi"
	"github.com/joshmedeski/sesh/v2/agentstatus"
	"github.com/joshmedeski/sesh/v2/tmux"
	"github.com/sahilm/fuzzy"
)

// agentRow contains conversation metadata; a nil status means unavailable,
// never idle. A missing directory is not replaced with the pane's working path.
type agentRow struct {
	status    agentstatus.Status
	directory string
}

type treeRow struct {
	key, label string
	selection  TreeSelection
	agent      *agentRow
}

func safeTreeText(text string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, ansi.Strip(text))
}

func buildTreeRows(data TreeData, view tmuxView) []treeRow {
	rows := []treeRow{}
	for _, session := range data.Topology.Sessions {
		if view == sessionsView {
			if !sessionHasOpenCode(session, data.Statuses) {
				rows = append(rows, treeRow{key: session.ID, label: safeTreeText(session.Name), selection: TreeTmuxSelection{Target: tmux.SessionTarget{Session: session.ID}}})
			}
			continue
		}
		for _, window := range session.Windows {
			for _, pane := range window.Panes {
				status := data.Statuses[pane.ID]
				if !paneHasOpenCode(pane, status) {
					continue
				}
				title, directory := "OpenCode (title unavailable)", ""
				if status.Status != nil {
					title = status.Title
					if title == "" {
						title = status.Conversation
					}
					if title == "" {
						title = "OpenCode (title unavailable)"
					}
					directory = status.Directory
				}
				rows = append(rows, treeRow{
					key:       session.ID + "/" + window.ID + "/" + pane.ID,
					label:     safeTreeText(session.Name) + " > " + safeTreeText(title),
					selection: TreeTmuxSelection{Target: tmux.PaneTarget{Session: session.ID, Window: window.ID, Pane: pane.ID}},
					agent:     &agentRow{status: status.Status, directory: safeTreeText(directory)},
				})
			}
		}
	}
	for _, key := range data.Entries.OrderedIndex {
		entry, exists := data.Entries.Directory[key]
		if !exists || entry.Src == "tmux" {
			continue
		}
		rows = append(rows, treeRow{key: "entry:" + entry.Src + ":" + key, label: "[" + safeTreeText(entry.Src) + "] " + safeTreeText(entry.Name), selection: TreeNameSelection{Name: entry.Name}})
	}
	return rows
}

// paneHasOpenCode uses the same detection for both mutually exclusive views.
func paneHasOpenCode(pane tmux.TreePane, status agentstatus.Snapshot) bool {
	return status.Status != nil || strings.Contains(strings.ToLower(pane.Command), "opencode")
}

func sessionHasOpenCode(session tmux.TreeSession, statuses map[string]agentstatus.Snapshot) bool {
	for _, window := range session.Windows {
		for _, pane := range window.Panes {
			if paneHasOpenCode(pane, statuses[pane.ID]) {
				return true
			}
		}
	}
	return false
}

func filterTreeRows(rows []treeRow, query string) []treeRow {
	if query == "" {
		return rows
	}
	labels := make([]string, len(rows))
	for i, row := range rows {
		labels[i] = row.label
		if row.agent != nil {
			indicator, _ := agentIndicator(row.agent.status)
			labels[i] += " " + row.agent.directory + " " + indicator
			if row.agent.status != nil {
				labels[i] += " " + row.agent.status.Label()
			}
		}
	}
	keep := make([]bool, len(rows))
	for _, match := range fuzzy.Find(query, labels) {
		keep[match.Index] = true
	}
	filtered := []treeRow{}
	for i, row := range rows {
		if keep[i] {
			filtered = append(filtered, row)
		}
	}
	return filtered
}
