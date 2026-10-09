package agentstatus

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSnapshotRoundTrip(t *testing.T) {
	now := time.UnixMilli(10000)
	dir := t.TempDir()
	path := filepath.Join(dir, Filename("/socket:42", "%1"))
	data := `{"version":1,"server":"/socket:42","pane":"%1","owner":"process-1","conversation":"ses_1","title":"Review","agent":"build","heartbeat":10000,"status":{"kind":"finished","execution":"exec_1","read":false}}`
	require.NoError(t, os.WriteFile(path, []byte(data), 0600))
	snapshot, err := NewReader(dir).Read("/socket:42", "%1", now)
	require.NoError(t, err)
	require.Equal(t, "Review", snapshot.Title)
	require.Equal(t, "finished · unread", snapshot.Status.Label())
	_, err = NewReader(dir).Read("/socket:42", "%1", now.Add(5*time.Second+time.Millisecond))
	require.Error(t, err)
	_, err = NewReader(dir).Read("other", "%1", now)
	require.Error(t, err)
}

func TestSnapshotRejectsInvalidState(t *testing.T) {
	for _, state := range []string{`{"kind":"bogus"}`, `{"kind":"finished","read":false}`, `{"kind":"working"}`, `{"kind":"idle","read":true}`} {
		dir := t.TempDir()
		data := `{"version":1,"server":"s","pane":"%1","owner":"o","conversation":"ses_1","heartbeat":10000,"status":` + state + `}`
		require.NoError(t, os.WriteFile(filepath.Join(dir, Filename("s", "%1")), []byte(data), 0600))
		_, err := NewReader(dir).Read("s", "%1", time.UnixMilli(10000))
		require.Error(t, err)
	}
}

func TestSnapshotBounds(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, Filename("s", "%1"))
	require.NoError(t, os.WriteFile(path, make([]byte, 65537), 0600))
	_, err := NewReader(dir).Read("s", "%1", time.Now())
	require.Error(t, err)
}

func TestPickerLeaseIsRemovedOnClose(t *testing.T) {
	dir := t.TempDir()
	close, err := StartPicker(dir, "s")
	require.NoError(t, err)
	names, err := filepath.Glob(filepath.Join(dir, "picker-*.json-*"))
	require.NoError(t, err)
	require.Len(t, names, 1)
	close()
	names, err = filepath.Glob(filepath.Join(dir, "picker-*.json-*"))
	require.NoError(t, err)
	require.Empty(t, names)
}

func TestSnapshotUsesNewestLiveOwner(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		owner     string
		heartbeat string
		title     string
	}{{"old", "9000", "Old"}, {"new", "10000", "New"}} {
		data := `{"version":1,"server":"s","pane":"%1","owner":"` + tc.owner + `","conversation":"ses_1","title":"` + tc.title + `","heartbeat":` + tc.heartbeat + `,"status":{"kind":"idle"}}`
		require.NoError(t, os.WriteFile(filepath.Join(dir, Filename("s", "%1")+"-"+tc.owner), []byte(data), 0600))
	}
	snapshot, err := NewReader(dir).Read("s", "%1", time.UnixMilli(10000))
	require.NoError(t, err)
	require.Equal(t, "New", snapshot.Title)
}

func TestSnapshotIgnoresUnpublishedTemporaryFiles(t *testing.T) {
	dir := t.TempDir()
	data := `{"version":1,"server":"s","pane":"%1","owner":"o","conversation":"ses_1","heartbeat":10000,"status":{"kind":"idle"}}`
	require.NoError(t, os.WriteFile(filepath.Join(dir, Filename("s", "%1")+"-owner.uuid.tmp"), []byte(data), 0600))
	_, err := NewReader(dir).Read("s", "%1", time.UnixMilli(10000))
	require.Error(t, err)
}
