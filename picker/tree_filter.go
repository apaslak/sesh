package picker

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/charmbracelet/x/ansi"
	"github.com/joshmedeski/sesh/v2/tmux"
	"github.com/sahilm/fuzzy"
)

type treeRow struct {
	key, parent, label string
	depth              int
	branch             bool
	selection          TreeSelection
}

func safeTreeText(text string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, ansi.Strip(text))
}

func buildTreeRows(data TreeData) []treeRow {
	rows := []treeRow{}
	for _, session := range data.Topology.Sessions {
		rows = append(rows, treeRow{key: session.ID, label: safeTreeText(session.Name), branch: true, selection: TreeTmuxSelection{Target: tmux.SessionTarget{Session: session.ID}}})
		for _, window := range session.Windows {
			wk := session.ID + "/" + window.ID
			rows = append(rows, treeRow{key: wk, parent: session.ID, depth: 1, label: fmt.Sprintf("%d: %s", window.Index, safeTreeText(window.Name)), branch: true, selection: TreeTmuxSelection{Target: tmux.WindowTarget{Session: session.ID, Window: window.ID}}})
			for _, pane := range window.Panes {
				label := fmt.Sprintf("%d: %s  %s", pane.Index, safeTreeText(pane.Command), safeTreeText(pane.Path))
				if status, ok := data.Statuses[pane.ID]; ok && status.Status != nil {
					label = fmt.Sprintf("%d: [%s] %s  %s  %s", pane.Index, safeTreeText(status.Status.Label()), safeTreeText(status.Agent), safeTreeText(status.Title), safeTreeText(pane.Path))
				} else if strings.Contains(strings.ToLower(pane.Command), "opencode") {
					label += "  [status unavailable]"
				}
				rows = append(rows, treeRow{key: wk + "/" + pane.ID, parent: wk, depth: 2, label: label, selection: TreeTmuxSelection{Target: tmux.PaneTarget{Session: session.ID, Window: window.ID, Pane: pane.ID}}})
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

func filterTreeRows(rows []treeRow, query string, collapsed map[string]bool) []treeRow {
	keep := make(map[string]bool)
	if query != "" {
		labels := make([]string, len(rows))
		for i := range rows {
			labels[i] = rows[i].label
		}
		for _, match := range fuzzy.Find(query, labels) {
			row := rows[match.Index]
			keep[row.key] = true
			for _, candidate := range rows {
				if (candidate.branch && strings.HasPrefix(row.key, candidate.key+"/")) || (row.branch && strings.HasPrefix(candidate.key, row.key+"/")) {
					keep[candidate.key] = true
				}
			}
		}
	}
	filtered := []treeRow{}
	for _, row := range rows {
		if query != "" {
			if keep[row.key] {
				filtered = append(filtered, row)
			}
			continue
		}
		hidden := false
		for key, value := range collapsed {
			if value && strings.HasPrefix(row.key, key+"/") {
				hidden = true
				break
			}
		}
		if !hidden {
			filtered = append(filtered, row)
		}
	}
	return filtered
}
