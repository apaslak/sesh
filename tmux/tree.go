package tmux

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/joshmedeski/sesh/v2/shell"
)

// TreeTarget keeps navigation independent of mutable names and window indices.
// Session ancestry is retained because windows may be linked into many sessions.
type TreeTarget interface{ treeTarget() string }

type SessionTarget struct{ Session string }
type WindowTarget struct{ Session, Window string }
type PaneTarget struct{ Session, Window, Pane string }

func (t SessionTarget) treeTarget() string { return t.Session + ":" }
func (t WindowTarget) treeTarget() string  { return t.Session + ":" + t.Window }
func (t PaneTarget) treeTarget() string    { return t.Session + ":" + t.Window + "." + t.Pane }

// TreeSnapshot is a live server-wide inventory, including linked window branches.
type TreeSnapshot struct {
	Server   string
	Sessions []TreeSession
}
type TreeSession struct {
	ID, Name string
	Windows  []TreeWindow
}
type TreeWindow struct {
	ID, Name string
	Index    int
	Active   bool
	Panes    []TreePane
}
type TreePane struct {
	ID, Command, Path, Title string
	Index                    int
	Active                   bool
}

// TreeBackend provides live topology and exact navigation without changing the
// legacy current-session pane API.
type TreeBackend struct {
	sh  shell.Shell
	bin string
}

func NewTreeBackend(sh shell.Shell, bin string) *TreeBackend {
	if bin == "" {
		bin = "tmux"
	}
	return &TreeBackend{sh: sh, bin: bin}
}

func (b *TreeBackend) Snapshot() (TreeSnapshot, error) {
	server, err := b.sh.Cmd(b.bin, "display-message", "-p", "#{socket_path}:#{pid}")
	if err != nil {
		return TreeSnapshot{}, err
	}
	fields := []string{"session_id", "session_name", "window_id", "window_index", "window_name", "window_active", "pane_id", "pane_index", "pane_active", "pane_current_command", "pane_current_path", "pane_title"}
	for i, field := range fields {
		// Control characters cannot be allowed to split inventory records.
		// Avoid POSIX character classes: their colons confuse tmux's format
		// modifier parser. Literal record delimiters are safe in this regex.
		fields[i] = "#{s|[\t\r\n]| |:" + field + "}"
	}
	lines, err := b.sh.ListCmd(b.bin, "list-panes", "-a", "-F", strings.Join(fields, "\t"))
	if err != nil {
		return TreeSnapshot{}, err
	}
	sessions, err := parseTreePanes(lines)
	return TreeSnapshot{Server: server, Sessions: sessions}, err
}

func parseTreePanes(lines []string) ([]TreeSession, error) {
	sessions := []TreeSession{}
	for _, line := range lines {
		if line == "" {
			continue
		}
		f := strings.Split(line, "\t")
		if len(f) != 12 {
			return nil, fmt.Errorf("invalid tmux tree record")
		}
		wi, err := strconv.Atoi(f[3])
		if err != nil {
			return nil, fmt.Errorf("invalid window index: %w", err)
		}
		pi, err := strconv.Atoi(f[7])
		if err != nil {
			return nil, fmt.Errorf("invalid pane index: %w", err)
		}
		if !validTreeID(f[0], '$') || !validTreeID(f[2], '@') || !validTreeID(f[6], '%') || wi < 0 || pi < 0 || (f[5] != "0" && f[5] != "1") || (f[8] != "0" && f[8] != "1") {
			return nil, fmt.Errorf("invalid tmux tree identity")
		}
		si := -1
		for i := range sessions {
			if sessions[i].ID == f[0] {
				si = i
				break
			}
		}
		if si < 0 {
			sessions = append(sessions, TreeSession{ID: f[0], Name: f[1]})
			si = len(sessions) - 1
		}
		windows := &sessions[si].Windows
		wj := -1
		for i := range *windows {
			if (*windows)[i].ID == f[2] {
				wj = i
				break
			}
		}
		if wj < 0 {
			*windows = append(*windows, TreeWindow{ID: f[2], Name: f[4], Index: wi, Active: f[5] == "1"})
			wj = len(*windows) - 1
		}
		(*windows)[wj].Panes = append((*windows)[wj].Panes, TreePane{ID: f[6], Index: pi, Active: f[8] == "1", Command: f[9], Path: f[10], Title: f[11]})
	}
	for si := range sessions {
		sort.SliceStable(sessions[si].Windows, func(i, j int) bool { return sessions[si].Windows[i].Index < sessions[si].Windows[j].Index })
		for wi := range sessions[si].Windows {
			panes := sessions[si].Windows[wi].Panes
			sort.SliceStable(panes, func(i, j int) bool { return panes[i].Index < panes[j].Index })
		}
	}
	return sessions, nil
}

func validTreeID(id string, prefix byte) bool {
	if len(id) < 2 || id[0] != prefix {
		return false
	}
	for _, r := range id[1:] {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func (b *TreeBackend) Focus(client string, target TreeTarget) error {
	if target == nil {
		return fmt.Errorf("missing tmux target")
	}
	name := target.treeTarget()
	if _, err := b.sh.Cmd(b.bin, "display-message", "-p", "-t", name, "#{session_id}"); err != nil {
		return fmt.Errorf("tmux target disappeared: %w", err)
	}
	args := []string{"switch-client"}
	if client != "" {
		args = append(args, "-c", client)
	}
	_, err := b.sh.Cmd(b.bin, append(args, "-t", name)...)
	return err
}
