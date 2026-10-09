package picker

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/joshmedeski/sesh/v2/agentstatus"
	"github.com/joshmedeski/sesh/v2/model"
	"github.com/joshmedeski/sesh/v2/tmux"
	"github.com/stretchr/testify/require"
)

func treeFixture() TreeData {
	return TreeData{Topology: tmux.TreeSnapshot{Server: "s", Sessions: []tmux.TreeSession{{ID: "$1", Name: "work", Windows: []tmux.TreeWindow{{ID: "@1", Index: 0, Name: "main", Panes: []tmux.TreePane{{ID: "%1", Command: "zsh", Path: "/work"}, {ID: "%2", Index: 1, Command: "opencode", Path: "/work"}}}}}}}, Statuses: map[string]agentstatus.Snapshot{"%2": {Title: "Review payments", Agent: "build", Status: agentstatus.Finished{Execution: "1"}}}}
}

func TestTreeFilteringKeepsAncestors(t *testing.T) {
	rows := buildTreeRows(treeFixture())
	require.Len(t, rows, 4)
	filtered := filterTreeRows(rows, "payments", nil)
	require.Len(t, filtered, 3)
	require.Equal(t, "$1", filtered[0].key)
	require.Equal(t, "$1/@1", filtered[1].key)
	require.Equal(t, "$1/@1/%2", filtered[2].key)
	require.Len(t, filterTreeRows(rows, "work", nil), 4)
	require.Len(t, filterTreeRows(rows, "", map[string]bool{"$1": true}), 1)
	require.Len(t, filterTreeRows(rows, "payments", map[string]bool{"$1": true}), 3)
}

func TestTreeRefreshPreservesSelection(t *testing.T) {
	m := NewTreeModel(nil, "")
	m.accept(treeFixture())
	m.cursor = 3
	m.accept(treeFixture())
	require.Equal(t, "$1/@1/%2", m.rows[m.cursor].key)
	data := treeFixture()
	data.Topology.Sessions[0].Windows[0].Panes = data.Topology.Sessions[0].Windows[0].Panes[:1]
	// Keep another conversation in this session, so its window remains visible.
	data.Statuses["%1"] = agentstatus.Snapshot{Title: "Remaining agent", Status: agentstatus.Idle{}}
	m.accept(data)
	require.Equal(t, "$1/@1", m.rows[m.cursor].key)
}

func TestTreeSourceShortcutsAndTypedSelection(t *testing.T) {
	for _, tc := range []struct {
		key    rune
		source TreeSource
	}{{'a', TreeAll}, {'t', TreeTmux}, {'g', TreeConfig}, {'x', TreeZoxide}, {'f', TreeFind}} {
		m := NewTreeModel(nil, "")
		result, _ := m.Update(tea.KeyPressMsg{Code: tc.key, Mod: tea.ModCtrl})
		switch next := result.(type) {
		case TreeModel:
			require.Equal(t, tc.source, next.source)
		default:
			t.Fatal("wrong model")
		}
	}
	m := NewTreeModel(nil, "")
	m.accept(treeFixture())
	m.cursor = 3
	result, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	switch next := result.(type) {
	case TreeModel:
		selection, picked := next.Chosen()
		require.True(t, picked)
		switch selected := selection.(type) {
		case TreeTmuxSelection:
			require.Equal(t, tmux.PaneTarget{Session: "$1", Window: "@1", Pane: "%2"}, selected.Target)
		default:
			t.Fatal("wrong selection")
		}
	default:
		t.Fatal("wrong model")
	}
}

func TestTreeIgnoresStaleSourceLoads(t *testing.T) {
	m := NewTreeModel(nil, "")
	m.generation = 2
	result, _ := m.Update(treeLoaded{generation: 1, data: treeFixture()})
	switch next := result.(type) {
	case TreeModel:
		require.Empty(t, next.rows)
	default:
		t.Fatal("wrong model")
	}
}

func TestTreeRenderSanitizesLabels(t *testing.T) {
	data := treeFixture()
	data.Statuses["%2"] = agentstatus.Snapshot{Title: "bad\x1b[2J\nlabel", Status: agentstatus.Idle{}}
	m := NewTreeModel(nil, "")
	m.accept(data)
	view := m.View().Content
	require.NotContains(t, view, "\x1b[2J")
	require.Contains(t, view, "bad label")
	require.NotContains(t, view, "preview")
}

func TestTreeFlatSourcesDoNotInventParentsFromPaths(t *testing.T) {
	data := TreeData{Entries: model.SeshSessions{Directory: model.SeshSessionMap{
		"/home":       {Src: "find", Name: "/home"},
		"/home/child": {Src: "find", Name: "/home/child"},
	}, OrderedIndex: []string{"/home", "/home/child"}}}
	filtered := filterTreeRows(buildTreeRows(data), "child", nil)
	require.Len(t, filtered, 1)
	require.Equal(t, "entry:find:/home/child", filtered[0].key)
}

func TestTreeLongTitlesDoNotHideAgentStatus(t *testing.T) {
	data := treeFixture()
	data.Statuses["%2"] = agentstatus.Snapshot{Title: strings.Repeat("long title ", 30), Agent: "build", Status: agentstatus.Working{Execution: "1"}}
	m := NewTreeModel(nil, "")
	m.width = 60
	m.accept(data)
	require.Contains(t, m.View().Content, "[working]")
}

func TestTreeSessionsWithoutOpenCodeStaySessionOnly(t *testing.T) {
	data := TreeData{Topology: tmux.TreeSnapshot{Sessions: []tmux.TreeSession{
		{ID: "$2", Name: "plain", Windows: []tmux.TreeWindow{
			{ID: "@2", Name: "hidden-editor", Panes: []tmux.TreePane{{ID: "%3", Command: "vim", Path: "/opencode-notes"}}},
			{ID: "@3", Name: "hidden-shell", Panes: []tmux.TreePane{{ID: "%4", Command: "zsh", Title: "opencode"}}},
		}},
	}}}
	m := NewTreeModel(nil, "")
	m.accept(data)
	require.Len(t, m.rows, 1)
	require.Equal(t, "plain", m.rows[0].label)
	require.False(t, m.rows[0].branch)
	require.NotContains(t, m.View().Content, "▾")
	require.NotContains(t, m.View().Content, "▸")
	require.NotContains(t, m.View().Content, "hidden-editor")
	require.Empty(t, filterTreeRows(m.all, "hidden-editor", nil))
	require.Empty(t, filterTreeRows(m.all, "vim", nil))
	require.Len(t, filterTreeRows(m.all, "plain", nil), 1)

	result, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	switch next := result.(type) {
	case TreeModel:
		selection, picked := next.Chosen()
		require.True(t, picked)
		require.Equal(t, TreeTmuxSelection{Target: tmux.SessionTarget{Session: "$2"}}, selection)
	default:
		t.Fatal("wrong model")
	}
}

func TestTreeKeepsHierarchyForOpenCodeDetection(t *testing.T) {
	for _, tc := range []struct {
		name, command string
		statuses      map[string]agentstatus.Snapshot
	}{
		{name: "command without plugin", command: "opencode"},
		{name: "command case", command: "OpenCode"},
		{name: "plugin with generic command", command: "bun", statuses: map[string]agentstatus.Snapshot{"%2": {Title: "Conversation", Status: agentstatus.Idle{}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := treeFixture()
			data.Topology.Sessions[0].Windows[0].Panes[1].Command = tc.command
			data.Statuses = tc.statuses
			data.Topology.Sessions = append(data.Topology.Sessions, tmux.TreeSession{ID: "$2", Name: "plain", Windows: []tmux.TreeWindow{{ID: "@2", Panes: []tmux.TreePane{{ID: "%3", Command: "zsh"}}}}})
			rows := buildTreeRows(data)
			require.Len(t, rows, 5)
			require.True(t, rows[0].branch)
			require.Equal(t, "$1/@1/%1", rows[2].key)
			require.Equal(t, "$1/@1/%2", rows[3].key)
			require.Equal(t, "$2", rows[4].key)
			require.False(t, rows[4].branch)
		})
	}
}

func TestTreeIgnoresUnusableOrUnrelatedOpenCodeSnapshots(t *testing.T) {
	data := treeFixture()
	data.Topology.Sessions[0].Windows[0].Panes[1].Command = "bun"
	data.Statuses = map[string]agentstatus.Snapshot{"%2": {Title: "Unusable"}, "%99": {Title: "Other pane", Status: agentstatus.Idle{}}}
	require.Len(t, buildTreeRows(data), 1)
}

func TestTreeRefreshAddsAndRemovesOpenCodeHierarchy(t *testing.T) {
	data := treeFixture()
	data.Topology.Sessions[0].Windows[0].Panes[1].Command = "zsh"
	data.Statuses = nil
	m := NewTreeModel(nil, "work")
	m.accept(data)
	require.Len(t, m.rows, 1)

	m.accept(treeFixture())
	require.Len(t, m.rows, 4)
	require.Equal(t, "work", m.input.Value())
	require.Equal(t, "$1", m.rows[m.cursor].key)
	m.cursor = 3
	m.accept(data)
	require.Len(t, m.rows, 1)
	require.Equal(t, "$1", m.rows[m.cursor].key)
	require.Equal(t, "work", m.input.Value())
	require.False(t, m.rows[0].branch)
}
