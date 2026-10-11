package pi

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

//go:embed observer.ts
var observer []byte

func observerDirectory() (string, error) {
	directory, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(directory, "a-gent", "pi"), nil
}

func installObserver(directory string) (string, error) {
	path := filepath.Join(directory, "observer.ts")
	if err := writeObserverFile(directory, "observer.ts", observer); err != nil {
		return "", fmt.Errorf("install Pi observer: %w", err)
	}
	return path, nil
}

// ConfigureObserver registers the observer as a normal Pi user extension so
// sessions started from a shell are observed as well as those launched by a-gent.
func ConfigureObserver() (string, error) {
	cacheDirectory, err := observerDirectory()
	if err != nil {
		return "", err
	}
	path, err := installObserver(cacheDirectory)
	if err != nil {
		return "", err
	}
	directory := os.Getenv("PI_CODING_AGENT_DIR")
	if directory == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		directory = filepath.Join(home, ".pi", "agent")
	}
	// Keep the runtime metadata in a-gent's cache even when Pi uses a custom
	// agent directory. Loading this module does not change Pi's other settings.
	modulePath, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	quoted, _ := json.Marshal(filepath.ToSlash(modulePath))
	loader := []byte("// Managed by a-gent: native Pi status observer.\nexport { default } from " + string(quoted) + ";\n")
	loaderPath := filepath.Join(directory, "extensions", "a-gent.ts")
	existing, err := os.ReadFile(loaderPath)
	if err == nil && !bytes.HasPrefix(existing, []byte("// Managed by a-gent:")) {
		return "", fmt.Errorf("refusing to replace existing Pi extension %s", loaderPath)
	}
	if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	if err := writeObserverFile(filepath.Join(directory, "extensions"), "a-gent.ts", loader); err != nil {
		return "", fmt.Errorf("register Pi observer: %w", err)
	}
	return path, nil
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
