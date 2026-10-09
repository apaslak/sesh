package agentstatus

import (
	"encoding/json"
	"os"
	"sync"
	"time"
)

// StartPicker holds a renewable visibility lease. It prevents the CLI bridge
// acknowledging responses covered by the tmux picker popup. Close is idempotent.
func StartPicker(directory, server string) (func(), error) {
	if err := os.MkdirAll(directory, 0700); err != nil {
		return nil, err
	}
	file, err := os.CreateTemp(directory, "picker-"+Filename(server, "")+"-")
	if err != nil {
		return nil, err
	}
	path := file.Name()
	file.Close()
	write := func() error {
		data, err := json.Marshal(struct {
			Heartbeat int64 `json:"heartbeat"`
		}{time.Now().UnixMilli()})
		if err != nil {
			return err
		}
		tmp, err := os.CreateTemp(directory, ".lease-")
		if err != nil {
			return err
		}
		defer os.Remove(tmp.Name())
		if _, err := tmp.Write(data); err != nil {
			tmp.Close()
			return err
		}
		if err := tmp.Close(); err != nil {
			return err
		}
		return os.Rename(tmp.Name(), path)
	}
	if err := write(); err != nil {
		os.Remove(path)
		return nil, err
	}
	stop, done := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				_ = write()
			case <-stop:
				return
			}
		}
	}()
	var once sync.Once
	return func() { once.Do(func() { close(stop); <-done; _ = os.Remove(path) }) }, nil
}
