package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var cloneCmd = &cobra.Command{
	Use:   "clone <git-url> [name]",
	Short: "Clone a git URL into the default index and jump to it",
	Long: `Run 'git clone' inside the default index's root, refresh the index, and
print the path to the new repo so the shell wrapper can cd into it.

  gg clone https://github.com/foo/bar              # clones into <default>/bar
  gg clone git@github.com:foo/bar.git frontend     # clones into <default>/frontend`,
	Args:         cobra.RangeArgs(1, 2),
	SilenceUsage: true,
	RunE:         runClone,
}

func init() {
	rootCmd.AddCommand(cloneCmd)
}

func runClone(cmd *cobra.Command, args []string) error {
	url := args[0]

	reg, err := loadRegistry()
	if err != nil {
		return err
	}
	root, _ := effectiveDefault(reg)
	if root == "" {
		return fmt.Errorf("no indexes registered — run 'gg init <folder>' first")
	}

	var name string
	if len(args) == 2 {
		name = args[1]
	} else {
		name = inferRepoName(url)
		if name == "" {
			return fmt.Errorf("could not infer repo name from %q — pass an explicit name as the second arg", url)
		}
	}

	dest := filepath.Join(root, name)
	if _, err := os.Stat(dest); err == nil {
		return fmt.Errorf("%s already exists", dest)
	}

	git := exec.Command("git", "clone", url, name)
	git.Dir = root
	git.Stdin = os.Stdin
	git.Stdout = cmd.ErrOrStderr()
	git.Stderr = cmd.ErrOrStderr()
	if err := git.Run(); err != nil {
		return fmt.Errorf("git clone: %w", err)
	}

	refreshAllIndexes(cmd.ErrOrStderr())

	fmt.Fprintln(cmd.OutOrStdout(), dest)
	return nil
}

func inferRepoName(url string) string {
	s := strings.TrimRight(url, "/")
	s = strings.TrimSuffix(s, ".git")
	if i := strings.LastIndexAny(s, "/:"); i >= 0 {
		s = s[i+1:]
	}
	return s
}
