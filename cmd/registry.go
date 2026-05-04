package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type Registry struct {
	Roots   []string `json:"roots"`
	Default string   `json:"default,omitempty"`
}

func registryPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("user home: %w", err)
	}
	return filepath.Join(home, ".gg", "registry.json"), nil
}

func loadRegistry() (Registry, error) {
	var r Registry
	p, err := registryPath()
	if err != nil {
		return r, err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return r, nil
		}
		return r, err
	}
	if len(data) == 0 {
		return r, nil
	}
	if err := json.Unmarshal(data, &r); err != nil {
		return r, fmt.Errorf("parse registry %s: %w", p, err)
	}
	return r, nil
}

func saveRegistry(r Registry) error {
	p, err := registryPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o644)
}

func registerRoot(root string) error {
	r, err := loadRegistry()
	if err != nil {
		return err
	}
	for _, x := range r.Roots {
		if x == root {
			return nil
		}
	}
	r.Roots = append(r.Roots, root)
	return saveRegistry(r)
}

func loadAllRepos() ([]Repo, error) {
	reg, err := loadRegistry()
	if err != nil {
		return nil, err
	}
	var all []Repo
	for _, root := range reg.Roots {
		all = append(all, readIndexRepos(root)...)
	}
	return all, nil
}

func readIndexRepos(root string) []Repo {
	data, err := os.ReadFile(filepath.Join(root, ".gg", "index.json"))
	if err != nil {
		return nil
	}
	var idx Index
	if err := json.Unmarshal(data, &idx); err != nil {
		return nil
	}
	return idx.Repos
}

// effectiveDefault returns (path, isExplicit). If a default is set in the
// registry, returns it. Otherwise falls back to the first registered root
// (the "original" index). Returns ("", false) if nothing is registered.
func effectiveDefault(reg Registry) (string, bool) {
	if reg.Default != "" {
		return reg.Default, true
	}
	if len(reg.Roots) > 0 {
		return reg.Roots[0], false
	}
	return "", false
}

func loadDefaultRepos() ([]Repo, error) {
	reg, err := loadRegistry()
	if err != nil {
		return nil, err
	}
	root, _ := effectiveDefault(reg)
	if root == "" {
		return nil, nil
	}
	return readIndexRepos(root), nil
}
