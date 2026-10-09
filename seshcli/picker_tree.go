package seshcli

import (
	"fmt"
	"os"
	"time"

	"github.com/joshmedeski/sesh/v2/agentstatus"
	"github.com/joshmedeski/sesh/v2/lister"
	"github.com/joshmedeski/sesh/v2/model"
	"github.com/joshmedeski/sesh/v2/picker"
	"github.com/joshmedeski/sesh/v2/shell"
	"github.com/joshmedeski/sesh/v2/tmux"
	"github.com/spf13/cobra"
)

func runTreePicker(cmd *cobra.Command, deps *Deps) error {
	if !deps.Tmux.IsAttached() {
		return fmt.Errorf("tree picker requires tmux")
	}
	backend := tmux.NewTreeBackend(deps.Shell, deps.Config.TmuxCommand)
	directory, err := agentstatus.DefaultDirectory()
	if err != nil {
		return err
	}
	initial, err := backend.Snapshot()
	if err != nil {
		return err
	}
	close, err := agentstatus.StartPicker(directory, initial.Server)
	var reader *agentstatus.Reader
	if err == nil {
		defer close()
		reader = agentstatus.NewReader(directory)
	}
	client := deps.Tmux.ResolveClient()
	query, _ := cmd.Flags().GetString("query")
	selection, picked, err := picker.PickTree(treeFetch(deps, backend, reader), query)
	if err != nil || !picked {
		return err
	}
	switch selection := selection.(type) {
	case picker.TreeTmuxSelection:
		return backend.Focus(client, selection.Target)
	case picker.TreeNameSelection:
		_, err := deps.Connector.Connect(selection.Name, model.ConnectOpts{})
		return err
	default:
		return fmt.Errorf("unknown tree selection")
	}
}

func treeFetch(deps *Deps, backend *tmux.TreeBackend, reader *agentstatus.Reader) picker.TreeFetchFunc {
	return func(source picker.TreeSource) (picker.TreeData, error) {
		data := picker.TreeData{Statuses: map[string]agentstatus.Snapshot{}}
		if source == picker.TreeTmux || source == picker.TreeAll {
			topology, err := backend.Snapshot()
			if err != nil {
				return data, err
			}
			data.Topology = topology
			for _, session := range topology.Sessions {
				for _, window := range session.Windows {
					for _, pane := range window.Panes {
						if reader == nil {
							continue
						}
						if snapshot, err := reader.Read(topology.Server, pane.ID, time.Now()); err == nil {
							data.Statuses[pane.ID] = snapshot
						}
					}
				}
			}
		}
		if source == picker.TreeTmux {
			return data, nil
		}
		if source == picker.TreeFind {
			home, err := os.UserHomeDir()
			if err != nil {
				return data, err
			}
			entries, err := treeFind(deps.Shell, home)
			data.Entries = entries
			return data, err
		}
		opts := lister.ListOptions{}
		if source == picker.TreeConfig {
			opts.Config = true
		}
		if source == picker.TreeZoxide {
			opts.Zoxide = true
		}
		entries, err := deps.Lister.List(opts)
		if err != nil {
			return data, err
		}
		if source == picker.TreeAll {
			existing := map[string]bool{}
			for _, session := range data.Topology.Sessions {
				existing[session.Name] = true
			}
			keys := []string{}
			for _, key := range entries.OrderedIndex {
				entry := entries.Directory[key]
				if entry.Src != "tmux" && !existing[entry.Name] {
					keys = append(keys, key)
				}
			}
			entries.OrderedIndex = keys
		}
		data.Entries = entries
		return data, nil
	}
}

func treeFind(sh shell.Shell, home string) (model.SeshSessions, error) {
	paths, err := sh.ListCmd("fd", "-H", "-d", "2", "-t", "d", "-E", ".Trash", ".", home)
	if err != nil {
		return model.SeshSessions{}, fmt.Errorf("directory search requires fd: %w", err)
	}
	entries := model.SeshSessions{Directory: model.SeshSessionMap{}}
	for _, path := range paths {
		if path != "" {
			entries.Directory[path] = model.SeshSession{Name: path, Path: path, Src: "find"}
			entries.OrderedIndex = append(entries.OrderedIndex, path)
		}
	}
	return entries, nil
}
