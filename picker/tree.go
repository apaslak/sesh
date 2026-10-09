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

// treeListState restores the unified tmux list after visiting another source.
type treeListState struct {
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
	tmuxState             treeListState
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
	m.all = buildTreeRows(data)
	m.filter(key)
}

func (m *TreeModel) saveTmuxState() {
	if m.loading && len(m.rows) == 0 {
		m.tmuxState.query = m.input.Value()
		return
	}
	m.tmuxState = treeListState{query: m.input.Value(), key: m.selectedKey(), cursor: m.cursor}
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
		case "tab", "up", "ctrl+p":
			m.cursor = min(m.cursor+1, max(0, len(m.rows)-1))
			return m, nil
		case "shift+tab", "down", "ctrl+n":
			m.cursor = max(0, m.cursor-1)
			return m, nil
		case "ctrl+o":
			return m, nil
		case "ctrl+a", "ctrl+t", "ctrl+g", "ctrl+x", "ctrl+f":
			if m.source == TreeTmux {
				m.saveTmuxState()
			}
			sources := map[string]TreeSource{"ctrl+a": TreeAll, "ctrl+t": TreeTmux, "ctrl+g": TreeConfig, "ctrl+x": TreeZoxide, "ctrl+f": TreeFind}
			m.source = sources[key]
			m.generation++
			m.loading = m.fetch != nil
			m.all = nil
			m.rows = nil
			m.cursor = 0
			m.pendingKey = ""
			if m.source == TreeTmux {
				state := m.tmuxState
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
	header := "Tmux"
	if m.source != TreeTmux {
		header = m.source.String()
	}
	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#c6a0f6")).Render(header)
	count := lipgloss.NewStyle().Foreground(lipgloss.Color("#a5adcb")).Render(fmt.Sprintf(" · %d", len(m.rows)))
	var lines []string
	if m.height >= 3 {
		lines = append(lines, ansi.Truncate("  "+title+count, m.width, "…"))
	}
	available := max(0, m.height-4)
	var resultLines []string
	if len(m.rows) > 0 && available > 0 {
		// Keep ranking/selection in logical order. Only reverse whole visible
		// blocks, so each agent's path stays below its title.
		start := min(m.cursor, len(m.rows)-1)
		selected := renderTreeRow(m.rows[start], m.width, m.home, true)
		selected = selected[:min(len(selected), available)]
		blocks := [][]string{selected}
		used := len(selected)
		for start > 0 {
			block := renderTreeRow(m.rows[start-1], m.width, m.home, false)
			if used+len(block) > available {
				break
			}
			start--
			blocks = append(blocks, block)
			used += len(block)
		}
		for _, block := range blocks {
			resultLines = append(resultLines, block...)
		}
		// Later-ranked rows go above the selection and earlier-ranked rows.
		blocks = nil
		for i := m.cursor + 1; i < len(m.rows); i++ {
			block := renderTreeRow(m.rows[i], m.width, m.home, false)
			if used+len(block) > available {
				break
			}
			blocks = append(blocks, block)
			used += len(block)
		}
		var upperLines []string
		for i := len(blocks) - 1; i >= 0; i-- {
			upperLines = append(upperLines, blocks[i]...)
		}
		resultLines = append(upperLines, resultLines...)
	}
	if len(m.rows) == 0 && available > 0 {
		if m.loading {
			resultLines = append(resultLines, ansi.Truncate("Loading...", m.width, "…"))
		} else {
			message := "No matches"
			if m.input.Value() == "" && m.source == TreeTmux {
				message = "No tmux sessions"
			}
			resultLines = append(resultLines, ansi.Truncate(message, m.width, "…"))
		}
	}
	for padding := available - len(resultLines); padding > 0; padding-- {
		lines = append(lines, "")
	}
	lines = append(lines, resultLines...)
	if m.height >= 4 {
		lines = append(lines, "")
	}
	lines = append(lines, ansi.Truncate(m.input.View(), m.width, "…"))
	emphasis := lipgloss.NewStyle().Foreground(lipgloss.Color("#cad3f5"))
	muted := lipgloss.NewStyle().Foreground(lipgloss.Color("#a5adcb"))
	footer := "  " + emphasis.Render("Enter open") + muted.Render("  ^a all  ^t tmux  ^g configs  ^x zoxide  ^f find")
	if m.err != nil {
		footer = lipgloss.NewStyle().Foreground(lipgloss.Color("#ed8796")).Render("  Refresh failed: " + safeTreeText(m.err.Error()))
	}
	if m.height >= 2 {
		lines = append(lines, ansi.Truncate(footer, m.width, "…"))
	}
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
