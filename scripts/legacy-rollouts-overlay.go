// Generate the legacy Enterprise Rollouts compatibility layer. The native Core
// contract stays byte-exact; no current Core path or component is overwritten.
package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"sigs.k8s.io/yaml"
)

const (
	coreSHA        = "a8e547bdaaa17c544330c1b68f034b65673f095f935e8e8a94d1aa7a17cf7777"
	legacySHA      = "41a1f9a231d9dc0c5850e381a44caa938330099b7ff730b0b834f8d950e2f1e1"
	legacyRevision = "3a10169d37b40f66adea76c530da81953a91b7ad"
)

type document struct {
	Paths      map[string]json.RawMessage            `json:"paths"`
	Components map[string]map[string]json.RawMessage `json:"components"`
}

func readDocument(filename, expectedSHA string) (document, error) {
	var result document
	root, err := os.OpenRoot(".")
	if err != nil {
		return result, err
	}
	raw, err := root.ReadFile(filename)
	closeErr := root.Close()
	if err != nil {
		return result, err
	}
	if closeErr != nil {
		return result, closeErr
	}
	if fmt.Sprintf("%x", sha256.Sum256(raw)) != expectedSHA {
		return result, fmt.Errorf("%s differs from its immutable source pin", filename)
	}
	encoded, err := yaml.YAMLToJSON(raw)
	if err != nil {
		return result, err
	}
	err = json.Unmarshal(encoded, &result)
	return result, err
}

func references(raw json.RawMessage) ([]string, error) {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, err
	}
	var result []string
	var walk func(any)
	walk = func(value any) {
		switch typed := value.(type) {
		case map[string]any:
			if ref, ok := typed["$ref"].(string); ok {
				result = append(result, ref)
			}
			for _, child := range typed {
				walk(child)
			}
		case []any:
			for _, child := range typed {
				walk(child)
			}
		}
	}
	walk(value)
	return result, nil
}

func run() error {
	core, err := readDocument("spec.yaml", coreSHA)
	if err != nil {
		return err
	}
	legacy, err := readDocument("legacy-enterprise-spec.yaml", legacySHA)
	if err != nil {
		return err
	}
	paths := map[string]json.RawMessage{}
	components := map[string]map[string]json.RawMessage{}
	var pending []json.RawMessage
	for path, item := range legacy.Paths {
		if path != "/orgs/{orgId}/rollouts" && !strings.HasPrefix(path, "/orgs/{orgId}/rollouts/") {
			continue
		}
		if _, exists := core.Paths[path]; exists {
			return fmt.Errorf("legacy overlay would overwrite Core path %s", path)
		}
		paths[path] = item
		pending = append(pending, item)
	}
	if len(paths) == 0 {
		return fmt.Errorf("legacy Rollouts paths are missing")
	}
	for len(pending) > 0 {
		refs, err := references(pending[0])
		if err != nil {
			return err
		}
		pending = pending[1:]
		for _, ref := range refs {
			parts := strings.Split(ref, "/")
			if len(parts) != 4 || parts[0] != "#" || parts[1] != "components" {
				return fmt.Errorf("unsupported legacy reference %s", ref)
			}
			category, name := parts[2], parts[3]
			if _, exists := core.Components[category][name]; exists {
				continue
			}
			if _, exists := components[category][name]; exists {
				continue
			}
			item, exists := legacy.Components[category][name]
			if !exists {
				return fmt.Errorf("unresolved legacy reference %s", ref)
			}
			if components[category] == nil {
				components[category] = map[string]json.RawMessage{}
			}
			components[category][name] = item
			pending = append(pending, item)
		}
	}
	overlay := map[string]any{
		"overlay": "1.0.0",
		"info":    map[string]string{"title": "Legacy Enterprise Rollouts compatibility only, not native Core routes", "version": legacyRevision},
		"actions": []any{map[string]any{"target": "$", "description": "Add only immutable legacy Rollouts paths and their missing components",
			"update": map[string]any{"paths": paths, "components": components}}},
	}
	encoded, err := json.MarshalIndent(overlay, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile("legacy-rollouts-overlay.yaml", append(encoded, '\n'), 0o600)
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
