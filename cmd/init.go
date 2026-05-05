package cmd

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"
)

type Repo struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	Remote    string `json:"remote,omitempty"`
	Branch    string `json:"branch,omitempty"`
	HeadSHA   string `json:"head_sha,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
	Diverged  bool   `json:"diverged,omitempty"`
}

type Index struct {
	Root      string    `json:"root"`
	IndexedAt time.Time `json:"indexed_at"`
	StateHash string    `json:"state_hash,omitempty"`
	Repos     []Repo    `json:"repos"`
}

const defaultMaxDepth = 6
const inspectConcurrency = 16

var (
	initMaxDepth int
	initOutput   string
	initNoShell  bool
)

var initCmd = &cobra.Command{
	Use:     "init [folder]",
	Aliases: []string{"index"},
	Short:   "Index a folder containing multiple git repositories",
	Long: `Walk the given folder (default: current directory), find every git
repository underneath it, and write an index of them to <folder>/.gg/index.json.

A directory is treated as a git repository when it contains a .git
subdirectory. Once a repo is found, its subtree is not descended into.

Invoked as 'gg index', the shell wrapper installation is skipped (the
same as passing --no-shell). Useful when the wrapper is already in place
and you just want to (re)index a folder.`,
	Args: cobra.MaximumNArgs(1),
	PreRun: func(cmd *cobra.Command, args []string) {
		if cmd.CalledAs() == "index" {
			initNoShell = true
		}
	},
	RunE: runInit,
}

func init() {
	rootCmd.AddCommand(initCmd)
	initCmd.Flags().IntVar(&initMaxDepth, "max-depth", defaultMaxDepth, "maximum depth to descend from the root folder")
	initCmd.Flags().StringVarP(&initOutput, "output", "o", "", "path to write the index file (default: <folder>/.gg/index.json)")
	initCmd.Flags().BoolVar(&initNoShell, "no-shell", false, "skip installing the gg shell wrapper into your shell config")
}

func runInit(cmd *cobra.Command, args []string) error {
	root := "."
	if len(args) == 1 {
		root = args[0]
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("resolve path: %w", err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return fmt.Errorf("stat %s: %w", abs, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", abs)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Indexing %s...\n", abs)

	out, err := resolveIndexPath(initOutput, abs)
	if err != nil {
		return err
	}
	count, err := indexRoot(abs, out, initMaxDepth)
	if err != nil {
		return err
	}

	if err := registerRoot(abs); err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: failed to update registry: %v\n", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Indexed %d repositories. Wrote %s\n", count, out)

	if !initNoShell {
		reportShellInstall(cmd)
	}
	return nil
}

func reportShellInstall(cmd *cobra.Command) {
	res, err := installShellWrapper()
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: shell setup failed: %v\n", err)
		return
	}
	switch res.Action {
	case ShellInstalled:
		fmt.Fprintf(cmd.OutOrStdout(), "Installed gg shell wrapper into %s — reload your shell to use 'gg jump'.\n", res.Path)
	case ShellUpdated:
		fmt.Fprintf(cmd.OutOrStdout(), "Updated gg shell wrapper in %s — reload your shell.\n", res.Path)
	case ShellUnchanged:
		// silent — already configured
	case ShellSkipped:
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: could not detect shell from $SHELL; skipping shell setup. Re-run with --no-shell to silence.\n")
	}
}

func resolveIndexPath(override, root string) (string, error) {
	if override != "" {
		return filepath.Abs(override)
	}
	return filepath.Join(root, ".gg", "index.json"), nil
}

func indexRoot(root, out string, maxDepth int) (int, error) {
	repos, err := findRepos(root, maxDepth)
	if err != nil {
		return 0, fmt.Errorf("find repos: %w", err)
	}

	paths := make([]string, len(repos))
	for i, r := range repos {
		paths[i] = r.Path
	}

	idx := Index{
		Root:      root,
		IndexedAt: time.Now(),
		StateHash: stateHashForPaths(paths),
		Repos:     repos,
	}

	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return 0, fmt.Errorf("mkdir %s: %w", filepath.Dir(out), err)
	}
	data, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return 0, fmt.Errorf("marshal index: %w", err)
	}
	if err := os.WriteFile(out, data, 0o644); err != nil {
		return 0, fmt.Errorf("write index: %w", err)
	}
	return len(repos), nil
}

func refreshAllIndexes(errOut io.Writer) {
	reg, err := loadRegistry()
	if err != nil {
		fmt.Fprintf(errOut, "warning: load registry: %v\n", err)
		return
	}
	for _, root := range reg.Roots {
		if _, err := os.Stat(root); err != nil {
			fmt.Fprintf(errOut, "warning: registered root unavailable (%s): %v\n", root, err)
			continue
		}
		if !rootChanged(root) {
			continue
		}
		out := filepath.Join(root, ".gg", "index.json")
		if _, err := indexRoot(root, out, defaultMaxDepth); err != nil {
			fmt.Fprintf(errOut, "warning: reindex %s: %v\n", root, err)
		}
	}
}

func rootChanged(root string) bool {
	idxPath := filepath.Join(root, ".gg", "index.json")
	data, err := os.ReadFile(idxPath)
	if err != nil {
		return true
	}
	var idx Index
	if err := json.Unmarshal(data, &idx); err != nil {
		return true
	}
	if idx.StateHash == "" {
		return true
	}
	paths, err := findRepoPaths(root, defaultMaxDepth)
	if err != nil {
		return true
	}
	return stateHashForPaths(paths) != idx.StateHash
}

func stateHashForPaths(paths []string) string {
	sorted := append([]string(nil), paths...)
	sort.Strings(sorted)
	h := sha256.New()
	for _, p := range sorted {
		gitDir := filepath.Join(p, ".git")
		if info, err := os.Stat(gitDir); err == nil {
			fmt.Fprintf(h, "%s|d|%d\n", p, info.ModTime().UnixNano())
		}
		for _, f := range []string{"HEAD", "index"} {
			if info, err := os.Stat(filepath.Join(gitDir, f)); err == nil {
				fmt.Fprintf(h, "%s|%s|%d\n", p, f, info.ModTime().UnixNano())
			}
		}
	}
	sum := h.Sum(nil)
	return hex.EncodeToString(sum[:8])
}

func findRepoPaths(root string, maxDepth int) ([]string, error) {
	var paths []string
	rootDepth := strings.Count(filepath.Clean(root), string(os.PathSeparator))
	sep := string(os.PathSeparator)

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if !d.IsDir() {
			return nil
		}

		depth := strings.Count(filepath.Clean(path), sep) - rootDepth
		if depth > maxDepth {
			return filepath.SkipDir
		}

		name := d.Name()
		if path != root {
			if name == "node_modules" || (strings.HasPrefix(name, ".") && name != "." && name != "..") {
				return filepath.SkipDir
			}
		}

		gitDir := filepath.Join(path, ".git")
		if st, err := os.Stat(gitDir); err == nil && st.IsDir() {
			paths = append(paths, path)
			return filepath.SkipDir
		}
		return nil
	})
	return paths, err
}

func findRepos(root string, maxDepth int) ([]Repo, error) {
	paths, err := findRepoPaths(root, maxDepth)
	if err != nil {
		return nil, err
	}

	repos := make([]Repo, len(paths))
	sem := make(chan struct{}, inspectConcurrency)
	var wg sync.WaitGroup
	for i, p := range paths {
		wg.Add(1)
		go func(i int, p string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			repos[i] = inspectRepo(p)
		}(i, p)
	}
	wg.Wait()
	return repos, nil
}

func inspectRepo(path string) Repo {
	return Repo{
		Name:      filepath.Base(path),
		Path:      path,
		Remote:    gitOutput(path, "config", "--get", "remote.origin.url"),
		Branch:    gitOutput(path, "rev-parse", "--abbrev-ref", "HEAD"),
		HeadSHA:   gitOutput(path, "rev-parse", "HEAD"),
		UpdatedAt: gitOutput(path, "log", "-1", "--format=%cI"),
		Diverged:  !isStale(path),
	}
}

func gitOutput(dir string, args ...string) string {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func gitOK(dir string, args ...string) bool {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	return cmd.Run() == nil
}
