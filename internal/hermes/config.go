package hermes

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

func mappingValue(mapping *yaml.Node, key string, kind yaml.Kind) (*yaml.Node, error) {
	for i := 0; i < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value != key {
			continue
		}
		value := mapping.Content[i+1]
		if value.Kind != kind {
			return nil, fmt.Errorf("Hermes %s must be a %s", key, kindName(kind))
		}
		return value, nil
	}
	value := &yaml.Node{Kind: kind}
	mapping.Content = append(mapping.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, value)
	return value, nil
}

func kindName(kind yaml.Kind) string {
	if kind == yaml.MappingNode {
		return "mapping"
	}
	return "list"
}

// Change only the enabled list. YAML nodes retain comments and unknown settings;
// an explicit disable is a user preference that automatic setup must respect.
func enablePlugin(directory string) error {
	path := filepath.Join(directory, "config.yaml")
	if info, err := os.Lstat(path); err == nil && info.Mode()&os.ModeSymlink != 0 {
		resolved, err := filepath.EvalSymlinks(path)
		if err != nil {
			return err
		}
		path = resolved
	}
	original, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	var document yaml.Node
	decoder := yaml.NewDecoder(bytes.NewReader(original))
	if err := decoder.Decode(&document); err != nil && err != io.EOF {
		return err
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); err != io.EOF {
		return fmt.Errorf("Hermes config must contain one YAML document")
	}
	if len(document.Content) == 0 {
		document.Kind = yaml.DocumentNode
		document.Content = []*yaml.Node{{Kind: yaml.MappingNode, Tag: "!!map"}}
	}
	// Decode once into a value to reject duplicate keys before editing nodes.
	var validated map[string]any
	if err := yaml.Unmarshal(original, &validated); err != nil {
		return err
	}
	root := document.Content[0]
	if root.Kind != yaml.MappingNode {
		return fmt.Errorf("Hermes config must be a mapping")
	}
	plugins, err := mappingValue(root, "plugins", yaml.MappingNode)
	if err != nil {
		return err
	}
	for i := 0; i < len(plugins.Content); i += 2 {
		if plugins.Content[i].Value != "disabled" {
			continue
		}
		value := plugins.Content[i+1]
		if value.Kind != yaml.SequenceNode {
			return fmt.Errorf("Hermes plugins.disabled must be a list")
		}
		for _, name := range value.Content {
			if name.Value == "a-gent" {
				return fmt.Errorf("a-gent is explicitly disabled; enable it in Hermes to use live status")
			}
		}
	}
	enabled, err := mappingValue(plugins, "enabled", yaml.SequenceNode)
	if err != nil {
		return err
	}
	for _, name := range enabled.Content {
		if name.Kind != yaml.ScalarNode || name.Tag != "!!str" {
			return fmt.Errorf("Hermes plugins.enabled must contain plugin names")
		}
		if name.Value == "a-gent" {
			return nil
		}
	}
	enabled.Content = append(enabled.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "a-gent"})
	var output bytes.Buffer
	encoder := yaml.NewEncoder(&output)
	encoder.SetIndent(2)
	if err := encoder.Encode(&document); err != nil {
		return err
	}
	if err := encoder.Close(); err != nil {
		return err
	}
	// Avoid replacing a config edited while setup was preparing its update.
	current, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if !bytes.Equal(current, original) {
		return fmt.Errorf("Hermes config changed during observer setup; retry")
	}
	return writeObserverFile(filepath.Dir(path), filepath.Base(path), output.Bytes())
}
