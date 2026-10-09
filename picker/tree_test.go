package picker

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/joshmedeski/sesh/v2/agentstatus"
	"github.com/joshmedeski/sesh/v2/model"
	"github.com/joshmedeski/sesh/v2/tmux"
	"github.com/stretchr/testify/require"
)

func treeFixture() TreeData {
	return TreeData{Topology: tmux.TreeSnapshot{Server: "s", Sessions: []tmux.TreeSession{
		{ID: "$1", Name: "work", Windows: []tmux.TreeWindow{{ID: "@1", Name: "main", Panes: []tmux.TreePane{
			{ID: "%1", Command: "zsh", Path: "/pane/shell"},
			{ID: "%2", Command: "bun", Path: "/pane/not-conversation"},
			{ID: "%3", Command: "opencode", Path: "/pane/unknown"},
		}}}},
		{ID: "$2", Name: "plain", Windows: []tmux.TreeWindow{{ID: "@2", Name: "hidden-editor", Panes: []tmux.TreePane{{ID: "%4", Command: "vim", Path: "/opencode-notes", Title: "opencode"}}}}},
		{ID: "$3", Name: "other"},
	}}, Statuses: map[string]agentstatus.Snapshot{"%2": {Conversation: "ses_1", Title: "Review payments", Directory: "/conversation/project", Status: agentstatus.Working{Execution: "1"}}}}
}

func updateTree(t *testing.T, m TreeModel, msg tea.Msg) TreeModel {
	t.Helper()
	result, _ := m.Update(msg)
	switch next := result.(type) {
	case TreeModel:
		return next
	default:
		t.Fatal("wrong model")
		return m
	}
}

func TestTreeDefaultShowsOnlyFlatAgents(t *testing.T) {
	m := NewTreeModel(nil, "")
	m.accept(treeFixture())
	require.Len(t, m.rows, 2)
	require.Equal(t, "$1/@1/%2", m.rows[0].key)
	require.Equal(t, "$1/@1/%3", m.rows[1].key)
	view := ansi.Strip(m.View().Content)
	require.Contains(t, view, "Agents")
	require.Contains(t, view, "work > Review payments")
	require.Contains(t, view, "● working")
	require.Contains(t, view, "/conversation/project")
	require.Contains(t, view, "? unavailable")
	require.NotContains(t, view, "/pane/not-conversation")
	require.NotContains(t, view, "/pane/unknown")
	require.NotContains(t, view, "hidden-editor")
	require.NotContains(t, view, "▾")
	require.NotContains(t, view, "preview")
}

func TestTreeToggleShowsOnlySessionsWithoutAgents(t *testing.T) {
	m := NewTreeModel(nil, "")
	m.accept(treeFixture())
	m = updateTree(t, m, tea.KeyPressMsg{Code: 'o', Mod: tea.ModCtrl})
	require.Len(t, m.rows, 2)
	require.Equal(t, "plain", m.rows[0].label)
	require.Equal(t, "other", m.rows[1].label)
	view := ansi.Strip(m.View().Content)
	require.Contains(t, view, "Sessions")
	require.NotContains(t, view, "work")
	require.NotContains(t, view, "hidden-editor")
	require.NotContains(t, view, "working")

	m = updateTree(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	selection, picked := m.Chosen()
	require.True(t, picked)
	require.Equal(t, TreeTmuxSelection{Target: tmux.SessionTarget{Session: "$2"}}, selection)
}

func TestTreeToggleRemembersEachViewsSearchAndSelection(t *testing.T) {
	m := NewTreeModel(nil, "work")
	m.accept(treeFixture())
	m.cursor = 1
	m = updateTree(t, m, tea.KeyPressMsg{Code: 'o', Mod: tea.ModCtrl})
	require.Equal(t, "", m.input.Value())
	require.Len(t, m.rows, 2)
	m.input.SetValue("plain")
	m.filter("")
	m = updateTree(t, m, tea.KeyPressMsg{Code: 'o', Mod: tea.ModCtrl})
	require.Equal(t, "work", m.input.Value())
	require.Equal(t, "$1/@1/%3", m.rows[m.cursor].key)
	m = updateTree(t, m, tea.KeyPressMsg{Code: 'o', Mod: tea.ModCtrl})
	require.Equal(t, "plain", m.input.Value())
	require.Equal(t, "$2", m.rows[m.cursor].key)
}

func TestTreeAgentSearchMatchesSessionConversationAndDirectory(t *testing.T) {
	for _, query := range []string{"payments", "conversation/project"} {
		m := NewTreeModel(nil, query)
		m.accept(treeFixture())
		require.Len(t, m.rows, 1)
		require.Equal(t, "$1/@1/%2", m.rows[0].key)
	}
	m := NewTreeModel(nil, "work")
	m.accept(treeFixture())
	require.Len(t, m.rows, 2)
	m = updateTree(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	selection, picked := m.Chosen()
	require.True(t, picked)
	require.Equal(t, TreeTmuxSelection{Target: tmux.PaneTarget{Session: "$1", Window: "@1", Pane: "%2"}}, selection)
}

func TestTreeAgentStatusIndicatorsAreDistinctWithoutColor(t *testing.T) {
	for _, tc := range []struct {
		status agentstatus.Status
		want   string
	}{
		{agentstatus.Idle{}, "○ idle"},
		{agentstatus.Working{Execution: "1"}, "● working"},
		{agentstatus.Attention{Reason: "permission"}, "! attention"},
		{agentstatus.Finished{Execution: "1"}, "◆ unread"},
		{agentstatus.Finished{Execution: "1", Read: true}, "✓ read"},
	} {
		data := treeFixture()
		data.Statuses["%2"] = agentstatus.Snapshot{Title: "Review", Directory: "/conversation", Status: tc.status}
		m := NewTreeModel(nil, "Review")
		m.accept(data)
		require.Contains(t, ansi.Strip(m.View().Content), tc.want)
	}
}

func TestTreeLegacySnapshotDoesNotMislabelPanePathAsConversationPath(t *testing.T) {
	data := treeFixture()
	data.Statuses["%2"] = agentstatus.Snapshot{Title: "Review", Status: agentstatus.Idle{}}
	m := NewTreeModel(nil, "Review")
	m.accept(data)
	view := ansi.Strip(m.View().Content)
	require.Contains(t, view, "path unavailable")
	require.NotContains(t, view, "/pane/not-conversation")
}

func TestTreeWideViewKeepsPathWhenTitleIsLong(t *testing.T) {
	data := treeFixture()
	data.Statuses["%2"] = agentstatus.Snapshot{Title: strings.Repeat("long title ", 30), Directory: "/conversation/project", Status: agentstatus.Working{Execution: "1"}}
	m := NewTreeModel(nil, "long")
	m.width = 120
	m.accept(data)
	view := ansi.Strip(m.View().Content)
	require.Contains(t, view, "● working")
	require.Contains(t, view, "/conversation/project")
	for _, line := range strings.Split(view, "\n") {
		require.LessOrEqual(t, ansi.StringWidth(line), 120)
	}
}

func TestTreeAgentRowsKeepPathBelowTitleAtEveryWidth(t *testing.T) {
	m := NewTreeModel(nil, "payments")
	m.accept(treeFixture())
	for _, width := range []int{60, 120, 200} {
		lines := renderTreeRow(m.rows[0], width, "", true)
		require.Len(t, lines, 2)
		first, second := ansi.Strip(lines[0]), ansi.Strip(lines[1])
		require.Contains(t, first, "›")
		require.Contains(t, first, "● working")
		require.Contains(t, first, "work > Review payments")
		require.NotContains(t, first, "/conversation/project")
		require.Contains(t, second, "/conversation/project")
		for _, line := range lines {
			require.Equal(t, width, ansi.StringWidth(line))
		}
		unselected := renderTreeRow(m.rows[0], width, "", false)
		require.NotContains(t, ansi.Strip(unselected[0]), "›")
	}
}

func TestTreeHeaderCountsMatchesAndFooterStaysAtBottom(t *testing.T) {
	m := NewTreeModel(nil, "payments")
	m.width, m.height = 120, 15
	m.accept(treeFixture())
	lines := strings.Split(ansi.Strip(m.View().Content), "\n")
	require.Len(t, lines, 15)
	require.Contains(t, lines[0], "Agents · 1")
	require.Contains(t, lines[14], "Ctrl-O sessions")
	require.Contains(t, lines[14], "Enter open")
	require.NotContains(t, strings.Join(lines[:14], "\n"), "Ctrl-O")
	m.input.SetValue("no match")
	m.filter("")
	lines = strings.Split(ansi.Strip(m.View().Content), "\n")
	require.Contains(t, lines[0], "Agents · 0")
	require.Contains(t, strings.Join(lines, "\n"), "No matches")
	require.Contains(t, lines[14], "Ctrl-O sessions")
}

func TestTreeAgentPathRightAlignedWithInset(t *testing.T) {
	for _, tc := range []struct {
		name, directory, home, want string
		width                       int
	}{
		{"short path", "/project", "", "/project", 60},
		{"home abbreviation", "/home/amy/project", "/home/amy", "~/project", 80},
		{"unicode", "/项目", "", "/项目", 40},
		{"unavailable", "", "", "path unavailable", 60},
		{"truncated", "/a/very/long/directory", "", "/a/very…", 10},
		{"one column", "/project", "", "", 1},
		{"zero columns", "/project", "", "", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, selected := range []bool{false, true} {
				row := treeRow{label: "session > title", agent: &agentRow{status: agentstatus.Idle{}, directory: tc.directory}}
				lines := renderTreeRow(row, tc.width, tc.home, selected)
				require.Len(t, lines, 2)
				path := ansi.Strip(lines[1])
				require.Equal(t, tc.width, ansi.StringWidth(path))
				require.Equal(t, tc.want, strings.TrimSpace(path))
				if tc.width > 0 {
					require.True(t, strings.HasSuffix(path, tc.want+" "), path)
				}
			}
		})
	}
}

func TestTreeLayoutFitsTinyTerminals(t *testing.T) {
	for _, size := range []struct{ width, height int }{{0, 0}, {1, 1}, {20, 3}, {30, 5}} {
		m := NewTreeModel(nil, "")
		m.width, m.height = size.width, size.height
		m.accept(treeFixture())
		view := ansi.Strip(m.View().Content)
		if size.height == 0 {
			require.Empty(t, view)
			continue
		}
		lines := strings.Split(view, "\n")
		require.LessOrEqual(t, len(lines), size.height)
		for _, line := range lines {
			require.LessOrEqual(t, ansi.StringWidth(line), size.width)
		}
	}
}

func TestTreeNarrowViewMovesPathBelowTitleAndScrollsSelection(t *testing.T) {
	m := NewTreeModel(nil, "")
	m.width, m.height = 60, 6
	m.accept(treeFixture())
	m = updateTree(t, m, tea.KeyPressMsg{Code: tea.KeyTab})
	view := ansi.Strip(m.View().Content)
	require.Contains(t, view, "? unavailable")
	require.Contains(t, view, "path unavailable")
	require.Equal(t, "$1/@1/%3", m.rows[m.cursor].key)
	lines := strings.Split(view, "\n")
	require.LessOrEqual(t, len(lines), 6)
	for _, line := range lines {
		require.LessOrEqual(t, ansi.StringWidth(line), 60)
	}
}

func TestTreeRenderSanitizesLabels(t *testing.T) {
	data := treeFixture()
	data.Statuses["%2"] = agentstatus.Snapshot{Title: "bad\x1b[2J\nlabel", Directory: "/path\x1b[2J", Status: agentstatus.Idle{}}
	m := NewTreeModel(nil, "bad")
	m.accept(data)
	view := m.View().Content
	require.NotContains(t, view, "\x1b[2J")
	require.Contains(t, view, "bad label")
}

func TestTreeRefreshPreservesSelectionAndQuery(t *testing.T) {
	m := NewTreeModel(nil, "work")
	m.accept(treeFixture())
	m.cursor = 1
	m.accept(treeFixture())
	require.Equal(t, "$1/@1/%3", m.rows[m.cursor].key)
	data := treeFixture()
	data.Topology.Sessions[0].Windows[0].Panes = data.Topology.Sessions[0].Windows[0].Panes[:2]
	m.accept(data)
	require.Len(t, m.rows, 1)
	require.Equal(t, "$1/@1/%2", m.rows[m.cursor].key)
	require.Equal(t, "work", m.input.Value())
}

func TestTreeRefreshMovesSessionsBetweenViews(t *testing.T) {
	m := NewTreeModel(nil, "")
	m.accept(treeFixture())
	data := treeFixture()
	data.Statuses = nil
	data.Topology.Sessions[0].Windows[0].Panes[2].Command = "zsh"
	m.accept(data)
	require.Empty(t, m.rows)
	m = updateTree(t, m, tea.KeyPressMsg{Code: 'o', Mod: tea.ModCtrl})
	require.Len(t, m.rows, 3)
	m.accept(treeFixture())
	require.Len(t, m.rows, 2)
}

func TestTreeSourceShortcutsAndStaleLoads(t *testing.T) {
	for _, tc := range []struct {
		key    rune
		source TreeSource
	}{{'a', TreeAll}, {'t', TreeTmux}, {'g', TreeConfig}, {'x', TreeZoxide}, {'f', TreeFind}} {
		m := NewTreeModel(nil, "")
		m.accept(treeFixture())
		m = updateTree(t, m, tea.KeyPressMsg{Code: tc.key, Mod: tea.ModCtrl})
		require.Equal(t, tc.source, m.source)
		m = updateTree(t, m, treeLoaded{generation: m.generation - 1, data: treeFixture()})
		require.Empty(t, m.rows)
	}
}

func TestTreeReturningFromSourceRestoresTmuxView(t *testing.T) {
	m := NewTreeModel(nil, "work")
	m.accept(treeFixture())
	m.cursor = 1
	m = updateTree(t, m, tea.KeyPressMsg{Code: 'g', Mod: tea.ModCtrl})
	m.input.SetValue("config query")
	m = updateTree(t, m, tea.KeyPressMsg{Code: 't', Mod: tea.ModCtrl})
	m = updateTree(t, m, treeLoaded{generation: m.generation, data: treeFixture()})
	require.Equal(t, "work", m.input.Value())
	require.Equal(t, "$1/@1/%3", m.selectedKey())
}

func TestTreeToggleDuringLoadRetainsPendingSelection(t *testing.T) {
	m := NewTreeModel(func(TreeSource) (TreeData, error) { return treeFixture(), nil }, "work")
	m = updateTree(t, m, treeLoaded{data: treeFixture()})
	m.cursor = 1
	m = updateTree(t, m, tea.KeyPressMsg{Code: 'g', Mod: tea.ModCtrl})
	m = updateTree(t, m, tea.KeyPressMsg{Code: 'o', Mod: tea.ModCtrl})
	m = updateTree(t, m, tea.KeyPressMsg{Code: 'o', Mod: tea.ModCtrl})
	m = updateTree(t, m, tea.KeyPressMsg{Code: 'o', Mod: tea.ModCtrl})
	m = updateTree(t, m, tea.KeyPressMsg{Code: 'o', Mod: tea.ModCtrl})
	m = updateTree(t, m, treeLoaded{generation: m.generation, data: treeFixture()})
	require.Equal(t, "work", m.input.Value())
	require.Equal(t, "$1/@1/%3", m.selectedKey())
}

func TestTreeRefreshFailureRetainsRowsAndRecovers(t *testing.T) {
	m := NewTreeModel(nil, "payments")
	m.accept(treeFixture())
	m = updateTree(t, m, treeLoaded{data: TreeData{}, err: errors.New("disconnected")})
	require.Len(t, m.rows, 1)
	require.Equal(t, "payments", m.input.Value())
	require.Contains(t, m.View().Content, "disconnected")
	m = updateTree(t, m, treeLoaded{data: treeFixture()})
	require.Nil(t, m.err)
	require.Len(t, m.rows, 1)
}

func TestTreeAllSourceIncludesNormalEntriesWithoutInventingParents(t *testing.T) {
	data := treeFixture()
	data.Entries = model.SeshSessions{Directory: model.SeshSessionMap{"/home": {Src: "find", Name: "/home"}, "/home/child": {Src: "find", Name: "/home/child"}}, OrderedIndex: []string{"/home", "/home/child"}}
	m := NewTreeModel(nil, "child")
	m.accept(data)
	require.Len(t, m.rows, 1)
	require.Equal(t, "entry:find:/home/child", m.rows[0].key)
}
