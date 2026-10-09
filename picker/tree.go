package picker

import (
	"fmt"
	"os"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/joshmedeski/sesh/v2/agentstatus"
	"github.com/joshmedeski/sesh/v2/model"
	"github.com/joshmedeski/sesh/v2/tmux"
)

// TreeSource names the existing source shortcuts without overloading delete keys.
type TreeSource uint8

const (
	TreeTmux TreeSource = iota
	TreeAll
	TreeConfig
	TreeZoxide
	TreeFind
)

func (s TreeSource) String() string {
	switch s {
	case TreeAll:
		return "all"
	case TreeConfig:
		return "configs"
	case TreeZoxide:
		return "zoxide"
	case TreeFind:
		return "find"
	default:
		return "tmux"
	}
}

// TreeData combines independent live topology and optional agent metadata.
type TreeData struct {
	Topology tmux.TreeSnapshot
	Statuses map[string]agentstatus.Snapshot
	Entries  model.SeshSessions
}
type TreeFetchFunc func(TreeSource) (TreeData, error)

// TreeSelection distinguishes a live tmux target from a sesh connection name.
type TreeSelection interface{ treeSelection() }
type TreeTmuxSelection struct{ Target tmux.TreeTarget }
type TreeNameSelection struct{ Name string }

func (TreeTmuxSelection) treeSelection() {}
func (TreeNameSelection) treeSelection() {}

type treeLoaded struct {
	generation uint64
	data       TreeData
	err        error
}
type treeTick struct{}

// tmuxView partitions live tmux entries into agent panes and agent-free sessions.
type tmuxView uint8

const (
	agentsView tmuxView = iota
	sessionsView
)

func (v tmuxView) String() string {
	if v == sessionsView {
		return "Sessions"
	}
	return "Agents"
}

type treeViewState struct {
	query, key string
	cursor     int
}

// TreeModel is opt-in; it deliberately does not inherit preview/deletion actions.
type TreeModel struct {
	fetch                 TreeFetchFunc
	source                TreeSource
	generation            uint64
	input                 textinput.Model
	all, rows             []treeRow
	view                  tmuxView
	views                 [2]treeViewState
	data                  TreeData
	pendingKey            string
	home                  string
	cursor, width, height int
	loading               bool
	err                   error
	chosen                TreeSelection
	quit                  bool
}

func NewTreeModel(fetch TreeFetchFunc, query string) TreeModel {
	input := textinput.New()
	input.Prompt = "  › "
	input.Placeholder = "Search conversations, sessions, paths…"
	input.SetValue(query)
	input.Focus()
	home, _ := os.UserHomeDir()
	return TreeModel{fetch: fetch, source: TreeTmux, input: input, home: home, width: 80, height: 24, loading: fetch != nil}
}

func (m TreeModel) load() tea.Cmd {
	if m.fetch == nil {
		return nil
	}
	fetch, source, generation := m.fetch, m.source, m.generation
	return func() tea.Msg {
		data, err := fetch(source)
		return treeLoaded{generation: generation, data: data, err: err}
	}
}
func treeRefresh() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg { return treeTick{} })
}
func (m TreeModel) Init() tea.Cmd { return tea.Batch(m.load(), treeRefresh(), textinput.Blink) }

func (m *TreeModel) selectedKey() string {
	if m.cursor >= 0 && m.cursor < len(m.rows) {
		return m.rows[m.cursor].key
	}
	return ""
}
func (m *TreeModel) filter(key string) {
	m.rows = filterTreeRows(m.all, m.input.Value())
	if key != "" {
		for i := range m.rows {
			if m.rows[i].key == key {
				m.cursor = i
				return
			}
		}
	}
	m.cursor = max(0, min(m.cursor, len(m.rows)-1))
}
func (m *TreeModel) accept(data TreeData) {
	key := m.selectedKey()
	if key == "" {
		key = m.pendingKey
	}
	m.pendingKey = ""
	m.data = data
	m.all = buildTreeRows(data, m.view)
	m.filter(key)
}

func (m *TreeModel) saveView() {
	if m.loading && len(m.rows) == 0 {
		m.views[m.view].query = m.input.Value()
		return
	}
	m.views[m.view] = treeViewState{query: m.input.Value(), key: m.selectedKey(), cursor: m.cursor}
}
func (m TreeModel) Chosen() (TreeSelection, bool) { return m.chosen, m.chosen != nil && !m.quit }

func (m TreeModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.input.SetWidth(max(1, m.width-4))
		return m, nil
	case treeLoaded:
		if msg.generation != m.generation {
			return m, nil
		}
		m.loading = false
		m.err = msg.err
		if msg.err == nil {
			m.accept(msg.data)
		}
		return m, nil
	case treeTick:
		if m.loading {
			return m, treeRefresh()
		}
		m.loading = m.fetch != nil
		return m, tea.Batch(m.load(), treeRefresh())
	case tea.KeyPressMsg:
		key := msg.String()
		switch key {
		case "esc", "ctrl+c":
			m.quit = true
			return m, tea.Quit
		case "enter":
			if len(m.rows) > 0 {
				m.chosen = m.rows[m.cursor].selection
				return m, tea.Quit
			}
			return m, nil
		case "tab", "down", "ctrl+n":
			m.cursor = min(m.cursor+1, max(0, len(m.rows)-1))
			return m, nil
		case "shift+tab", "up", "ctrl+p":
			m.cursor = max(0, m.cursor-1)
			return m, nil
		case "ctrl+o":
			if m.source == TreeTmux {
				m.saveView()
			}
			if m.view == agentsView {
				m.view = sessionsView
			} else {
				m.view = agentsView
			}
			state := m.views[m.view]
			m.pendingKey = state.key
			m.input.SetValue(state.query)
			m.cursor = state.cursor
			if m.source != TreeTmux {
				m.source = TreeTmux
				m.generation++
				m.loading = m.fetch != nil
				m.data = TreeData{}
				m.all = nil
				m.rows = nil
				m.err = nil
				return m, m.load()
			}
			m.all = buildTreeRows(m.data, m.view)
			m.filter(state.key)
			if !m.loading {
				m.pendingKey = ""
			}
			return m, nil
		case "ctrl+a", "ctrl+t", "ctrl+g", "ctrl+x", "ctrl+f":
			if m.source == TreeTmux {
				m.saveView()
			}
			sources := map[string]TreeSource{"ctrl+a": TreeAll, "ctrl+t": TreeTmux, "ctrl+g": TreeConfig, "ctrl+x": TreeZoxide, "ctrl+f": TreeFind}
			m.source = sources[key]
			m.generation++
			m.loading = m.fetch != nil
			m.all = nil
			m.rows = nil
			m.data = TreeData{}
			m.cursor = 0
			m.pendingKey = ""
			if m.source == TreeTmux {
				state := m.views[m.view]
				m.input.SetValue(state.query)
				m.cursor = state.cursor
				m.pendingKey = state.key
			}
			m.err = nil
			return m, m.load()
		case "ctrl+d":
			return m, nil
		}
	}
	value := m.input.Value()
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	if value != m.input.Value() {
		m.cursor = 0
		m.filter("")
	}
	return m, cmd
}

func (m TreeModel) View() tea.View {
	if m.height <= 0 || m.width <= 0 {
		view := tea.NewView("")
		view.AltScreen = true
		return view
	}
	header := m.view.String()
	if m.source != TreeTmux {
		header += " · " + m.source.String()
	}
	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#c6a0f6")).Render(header)
	count := lipgloss.NewStyle().Foreground(lipgloss.Color("#a5adcb")).Render(fmt.Sprintf(" · %d", len(m.rows)))
	lines := []string{ansi.Truncate("  "+title+count, m.width, "…"), ansi.Truncate(m.input.View(), m.width, "…"), ""}
	available := max(0, m.height-4)
	start, used := min(m.cursor, max(0, len(m.rows)-1)), 0
	if len(m.rows) > 0 {
		used = len(renderTreeRow(m.rows[start], m.width, m.home, true))
	}
	for start > 0 {
		height := len(renderTreeRow(m.rows[start-1], m.width, m.home, false))
		if used+height > available {
			break
		}
		start--
		used += height
	}
	used = 0
	for i := start; i < len(m.rows) && used < available; i++ {
		rowLines := renderTreeRow(m.rows[i], m.width, m.home, i == m.cursor)
		if used+len(rowLines) > available {
			rowLines = rowLines[:available-used]
		}
		lines = append(lines, rowLines...)
		used += len(rowLines)
	}
	if len(m.rows) == 0 && available > 0 {
		if m.loading {
			lines = append(lines, "Loading...")
		} else {
			message := "No matches"
			if m.input.Value() == "" && m.source == TreeTmux {
				message = "No OpenCode agents"
				if m.view == sessionsView {
					message = "No sessions without OpenCode"
				}
			}
			lines = append(lines, ansi.Truncate(message, max(0, m.width), "…"))
		}
	}
	nextView := "sessions"
	if m.view == sessionsView {
		nextView = "agents"
	}
	emphasis := lipgloss.NewStyle().Foreground(lipgloss.Color("#cad3f5"))
	muted := lipgloss.NewStyle().Foreground(lipgloss.Color("#a5adcb"))
	footer := "  " + emphasis.Render("Ctrl-O "+nextView+"  Enter open") + muted.Render("  ^a all  ^t tmux  ^g configs  ^x zoxide  ^f find")
	if m.err != nil {
		footer = lipgloss.NewStyle().Foreground(lipgloss.Color("#ed8796")).Render("  Refresh failed: " + safeTreeText(m.err.Error()))
	}
	if len(lines) > m.height-1 {
		lines = lines[:max(0, m.height-1)]
	}
	for len(lines) < m.height-1 {
		lines = append(lines, "")
	}
	lines = append(lines, ansi.Truncate(footer, max(0, m.width), "…"))
	view := tea.NewView(strings.Join(lines, "\n"))
	view.AltScreen = true
	return view
}

func PickTree(fetch TreeFetchFunc, query string) (TreeSelection, bool, error) {
	result, err := tea.NewProgram(NewTreeModel(fetch, query)).Run()
	if err != nil {
		return nil, false, err
	}
	switch result := result.(type) {
	case TreeModel:
		selection, ok := result.Chosen()
		return selection, ok, nil
	default:
		return nil, false, fmt.Errorf("unexpected tree picker model")
	}
}
