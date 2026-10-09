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

func TestTreeDefaultShowsAgentsBeforeAgentFreeSessions(t *testing.T) {
	m := NewTreeModel(nil, "")
	data := treeFixture()
	// Ordinary sessions preceding agents in tmux order must still rank after them.
	data.Topology.Sessions[0], data.Topology.Sessions[1] = data.Topology.Sessions[1], data.Topology.Sessions[0]
	m.accept(data)
	require.Len(t, m.rows, 4)
	require.Equal(t, "$1/@1/%2", m.rows[0].key)
	require.Equal(t, "$1/@1/%3", m.rows[1].key)
	require.Equal(t, "$2", m.rows[2].key)
	require.Equal(t, "$3", m.rows[3].key)
	view := ansi.Strip(m.View().Content)
	require.Contains(t, view, "Tmux")
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

func TestTreeUnifiedSearchFindsSessionWithoutHiddenPaneDetails(t *testing.T) {
	m := NewTreeModel(nil, "scratch")
	data := treeFixture()
	data.Topology.Sessions[1].Name = "scratch"
	m.accept(data)
	require.Len(t, m.rows, 1)
	require.Equal(t, "scratch", m.rows[0].label)
	view := ansi.Strip(m.View().Content)
	require.NotContains(t, view, "hidden-editor")

	m = updateTree(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	selection, picked := m.Chosen()
	require.True(t, picked)
	require.Equal(t, TreeTmuxSelection{Target: tmux.SessionTarget{Session: "$2"}}, selection)
	m.input.SetValue("hidden-editor")
	m.filter("")
	require.Empty(t, m.rows)
}

func TestTreeSearchRanksContiguousMatchBeforeScatteredAgentMatch(t *testing.T) {
	m := NewTreeModel(nil, "2018")
	data := treeFixture()
	data.Topology.Sessions[0].Name = "zp4 can-2015"
	data.Topology.Sessions[1].Name = "zp6 can-2018"
	snapshot := data.Statuses["%2"]
	snapshot.Title = "CAN-2015 depends on PR #382967"
	// Even the highest-priority agent must trail a better search match.
	snapshot.Status = agentstatus.Finished{Execution: "1"}
	data.Statuses["%2"] = snapshot
	m.accept(data)
	require.Len(t, m.rows, 2)
	require.Equal(t, "zp6 can-2018", m.rows[0].label)
	require.Equal(t, "zp4 can-2015 > CAN-2015 depends on PR #382967", m.rows[1].label)
	m = updateTree(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	selection, picked := m.Chosen()
	require.True(t, picked)
	require.Equal(t, TreeTmuxSelection{Target: tmux.SessionTarget{Session: "$2"}}, selection)
}

func TestTreeSearchPreservesOriginalOrderForEqualScores(t *testing.T) {
	rows := []treeRow{
		{key: "first", label: "project"},
		{key: "second", label: "project"},
		{key: "third", label: "project"},
	}
	require.Equal(t, rows, filterTreeRows(rows, "proj"))
}

func TestTreeAgentStatusOrderPreservesOrderWithinGroups(t *testing.T) {
	for _, query := range []string{"", "work"} {
		t.Run("query="+query, func(t *testing.T) {
			data := treeFixture()
			data.Topology.Sessions[0].Windows[0].Panes = nil
			for _, tc := range []struct {
				pane   string
				status agentstatus.Status
			}{
				{"%10", agentstatus.Finished{Execution: "1", Read: true}},
				{"%11", nil},
				{"%12", agentstatus.Working{Execution: "1"}},
				{"%13", agentstatus.Attention{Reason: "permission"}},
				{"%14", agentstatus.Finished{Execution: "1"}},
				{"%15", agentstatus.Idle{}},
				{"%16", agentstatus.Finished{Execution: "2"}},
				{"%17", agentstatus.Attention{Reason: "failure"}},
				{"%18", agentstatus.Working{Execution: "2"}},
				{"%19", agentstatus.Finished{Execution: "2", Read: true}},
			} {
				data.Topology.Sessions[0].Windows[0].Panes = append(data.Topology.Sessions[0].Windows[0].Panes, tmux.TreePane{ID: tc.pane, Command: "opencode"})
				data.Statuses[tc.pane] = agentstatus.Snapshot{Title: "Conversation", Directory: "/project", Status: tc.status}
			}
			data.Topology.Sessions[1].Name = "work plain"
			data.Topology.Sessions[2].Name = "work other"
			// Keep ordinary sessions interleaved in topology, not already at the end.
			data.Topology.Sessions[0], data.Topology.Sessions[1] = data.Topology.Sessions[1], data.Topology.Sessions[0]
			data.Entries = model.SeshSessions{Directory: model.SeshSessionMap{"/work": {Src: "config", Name: "work config"}}, OrderedIndex: []string{"/work"}}
			m := NewTreeModel(nil, query)
			m.accept(data)
			keys := make([]string, len(m.rows))
			for i, row := range m.rows {
				keys[i] = row.key
			}
			require.Equal(t, []string{
				"$1/@1/%14", "$1/@1/%16", "$1/@1/%13", "$1/@1/%17",
				"$1/@1/%12", "$1/@1/%18", "$1/@1/%10", "$1/@1/%19",
				"$1/@1/%11", "$1/@1/%15", "$2", "$3", "entry:config:/work",
			}, keys)
		})
	}
}

func TestTreeStatusRefreshReordersWithoutChangingSelectedPane(t *testing.T) {
	for _, query := range []string{"", "work"} {
		t.Run("query="+query, func(t *testing.T) {
			data := treeFixture()
			m := NewTreeModel(nil, query)
			m.accept(data)
			require.Equal(t, "$1/@1/%2", m.selectedKey())
			data.Statuses["%3"] = agentstatus.Snapshot{Title: "Review payments", Directory: "/conversation/project", Status: agentstatus.Finished{Execution: "1"}}
			m = updateTree(t, m, treeLoaded{data: data})
			require.Equal(t, "$1/@1/%3", m.rows[0].key)
			require.Equal(t, "$1/@1/%2", m.selectedKey())
			require.Equal(t, query, m.input.Value())
			m = updateTree(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
			selection, picked := m.Chosen()
			require.True(t, picked)
			require.Equal(t, TreeTmuxSelection{Target: tmux.PaneTarget{Session: "$1", Window: "@1", Pane: "%2"}}, selection)
		})
	}
}

func TestTreeCtrlODoesNotSwitchUnifiedList(t *testing.T) {
	m := NewTreeModel(nil, "work")
	m.accept(treeFixture())
	m.cursor = 1
	m = updateTree(t, m, tea.KeyPressMsg{Code: 'o', Mod: tea.ModCtrl})
	require.Equal(t, "work", m.input.Value())
	require.Equal(t, "$1/@1/%3", m.rows[m.cursor].key)
	require.NotContains(t, m.View().Content, "Ctrl-O")
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

func TestTreeAgentRowsKeepPathInline(t *testing.T) {
	m := NewTreeModel(nil, "payments")
	m.accept(treeFixture())
	for _, width := range []int{80, 120, 200} {
		lines := renderTreeRow(m.rows[0], width, "", true)
		require.Len(t, lines, 1)
		first := ansi.Strip(lines[0])
		require.Contains(t, first, "›")
		require.Contains(t, first, "● working")
		require.Contains(t, first, "work > Review payments")
		require.True(t, strings.HasSuffix(first, "  /conversation/project "), first)
		require.Contains(t, lines[0], "38;2;165;173;203", "path retains muted foreground")
		require.Contains(t, lines[0], "48;2;54;58;79", "selection retains row background")
		for _, line := range lines {
			require.Equal(t, width, ansi.StringWidth(line))
		}
		unselected := renderTreeRow(m.rows[0], width, "", false)
		require.NotContains(t, ansi.Strip(unselected[0]), "›")
	}
}

func TestTreeAgentInlineRowTruncatesWithoutHidingStatus(t *testing.T) {
	row := treeRow{label: strings.Repeat("long title ", 30), agent: &agentRow{
		status: agentstatus.Attention{Reason: "permission"}, directory: "/a/very/long/conversation/path",
	}}
	for _, width := range []int{0, 1, 10, 20, 30, 40, 60, 120} {
		lines := renderTreeRow(row, width, "", true)
		require.Len(t, lines, 1)
		line := ansi.Strip(lines[0])
		require.Equal(t, width, ansi.StringWidth(line))
		require.NotContains(t, line, "\n")
		if width >= 20 {
			require.Contains(t, line, "! attention")
		}
		if width >= 30 {
			require.Contains(t, line, "lon")
			require.Contains(t, line, "/a/")
			require.Contains(t, line, "…")
		}
	}
}

func TestTreeHeaderCountsMatchesAndFooterStaysAtBottom(t *testing.T) {
	m := NewTreeModel(nil, "payments")
	m.width, m.height = 120, 15
	m = updateTree(t, m, tea.WindowSizeMsg{Width: 120, Height: 15})
	m.accept(treeFixture())
	lines := strings.Split(ansi.Strip(m.View().Content), "\n")
	require.Len(t, lines, 15)
	require.Contains(t, lines[0], "Tmux · 1")
	require.Contains(t, lines[13], "› payments")
	require.Contains(t, lines[14], "Enter open")
	require.NotContains(t, strings.Join(lines[:14], "\n"), "Ctrl-O")
	m.input.SetValue("no match")
	m.filter("")
	lines = strings.Split(ansi.Strip(m.View().Content), "\n")
	require.Contains(t, lines[0], "Tmux · 0")
	require.Contains(t, strings.Join(lines, "\n"), "No matches")
	require.Contains(t, lines[14], "Enter open")
}

func TestTreeBottomSearchAnchorsBestMatch(t *testing.T) {
	m := NewTreeModel(nil, "")
	m = updateTree(t, m, tea.WindowSizeMsg{Width: 120, Height: 15})
	m.accept(treeFixture())
	lines := strings.Split(ansi.Strip(m.View().Content), "\n")
	require.Len(t, lines, 15)
	require.Contains(t, lines[0], "Tmux · 4")
	require.Empty(t, strings.TrimSpace(lines[1]))
	require.Empty(t, strings.TrimSpace(lines[7]))
	require.Contains(t, lines[8], "other")
	require.Contains(t, lines[9], "plain")
	require.Contains(t, lines[10], "? unavailable")
	require.Contains(t, lines[10], "path unavailable")
	require.Contains(t, lines[11], "› ● working")
	require.Contains(t, lines[11], "work > Review payments")
	require.Contains(t, lines[11], "/conversation/project")
	require.Empty(t, strings.TrimSpace(lines[12]))
	require.Contains(t, lines[13], "Search conversations")
	require.Contains(t, lines[14], "Enter open")
	require.Equal(t, "$1/@1/%2", m.rows[0].key)
	m = updateTree(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	selection, ok := m.Chosen()
	require.True(t, ok)
	require.Equal(t, TreeTmuxSelection{Target: tmux.PaneTarget{Session: "$1", Window: "@1", Pane: "%2"}}, selection)
}

func TestTreeBottomSearchKeepsInlineRowsInRankOrder(t *testing.T) {
	m := NewTreeModel(nil, "")
	m = updateTree(t, m, tea.WindowSizeMsg{Width: 120, Height: 10})
	m.accept(treeFixture())
	raw := strings.Split(m.View().Content, "\n")
	lines := strings.Split(ansi.Strip(m.View().Content), "\n")
	require.Contains(t, lines[3], "other")
	require.Contains(t, lines[4], "plain")
	require.Contains(t, lines[5], "work > OpenCode (title unavailable)")
	require.Contains(t, lines[5], "path unavailable")
	require.Contains(t, lines[6], "work > Review payments")
	require.Contains(t, lines[6], "/conversation/project")
	for _, index := range []int{6} {
		require.Contains(t, raw[index], "\x1b[")
		require.Equal(t, 120, ansi.StringWidth(raw[index]))
	}
	m.cursor = 1
	lines = strings.Split(ansi.Strip(m.View().Content), "\n")
	require.Contains(t, lines[5], "work > OpenCode (title unavailable)")
	require.Contains(t, lines[5], "path unavailable")
	require.Contains(t, lines[6], "work > Review payments")
	require.Contains(t, lines[6], "/conversation/project")
}

func TestTreeBottomSearchNavigation(t *testing.T) {
	for _, tc := range []struct {
		name       string
		key        tea.KeyPressMsg
		from, want int
	}{
		{"up", tea.KeyPressMsg{Code: tea.KeyUp}, 0, 1},
		{"ctrl-p", tea.KeyPressMsg{Code: 'p', Mod: tea.ModCtrl}, 0, 1},
		{"tab", tea.KeyPressMsg{Code: tea.KeyTab}, 0, 1},
		{"down", tea.KeyPressMsg{Code: tea.KeyDown}, 1, 0},
		{"ctrl-n", tea.KeyPressMsg{Code: 'n', Mod: tea.ModCtrl}, 1, 0},
		{"shift-tab", tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}, 1, 0},
		{"up clamps", tea.KeyPressMsg{Code: tea.KeyUp}, 3, 3},
		{"down clamps", tea.KeyPressMsg{Code: tea.KeyDown}, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := NewTreeModel(nil, "")
			m.accept(treeFixture())
			m.cursor = tc.from
			m = updateTree(t, m, tc.key)
			require.Equal(t, tc.want, m.cursor)
			m = updateTree(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
			selection, ok := m.Chosen()
			require.True(t, ok)
			require.Equal(t, m.rows[tc.want].selection, selection)
		})
	}
}

func TestTreeBottomSearchScrollAndResize(t *testing.T) {
	m := NewTreeModel(nil, "")
	m.accept(treeFixture())
	for _, height := range []int{6, 5, 10, 7} {
		m = updateTree(t, m, tea.WindowSizeMsg{Width: 60, Height: height})
		for _, index := range []int{0, 1, 2, 3, 2, 1, 0} {
			m.cursor = index
			lines := strings.Split(ansi.Strip(m.View().Content), "\n")
			require.Len(t, lines, height)
			require.Contains(t, strings.Join(lines[1:height-3], "\n"), "›")
			if index < 2 {
				path := "/conversation/"
				if index == 1 {
					path = "path unavailable"
				}
				require.Contains(t, strings.Join(lines[1:height-3], "\n"), path)
			}
			require.Contains(t, lines[height-2], "Search conversations")
			for _, line := range lines {
				require.LessOrEqual(t, ansi.StringWidth(line), 60)
			}
		}
	}
}

func TestTreeBottomSearchStateLayouts(t *testing.T) {
	for _, tc := range []struct {
		name, query, message string
		loading              bool
		err                  error
	}{
		{"empty", "missing", "No matches", false, nil},
		{"loading", "", "Loading...", true, nil},
		{"error", "", "Refresh failed: offline", false, errors.New("offline")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := NewTreeModel(nil, tc.query)
			m = updateTree(t, m, tea.WindowSizeMsg{Width: 80, Height: 10})
			m.loading, m.err = tc.loading, tc.err
			lines := strings.Split(ansi.Strip(m.View().Content), "\n")
			require.Len(t, lines, 10)
			require.Contains(t, lines[8], "›")
			require.Contains(t, strings.Join(lines, "\n"), tc.message)
			m = updateTree(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
			_, picked := m.Chosen()
			require.False(t, picked)
		})
	}
	for _, source := range []TreeSource{TreeTmux, TreeAll, TreeConfig, TreeZoxide, TreeFind} {
		m := NewTreeModel(nil, "work")
		m.source = source
		m.accept(treeFixture())
		m = updateTree(t, m, tea.KeyPressMsg{Code: tea.KeyTab})
		m = updateTree(t, m, treeLoaded{data: treeFixture()})
		lines := strings.Split(ansi.Strip(m.View().Content), "\n")
		require.Contains(t, lines[len(lines)-2], "› work")
		require.Equal(t, "$1/@1/%3", m.selectedKey())
	}
}

func TestTreeTinyLayoutPrioritizesSearch(t *testing.T) {
	for height := 1; height <= 5; height++ {
		m := NewTreeModel(nil, "payments")
		m = updateTree(t, m, tea.WindowSizeMsg{Width: 80, Height: height})
		m.accept(treeFixture())
		lines := strings.Split(ansi.Strip(m.View().Content), "\n")
		require.Len(t, lines, height)
		inputRow := max(0, height-2)
		require.Contains(t, lines[inputRow], "› payments")
		if height >= 2 {
			require.Contains(t, lines[height-1], "Enter open")
		}
		if height >= 3 {
			require.Contains(t, lines[0], "Tmux · 1")
		}
		if height == 5 {
			require.Contains(t, lines[1], "Review payments")
		}
	}
}

func TestTreeNarrowLoadingMessageFitsWidth(t *testing.T) {
	m := NewTreeModel(nil, "")
	m = updateTree(t, m, tea.WindowSizeMsg{Width: 3, Height: 8})
	m.loading = true
	for _, line := range strings.Split(m.View().Content, "\n") {
		require.LessOrEqual(t, ansi.StringWidth(line), 3)
	}
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
		{"truncated", "/a/very/long/directory", "", "/a/very/l…", 40},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, selected := range []bool{false, true} {
				row := treeRow{label: "session > title", agent: &agentRow{status: agentstatus.Idle{}, directory: tc.directory}}
				lines := renderTreeRow(row, tc.width, tc.home, selected)
				require.Len(t, lines, 1)
				path := ansi.Strip(lines[0])
				require.Equal(t, tc.width, ansi.StringWidth(path))
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

func TestTreeNarrowViewKeepsPathInlineAndScrollsSelection(t *testing.T) {
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

func TestTreeRefreshReplacesAgentsWithSessionAndPreservesQuery(t *testing.T) {
	m := NewTreeModel(nil, "")
	m.accept(treeFixture())
	data := treeFixture()
	data.Statuses = nil
	data.Topology.Sessions[0].Windows[0].Panes[2].Command = "zsh"
	m.accept(data)
	require.Len(t, m.rows, 3)
	require.Equal(t, "$1", m.rows[0].key)
	m.accept(treeFixture())
	require.Len(t, m.rows, 4)
	require.Equal(t, "$1/@1/%2", m.rows[0].key)
	m.input.SetValue("work")
	m.filter("")
	m.accept(data)
	require.Equal(t, "work", m.input.Value())
	require.Len(t, m.rows, 1)
	require.Equal(t, "$1", m.selectedKey())
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

func TestTreeSourceReturnDuringLoadRetainsPendingSelection(t *testing.T) {
	m := NewTreeModel(func(TreeSource) (TreeData, error) { return treeFixture(), nil }, "work")
	m = updateTree(t, m, treeLoaded{data: treeFixture()})
	m.cursor = 1
	m = updateTree(t, m, tea.KeyPressMsg{Code: 'g', Mod: tea.ModCtrl})
	m = updateTree(t, m, tea.KeyPressMsg{Code: 't', Mod: tea.ModCtrl})
	m = updateTree(t, m, tea.KeyPressMsg{Code: 'g', Mod: tea.ModCtrl})
	m = updateTree(t, m, tea.KeyPressMsg{Code: 't', Mod: tea.ModCtrl})
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
