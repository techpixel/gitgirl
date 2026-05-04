package cmd

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

var jumpAll bool

var jumpCmd = &cobra.Command{
	Use:   "jump [repo]",
	Short: "Print the path of an indexed repository (or the default index root)",
	Long: `Print the absolute path of a repository previously indexed by 'gg init'.

The wrapper installed by 'gg init' captures this output and runs 'cd'
in your current shell. Without the wrapper the path is printed to stdout.

With no argument, prints the default index's root path (see 'gg default').

With a repo name, jump searches only the default index by default. Pass
--all to search every registered index instead.

Matching: case-insensitive, exact name first, then substring.`,
	Args:              cobra.MaximumNArgs(1),
	ValidArgsFunction: jumpComplete,
	SilenceUsage:      true,
	RunE:              runJump,
}

func init() {
	rootCmd.AddCommand(jumpCmd)
	jumpCmd.Flags().BoolVar(&jumpAll, "all", false, "search across all registered indexes (ignore the default)")
}

func runJump(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		reg, err := loadRegistry()
		if err != nil {
			return err
		}
		root, _ := effectiveDefault(reg)
		if root == "" {
			return fmt.Errorf("no indexes registered — run 'gg init <folder>' first")
		}
		fmt.Fprintln(cmd.OutOrStdout(), root)
		return nil
	}

	query := strings.ToLower(args[0])
	var repos []Repo
	var err error
	if jumpAll {
		repos, err = loadAllRepos()
	} else {
		repos, err = loadDefaultRepos()
	}
	if err != nil {
		return err
	}
	if len(repos) == 0 {
		return fmt.Errorf("no repositories indexed yet — run 'gg init <folder>' first")
	}

	var exact, sub []Repo
	for _, r := range repos {
		name := strings.ToLower(r.Name)
		switch {
		case name == query:
			exact = append(exact, r)
		case strings.Contains(name, query):
			sub = append(sub, r)
		}
	}

	matches := exact
	if len(matches) == 0 {
		matches = sub
	}

	switch len(matches) {
	case 0:
		return fmt.Errorf("no indexed repository matches %q", args[0])
	case 1:
		fmt.Fprintln(cmd.OutOrStdout(), matches[0].Path)
		return nil
	default:
		var b strings.Builder
		fmt.Fprintf(&b, "multiple repositories match %q:\n", args[0])
		for _, m := range matches {
			fmt.Fprintf(&b, "  %s\n", m.Path)
		}
		return fmt.Errorf("%s", strings.TrimRight(b.String(), "\n"))
	}
}

func jumpComplete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	repos, err := loadAllRepos()
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(repos))
	for _, r := range repos {
		if seen[r.Name] {
			continue
		}
		seen[r.Name] = true
		out = append(out, r.Name)
	}
	sort.Strings(out)
	return out, cobra.ShellCompDirectiveNoFileComp
}
