package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var newCmd = &cobra.Command{
	Use:   "new <name>",
	Short: "Create a new git repository in the default index and jump to it",
	Long: `Create a new directory in the default index's root, run 'git init',
refresh the index, and print the path so the shell wrapper can cd into it.

  gg new my-project

The name must be a single directory segment (no slashes, no '..'/'.').`,
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE:         runNew,
}

func init() {
	rootCmd.AddCommand(newCmd)
}

func runNew(cmd *cobra.Command, args []string) error {
	name := strings.TrimSpace(args[0])
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, `/\`) {
		return fmt.Errorf("invalid name %q: must be a single directory segment", args[0])
	}

	reg, err := loadRegistry()
	if err != nil {
		return err
	}
	root, _ := effectiveDefault(reg)
	if root == "" {
		return fmt.Errorf("no indexes registered — run 'gg init <folder>' first")
	}

	dest := filepath.Join(root, name)
	if _, err := os.Stat(dest); err == nil {
		return fmt.Errorf("%s already exists", dest)
	}

	if err := os.MkdirAll(dest, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", dest, err)
	}

	git := exec.Command("git", "init")
	git.Dir = dest
	git.Stdin = os.Stdin
	git.Stdout = cmd.ErrOrStderr()
	git.Stderr = cmd.ErrOrStderr()
	if err := git.Run(); err != nil {
		_ = os.Remove(dest)
		return fmt.Errorf("git init: %w", err)
	}

	refreshAllIndexes(cmd.ErrOrStderr())

	fmt.Fprintln(cmd.OutOrStdout(), dest)
	return nil
}
