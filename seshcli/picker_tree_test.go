package seshcli

import (
	"testing"

	"github.com/joshmedeski/sesh/v2/lister"
	"github.com/joshmedeski/sesh/v2/model"
	"github.com/joshmedeski/sesh/v2/picker"
	"github.com/joshmedeski/sesh/v2/shell"
	"github.com/joshmedeski/sesh/v2/tmux"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestPickerTreeFlag(t *testing.T) {
	cmd := NewPickerCommand(&BaseDeps{})
	require.NotNil(t, cmd.Flags().Lookup("tree"))
}

func TestTreeSourceLoaders(t *testing.T) {
	for _, tc := range []struct {
		source picker.TreeSource
		opts   lister.ListOptions
	}{
		{picker.TreeConfig, lister.ListOptions{Config: true}},
		{picker.TreeZoxide, lister.ListOptions{Zoxide: true}},
	} {
		li := lister.NewMockLister(t)
		entries := model.SeshSessions{Directory: model.SeshSessionMap{"entry": {Name: "entry", Src: "config"}}, OrderedIndex: []string{"entry"}}
		li.EXPECT().List(tc.opts).Return(entries, nil)
		fetch := treeFetch(&Deps{Lister: li}, nil, nil)
		data, err := fetch(tc.source)
		require.NoError(t, err)
		require.Equal(t, entries, data.Entries)
	}
}

func TestTreeDirectorySearch(t *testing.T) {
	sh := shell.NewMockShell(t)
	sh.EXPECT().ListCmd("fd", "-H", "-d", "2", "-t", "d", "-E", ".Trash", ".", "/home/amy").Return([]string{"/home/amy/project/"}, nil)
	entries, err := treeFind(sh, "/home/amy")
	require.NoError(t, err)
	require.Equal(t, "/home/amy/project/", entries.Directory[entries.OrderedIndex[0]].Name)
}

func TestTreeMissingStatusStorageDoesNotBlockNavigation(t *testing.T) {
	sh := shell.NewMockShell(t)
	sh.EXPECT().Cmd("tmux", "display-message", "-p", "#{socket_path}:#{pid}").Return("server", nil)
	sh.EXPECT().ListCmd("tmux", "list-panes", "-a", "-F", mock.Anything).Return([]string{"$1\twork\t@1\t0\tmain\t1\t%1\t0\t1\topencode\t/work\tagent"}, nil)
	data, err := treeFetch(&Deps{}, tmux.NewTreeBackend(sh, "tmux"), nil)(picker.TreeTmux)
	require.NoError(t, err)
	require.Len(t, data.Topology.Sessions, 1)
	require.Empty(t, data.Statuses)
}
