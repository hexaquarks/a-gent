package hermes

import (
	"bytes"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

//go:embed observer.py
var observer []byte

const managedMarker = "# Managed by a-gent:"

func observerDirectory() (string, error) {
	directory, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(directory, "a-gent", "hermes"), nil
}

// ConfigureObserver installs the observer in Hermes' normal plugin directory
// and enables it while preserving the other configuration values and comments.
func ConfigureObserver() (string, error) {
	directory, err := hermesHome()
	if err != nil {
		return "", err
	}
	path, err := installObserver(filepath.Join(directory, "plugins", "a-gent"))
	if err != nil {
		return "", err
	}
	if err := enablePlugin(directory); err != nil {
		return "", fmt.Errorf("enable Hermes observer: %w", err)
	}
	return path, nil
}

func hermesHome() (string, error) {
	if directory := os.Getenv("HERMES_HOME"); directory != "" {
		return filepath.Abs(directory)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	directory := filepath.Join(home, ".hermes")
	active, err := os.ReadFile(filepath.Join(directory, "active_profile"))
	if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	profile := strings.TrimSpace(string(active))
	if profile != "" && profile != "default" {
		if filepath.Base(profile) != profile || profile == "." || profile == ".." {
			return "", fmt.Errorf("invalid Hermes active profile")
		}
		directory = filepath.Join(directory, "profiles", profile)
	}
	return directory, nil
}

func installObserver(directory string) (string, error) {
	files := map[string][]byte{
		"__init__.py": observer,
		"plugin.yaml": []byte(managedMarker + " native Hermes observer.\n" +
			"name: a-gent\nversion: 1.0.0\n" +
			"description: Live session status and public previews for a-gent\n"),
	}
	// Check every destination before replacing either file.
	for name := range files {
		path := filepath.Join(directory, name)
		existing, err := os.ReadFile(path)
		if err == nil && !bytes.HasPrefix(existing, []byte(managedMarker)) {
			return "", fmt.Errorf("refusing to replace existing Hermes plugin %s", path)
		}
		if err != nil && !os.IsNotExist(err) {
			return "", err
		}
	}
	for name, data := range files {
		if err := writeObserverFile(directory, name, data); err != nil {
			return "", err
		}
	}
	return filepath.Join(directory, "__init__.py"), nil
}

func writeObserverFile(directory, name string, data []byte) error {
	if err := os.MkdirAll(directory, 0700); err != nil {
		return err
	}
	path := filepath.Join(directory, name)
	if existing, err := os.ReadFile(path); err == nil && bytes.Equal(existing, data) {
		return nil
	}
	file, err := os.CreateTemp(directory, ".a-gent-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	_, writeErr := file.Write(data)
	closeErr := file.Close()
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(file.Name(), path)
}
