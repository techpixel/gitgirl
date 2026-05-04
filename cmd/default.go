package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"
)

var defaultUnset bool

var defaultCmd = &cobra.Command{
	Use:   "default [path]",
	Short: "Show or set the default index used by gg jump",
	Long: `With no arguments, print the current default index that 'gg jump' searches.
If no default has been set explicitly, the first registered index (the
"original" one) is used implicitly and shown with an "(implicit)" tag.

With a path argument, set the default index. The path must be a registered
root — see 'gg list --index'.

With --unset, clear an explicit default and revert to the implicit one.`,
	Args:              cobra.MaximumNArgs(1),
	SilenceUsage:      true,
	ValidArgsFunction: defaultComplete,
	RunE:              runDefault,
}

func init() {
	rootCmd.AddCommand(defaultCmd)
	defaultCmd.Flags().BoolVar(&defaultUnset, "unset", false, "clear an explicit default and revert to the original index")
}

func runDefault(cmd *cobra.Command, args []string) error {
	reg, err := loadRegistry()
	if err != nil {
		return err
	}

	if defaultUnset {
		if len(args) > 0 {
			return fmt.Errorf("--unset takes no path")
		}
		if reg.Default == "" {
			fmt.Fprintln(cmd.OutOrStdout(), "no explicit default set")
			return nil
		}
		reg.Default = ""
		if err := saveRegistry(reg); err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "Cleared default index")
		return nil
	}

	if len(args) == 0 {
		root, explicit := effectiveDefault(reg)
		if root == "" {
			fmt.Fprintln(cmd.OutOrStdout(), "no indexes registered")
			return nil
		}
		if explicit {
			fmt.Fprintln(cmd.OutOrStdout(), root)
		} else {
			fmt.Fprintf(cmd.OutOrStdout(), "%s (implicit)\n", root)
		}
		return nil
	}

	abs, err := filepath.Abs(args[0])
	if err != nil {
		return fmt.Errorf("resolve path: %w", err)
	}
	registered := false
	for _, r := range reg.Roots {
		if r == abs {
			registered = true
			break
		}
	}
	if !registered {
		return fmt.Errorf("%s is not a registered index — run 'gg init %q' first", abs, abs)
	}
	reg.Default = abs
	if err := saveRegistry(reg); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Set default index to %s\n", abs)
	return nil
}

func defaultComplete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	reg, err := loadRegistry()
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	return reg.Roots, cobra.ShellCompDirectiveNoFileComp
}
