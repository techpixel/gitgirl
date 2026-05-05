package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

const staleCheckConcurrency = 16

var (
	listStale bool
	listIndex bool
)

var listCmd = &cobra.Command{
	Use:     "list [pattern]",
	Aliases: []string{"ls"},
	Short:   "List indexed repositories",
	Long: `List indexed repositories. With a positional pattern, only show repos
whose name or path contains the pattern (case-insensitive).

With --stale, only show repos whose local copy carries nothing the
remote doesn't already have: clean working tree, on the default branch
(origin/HEAD, falling back to main then master), and not ahead of
origin/<default>.

With --index, list registered index roots instead of repos. The
pattern, if given, filters root paths.`,
	Args:         cobra.MaximumNArgs(1),
	SilenceUsage: true,
	RunE:         runList,
}

func init() {
	rootCmd.AddCommand(listCmd)
	listCmd.Flags().BoolVar(&listStale, "stale", false, "only repos whose local copy carries no unique work vs. origin")
	listCmd.Flags().BoolVar(&listIndex, "index", false, "list registered indexes (roots) instead of repos")
	listCmd.MarkFlagsMutuallyExclusive("index", "stale")
}

func runList(cmd *cobra.Command, args []string) error {
	refreshAllIndexes(cmd.ErrOrStderr())

	if listIndex {
		return runListIndex(cmd, args)
	}

	repos, err := loadAllRepos()
	if err != nil {
		return err
	}

	if len(args) == 1 {
		repos = filterByPattern(repos, args[0])
	}
	if listStale {
		repos = filterStale(repos)
	}

	return printRepoTable(cmd.OutOrStdout(), repos)
}

func runListIndex(cmd *cobra.Command, args []string) error {
	reg, err := loadRegistry()
	if err != nil {
		return err
	}

	pattern := ""
	if len(args) == 1 {
		pattern = strings.ToLower(args[0])
	}

	w := cmd.OutOrStdout()
	defaultRoot, defaultExplicit := effectiveDefault(reg)

	type row struct {
		root, repos, when string
		isDefault         bool
		explicit          bool
	}
	var rows []row
	for _, r := range reg.Roots {
		if pattern != "" && !strings.Contains(strings.ToLower(r), pattern) {
			continue
		}
		repos, when := readIndexMeta(r)
		rows = append(rows, row{r, repos, when, r == defaultRoot, defaultExplicit})
	}
	if len(rows) == 0 {
		fmt.Fprintln(w, "no indexes")
		return nil
	}

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ROOT\tREPOS\tLAST INDEXED")
	for _, r := range rows {
		label := r.root
		if r.isDefault {
			if r.explicit {
				label += " (default)"
			} else {
				label += " (default, implicit)"
			}
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\n", label, r.repos, r.when)
	}
	return tw.Flush()
}

func readIndexMeta(root string) (repos, when string) {
	data, err := os.ReadFile(filepath.Join(root, ".gg", "index.json"))
	if err != nil {
		return "-", "-"
	}
	var idx Index
	if err := json.Unmarshal(data, &idx); err != nil {
		return "-", "-"
	}
	return fmt.Sprintf("%d", len(idx.Repos)), idx.IndexedAt.Format("2006-01-02 15:04:05")
}

func filterByPattern(repos []Repo, pattern string) []Repo {
	q := strings.ToLower(pattern)
	out := make([]Repo, 0, len(repos))
	for _, r := range repos {
		if strings.Contains(strings.ToLower(r.Name), q) ||
			strings.Contains(strings.ToLower(r.Path), q) {
			out = append(out, r)
		}
	}
	return out
}

func filterStale(repos []Repo) []Repo {
	stale := make([]bool, len(repos))
	sem := make(chan struct{}, staleCheckConcurrency)
	var wg sync.WaitGroup
	for i, r := range repos {
		wg.Add(1)
		go func(i int, r Repo) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			stale[i] = isStale(r.Path)
		}(i, r)
	}
	wg.Wait()

	out := make([]Repo, 0, len(repos))
	for i, r := range repos {
		if stale[i] {
			out = append(out, r)
		}
	}
	return out
}

func isStale(path string) bool {
	if gitOutput(path, "status", "--porcelain") != "" {
		return false
	}
	branch := gitOutput(path, "rev-parse", "--abbrev-ref", "HEAD")
	def := defaultBranch(path)
	if branch == "" || def == "" || branch != def {
		return false
	}
	if gitOK(path, "rev-parse", "--verify", "--quiet", "origin/"+def) {
		ahead := gitOutput(path, "rev-list", "--count", "origin/"+def+"..HEAD")
		if ahead != "0" {
			return false
		}
	}
	return true
}

func defaultBranch(path string) string {
	if ref := gitOutput(path, "symbolic-ref", "--short", "refs/remotes/origin/HEAD"); ref != "" {
		if i := strings.Index(ref, "/"); i >= 0 {
			return ref[i+1:]
		}
		return ref
	}
	if gitOK(path, "rev-parse", "--verify", "--quiet", "main") {
		return "main"
	}
	if gitOK(path, "rev-parse", "--verify", "--quiet", "master") {
		return "master"
	}
	return ""
}

func printRepoTable(w io.Writer, repos []Repo) error {
	if len(repos) == 0 {
		fmt.Fprintln(w, "no repositories")
		return nil
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, " \tNAME\tBRANCH\tPATH")
	for _, r := range repos {
		b := r.Branch
		if b == "" {
			b = "-"
		}
		marker := " "
		if r.Diverged {
			marker = "*"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", marker, r.Name, b, r.Path)
	}
	return tw.Flush()
}
