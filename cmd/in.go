package cmd

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

var inAll bool

var inCmd = &cobra.Command{
	Use:   "in <repo> <command> [args...]",
	Short: "Run a command inside an indexed repository",
	Long: `Resolve a repository by name and run the given command with that repo
as the working directory. Stdin/stdout/stderr are wired through and the
command's exit code is returned.

  gg in bagel code .
  gg in bagel npm test
  gg in bagel ls -la

By default 'in' searches only the default index. Pass --all to search
every registered index instead. Flags after the repo name are passed to
the command, not to gg.

Matching: case-insensitive, exact name first, then substring.`,
	Args:              cobra.MinimumNArgs(2),
	SilenceUsage:      true,
	ValidArgsFunction: inComplete,
	RunE:              runIn,
}

func init() {
	rootCmd.AddCommand(inCmd)
	inCmd.Flags().BoolVar(&inAll, "all", false, "search across all registered indexes (ignore the default)")
	inCmd.Flags().SetInterspersed(false)
}

func runIn(cmd *cobra.Command, args []string) error {
	repo, err := resolveRepo(args[0], inAll)
	if err != nil {
		return err
	}

	proc := exec.Command(args[1], args[2:]...)
	proc.Dir = repo.Path
	proc.Stdin = os.Stdin
	proc.Stdout = os.Stdout
	proc.Stderr = os.Stderr

	if err := proc.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			os.Exit(ee.ExitCode())
		}
		return fmt.Errorf("run %s: %w", args[1], err)
	}
	return nil
}

func inComplete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) == 0 {
		return jumpComplete(cmd, args, toComplete)
	}
	return nil, cobra.ShellCompDirectiveDefault
}
