package opencode

import (
	"bytes"
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

//go:embed observer.ts
var observer []byte

// ConfigureObserver registers a read-only native SDK bridge for plain TUI launches.
// It leaves OpenCode's configuration and other plugins untouched.
func ConfigureObserver() error {
	directory := os.Getenv("OPENCODE_CONFIG_DIR")
	if directory == "" {
		directory = os.Getenv("XDG_CONFIG_HOME")
		if directory == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return err
			}
			directory = filepath.Join(home, ".config")
		}
		directory = filepath.Join(directory, "opencode")
	}
	directory = filepath.Join(directory, "plugins")
	if err := os.MkdirAll(directory, 0700); err != nil {
		return err
	}
	path := filepath.Join(directory, "a-gent.ts")
	existing, err := os.ReadFile(path)
	if err == nil && bytes.Equal(existing, observer) {
		return nil
	}
	if err == nil && !bytes.HasPrefix(existing, []byte("// OpenCode's plain TUI")) {
		return fmt.Errorf("refusing to replace existing OpenCode plugin %s", path)
	}
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	file, err := os.CreateTemp(directory, ".a-gent-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	_, writeErr := file.Write(observer)
	closeErr := file.Close()
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(file.Name(), path)
}

// EnableObserver prepares normal shell launches when OpenCode is installed.
func (adapter *Adapter) EnableObserver() {
	if _, err := exec.LookPath("opencode"); err == nil {
		adapter.setupErr = ConfigureObserver()
	}
}
