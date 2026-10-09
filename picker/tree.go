package picker

import (
	"fmt"
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

// TreeModel is opt-in; it deliberately does not inherit preview/deletion actions.
type TreeModel struct {
	fetch                 TreeFetchFunc
	source                TreeSource
	generation            uint64
	input                 textinput.Model
	all, rows             []treeRow
	collapsed             map[string]bool
	cursor, width, height int
	loading               bool
	err                   error
	chosen                TreeSelection
	quit                  bool
}

func NewTreeModel(fetch TreeFetchFunc, query string) TreeModel {
	input := textinput.New()
	input.Prompt = "> "
	input.Placeholder = "Filter sessions, windows, panes, conversations..."
	input.SetValue(query)
	input.Focus()
	return TreeModel{fetch: fetch, source: TreeTmux, input: input, collapsed: map[string]bool{}, width: 80, height: 24, loading: fetch != nil}
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
	m.rows = filterTreeRows(m.all, m.input.Value(), m.collapsed)
	for key != "" {
		for i := range m.rows {
			if m.rows[i].key == key {
				m.cursor = i
				return
			}
		}
		at := strings.LastIndex(key, "/")
		if at < 0 {
			break
		}
		key = key[:at]
	}
	m.cursor = max(0, min(m.cursor, len(m.rows)-1))
}
func (m *TreeModel) accept(data TreeData) {
	key := m.selectedKey()
	m.all = buildTreeRows(data)
	m.filter(key)
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
		case "left", "right":
			if len(m.rows) == 0 {
				return m, nil
			}
			row := m.rows[m.cursor]
			if key == "right" {
				if row.branch {
					delete(m.collapsed, row.key)
				}
			} else if row.branch && !m.collapsed[row.key] {
				m.collapsed[row.key] = true
			} else {
				for i := range m.rows {
					if m.rows[i].key == row.parent {
						m.cursor = i
						break
					}
				}
			}
			m.filter(m.selectedKey())
			return m, nil
		case "ctrl+a", "ctrl+t", "ctrl+g", "ctrl+x", "ctrl+f":
			sources := map[string]TreeSource{"ctrl+a": TreeAll, "ctrl+t": TreeTmux, "ctrl+g": TreeConfig, "ctrl+x": TreeZoxide, "ctrl+f": TreeFind}
			m.source = sources[key]
			m.generation++
			m.loading = m.fetch != nil
			m.all = nil
			m.rows = nil
			m.cursor = 0
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
	lines := []string{"sesh · " + m.source.String(), m.input.View()}
	available := max(0, m.height-4)
	start := max(0, m.cursor-available+1)
	for i := start; i < min(len(m.rows), start+available); i++ {
		row := m.rows[i]
		branch := "  "
		if row.branch {
			branch = "▾ "
			if m.collapsed[row.key] && m.input.Value() == "" {
				branch = "▸ "
			}
		}
		line := "  " + strings.Repeat("  ", row.depth) + branch + row.label
		line = ansi.Truncate(line, max(0, m.width), "…")
		if i == m.cursor {
			line = lipgloss.NewStyle().Reverse(true).Render(line)
		}
		lines = append(lines, line)
	}
	if len(m.rows) == 0 && available > 0 {
		if m.loading {
			lines = append(lines, "Loading...")
		} else {
			lines = append(lines, "No matches")
		}
	}
	footer := "^a all  ^t tmux  ^g configs  ^x zoxide  ^f find  ←/→ fold"
	if m.err != nil {
		footer = "Refresh failed: " + safeTreeText(m.err.Error())
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
