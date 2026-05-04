package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var deindexCmd = &cobra.Command{
	Use:   "deindex [folder]",
	Short: "Remove a folder from the gg registry",
	Long: `Remove a folder from ~/.gg/registry.json and delete its
<folder>/.gg/index.json. The folder itself and its repos are not touched.

If no folder is given, the current directory is used.`,
	Args:              cobra.MaximumNArgs(1),
	SilenceUsage:      true,
	ValidArgsFunction: deindexComplete,
	RunE:              runDeindex,
}

func init() {
	rootCmd.AddCommand(deindexCmd)
}

func runDeindex(cmd *cobra.Command, args []string) error {
	target, err := resolveDeindexTarget(args)
	if err != nil {
		return err
	}

	reg, err := loadRegistry()
	if err != nil {
		return err
	}

	kept := reg.Roots[:0]
	found := false
	for _, r := range reg.Roots {
		if r == target {
			found = true
			continue
		}
		kept = append(kept, r)
	}
	if !found {
		return fmt.Errorf("%s is not a registered index — run 'gg list --index' to see registered roots", target)
	}
	reg.Roots = kept
	if reg.Default == target {
		reg.Default = ""
	}
	if err := saveRegistry(reg); err != nil {
		return fmt.Errorf("save registry: %w", err)
	}

	idxFile := filepath.Join(target, ".gg", "index.json")
	switch err := os.Remove(idxFile); {
	case err == nil:
		os.Remove(filepath.Dir(idxFile))
	case !os.IsNotExist(err):
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: remove %s: %v\n", idxFile, err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Deindexed %s\n", target)
	return nil
}

func resolveDeindexTarget(args []string) (string, error) {
	if len(args) == 1 {
		return filepath.Abs(args[0])
	}
	return os.Getwd()
}

func deindexComplete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	reg, err := loadRegistry()
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	return reg.Roots, cobra.ShellCompDirectiveNoFileComp
}
