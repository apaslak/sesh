// Package agentstatus reads the local OpenCode CLI bridge without requiring an
// OpenCode server connection. Missing or untrusted records are unavailable.
package agentstatus

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Status is a sealed sum; only the decoder can create a parsed snapshot status.
type Status interface {
	Label() string
	status()
}
type Idle struct{}
type Working struct{ Execution string }
type Attention struct{ Reason string }
type Finished struct {
	Execution string
	Read      bool
}

func (Idle) status()              {}
func (Working) status()           {}
func (Attention) status()         {}
func (Finished) status()          {}
func (Idle) Label() string        { return "idle" }
func (Working) Label() string     { return "working" }
func (a Attention) Label() string { return "needs attention · " + a.Reason }
func (f Finished) Label() string {
	if f.Read {
		return "finished · read"
	}
	return "finished · unread"
}

// Snapshot contains display metadata only, never prompts or conversation text.
type Snapshot struct {
	Conversation, Title, Agent, Directory string
	Status                                Status
	heartbeat                             int64
}
type Reader struct{ directory string }

func NewReader(directory string) *Reader { return &Reader{directory: directory} }

// Filename shares a server-lifetime/pane namespace with the CLI bridge.
func Filename(server, pane string) string {
	hash := sha256.Sum256([]byte(server + "\x00" + pane))
	return hex.EncodeToString(hash[:]) + ".json"
}

func DefaultDirectory() (string, error) {
	base := os.Getenv("XDG_STATE_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(base, "sesh", "opencode"), nil
}

func (r *Reader) Read(server, pane string, now time.Time) (Snapshot, error) {
	names, err := os.ReadDir(r.directory)
	if err != nil {
		return Snapshot{}, err
	}
	base := Filename(server, pane)
	var newest Snapshot
	for _, name := range names {
		if name.IsDir() || strings.HasSuffix(name.Name(), ".tmp") {
			continue
		}
		if name.Name() != base && !strings.HasPrefix(name.Name(), base+"-") {
			continue
		}
		snapshot, err := r.readPath(filepath.Join(r.directory, name.Name()), server, pane, now)
		if err == nil && (newest.Status == nil || snapshot.heartbeat > newest.heartbeat) {
			newest = snapshot
		}
	}
	if newest.Status == nil {
		return Snapshot{}, fmt.Errorf("status unavailable")
	}
	return newest, nil
}

func (r *Reader) readPath(path, server, pane string, now time.Time) (Snapshot, error) {
	file, err := os.Open(path)
	if err != nil {
		return Snapshot{}, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, 65537))
	if err != nil {
		return Snapshot{}, err
	}
	if len(data) > 65536 {
		return Snapshot{}, fmt.Errorf("status snapshot too large")
	}
	var raw struct {
		Version      int    `json:"version"`
		Server       string `json:"server"`
		Pane         string `json:"pane"`
		Owner        string `json:"owner"`
		Conversation string `json:"conversation"`
		Title        string `json:"title"`
		Agent        string `json:"agent"`
		Directory    string `json:"directory"`
		Heartbeat    int64  `json:"heartbeat"`
		Status       struct {
			Kind      string `json:"kind"`
			Execution string `json:"execution"`
			Read      *bool  `json:"read"`
			Reason    string `json:"reason"`
		} `json:"status"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return Snapshot{}, err
	}
	age := now.Sub(time.UnixMilli(raw.Heartbeat))
	if raw.Version != 1 || raw.Server != server || raw.Pane != pane || raw.Owner == "" || raw.Conversation == "" || age > 5*time.Second || age < -5*time.Second {
		return Snapshot{}, fmt.Errorf("unavailable status snapshot")
	}
	var status Status
	switch raw.Status.Kind {
	case "idle":
		if raw.Status.Read != nil || raw.Status.Execution != "" {
			return Snapshot{}, fmt.Errorf("invalid idle status")
		}
		status = Idle{}
	case "working":
		if raw.Status.Execution == "" || raw.Status.Read != nil {
			return Snapshot{}, fmt.Errorf("invalid working status")
		}
		status = Working{Execution: raw.Status.Execution}
	case "attention":
		if raw.Status.Reason == "" || raw.Status.Read != nil {
			return Snapshot{}, fmt.Errorf("invalid attention status")
		}
		status = Attention{Reason: raw.Status.Reason}
	case "finished":
		if raw.Status.Execution == "" || raw.Status.Read == nil {
			return Snapshot{}, fmt.Errorf("invalid finished status")
		}
		status = Finished{Execution: raw.Status.Execution, Read: *raw.Status.Read}
	default:
		return Snapshot{}, fmt.Errorf("unknown status kind")
	}
	return Snapshot{Conversation: raw.Conversation, Title: raw.Title, Agent: raw.Agent, Directory: raw.Directory, Status: status, heartbeat: raw.Heartbeat}, nil
}
