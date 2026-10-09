package tmux

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/joshmedeski/sesh/v2/execwrap"
	"github.com/joshmedeski/sesh/v2/home"
	"github.com/joshmedeski/sesh/v2/oswrap"
	"github.com/joshmedeski/sesh/v2/shell"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestTreeSnapshotAllSessions(t *testing.T) {
	sh := shell.NewMockShell(t)
	sh.EXPECT().Cmd("custom-tmux", "display-message", "-p", "#{socket_path}:#{pid}").Return("/socket:42", nil)
	sh.EXPECT().ListCmd("custom-tmux", "list-panes", "-a", "-F", mock.Anything).Return([]string{
		"$1\twork space\t@1\t0\tmain\t1\t%1\t0\t1\tzsh\t/home/amy\tshell",
		"$1\twork space\t@1\t0\tmain\t1\t%2\t1\t0\topencode\t/home/amy\tagent",
		"$2\tother\t@1\t3\tmain\t1\t%1\t0\t1\tzsh\t/home/amy\tshell",
	}, nil)
	snapshot, err := NewTreeBackend(sh, "custom-tmux").Snapshot()
	require.NoError(t, err)
	require.Equal(t, "/socket:42", snapshot.Server)
	require.Len(t, snapshot.Sessions, 2)
	require.Equal(t, "work space", snapshot.Sessions[0].Name)
	require.Len(t, snapshot.Sessions[0].Windows[0].Panes, 2)
	require.Equal(t, "@1", snapshot.Sessions[1].Windows[0].ID)
	require.Equal(t, 3, snapshot.Sessions[1].Windows[0].Index)
}

func TestTreeFocusStableTargets(t *testing.T) {
	for _, tc := range []struct {
		target   TreeTarget
		expected string
	}{
		{SessionTarget{Session: "$1"}, "$1:"},
		{WindowTarget{Session: "$1", Window: "@2"}, "$1:@2"},
		{PaneTarget{Session: "$1", Window: "@2", Pane: "%3"}, "$1:@2.%3"},
	} {
		sh := shell.NewMockShell(t)
		sh.EXPECT().Cmd("tmux", "display-message", "-p", "-t", tc.expected, "#{session_id}").Return("$1", nil)
		sh.EXPECT().Cmd("tmux", "switch-client", "-c", "/dev/ttys000", "-t", tc.expected).Return("", nil)
		require.NoError(t, NewTreeBackend(sh, "tmux").Focus("/dev/ttys000", tc.target))
	}
}

func TestTreeFocusMissingTarget(t *testing.T) {
	sh := shell.NewMockShell(t)
	sh.EXPECT().Cmd("tmux", "display-message", "-p", "-t", "$1:@2.%3", "#{session_id}").Return("", errors.New("missing pane"))
	require.Error(t, NewTreeBackend(sh, "tmux").Focus("client", PaneTarget{Session: "$1", Window: "@2", Pane: "%3"}))
}

func TestTreeRejectsMalformedInventory(t *testing.T) {
	_, err := parseTreePanes([]string{"$1\twork\t@1\tinvalid\tmain\t1\t%1\t0\t1\tzsh\t/home\tshell"})
	require.Error(t, err)
}

func TestTreeLiveInventory(t *testing.T) {
	bin, err := exec.LookPath("tmux")
	if err != nil {
		t.Skip("tmux not installed")
	}
	socket := fmt.Sprintf("sesh-tree-test-%d-%d", os.Getpid(), time.Now().UnixNano())
	dirBase, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	dir := filepath.Join(dirBase, "foo\tbar\nbaz")
	require.NoError(t, os.Mkdir(dir, 0700))
	output, err := exec.Command(bin, "-L", socket, "-f", "/dev/null", "new-session", "-d", "-s", "first", "-c", dir, "/bin/sh").CombinedOutput()
	require.NoError(t, err, string(output))
	t.Cleanup(func() { _ = exec.Command(bin, "-L", socket, "kill-server").Run() })
	output, err = exec.Command(bin, "-L", socket, "new-session", "-d", "-s", "second", "/bin/sh").CombinedOutput()
	require.NoError(t, err, string(output))
	output, err = exec.Command(bin, "-L", socket, "link-window", "-s", "first:0", "-t", "second:5").CombinedOutput()
	require.NoError(t, err, string(output))
	wrapper := filepath.Join(t.TempDir(), "tmux")
	quoted := "'" + strings.ReplaceAll(bin, "'", "'\\''") + "'"
	require.NoError(t, os.WriteFile(wrapper, []byte("#!/bin/sh\nexec "+quoted+" -L "+socket+" \"$@\"\n"), 0700))
	sh := shell.NewShell(execwrap.NewExec(), home.NewHome(oswrap.NewOs()))
	snapshot, err := NewTreeBackend(sh, wrapper).Snapshot()
	require.NoError(t, err)
	require.Len(t, snapshot.Sessions, 2)
	require.Equal(t, "first", snapshot.Sessions[0].Name)
	require.Equal(t, "second", snapshot.Sessions[1].Name)
	require.Equal(t, 0, snapshot.Sessions[0].Windows[0].Index)
	require.Equal(t, filepath.Join(dirBase, "foo bar baz"), snapshot.Sessions[0].Windows[0].Panes[0].Path)
	require.Len(t, snapshot.Sessions[1].Windows, 2)
	require.Equal(t, snapshot.Sessions[0].Windows[0].ID, snapshot.Sessions[1].Windows[1].ID)
}
