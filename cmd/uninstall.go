package cmd

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var (
	uninstallAll    bool
	uninstallYes    bool
	uninstallDryRun bool
)

var uninstallCmd = &cobra.Command{
	Use:     "uninstall",
	Aliases: []string{"cleanup", "remove"},
	Short:   "Remove the gg shell setup (and optionally all data)",
	Long: `Remove the gg shell wrapper, completion, and shell-init snippet from your
shell config files. With --all, also remove the registry at
~/.gg/registry.json and every <root>/.gg/index.json file.

Does not uninstall the gg binary itself, remove ~/go/bin from PATH, or
modify fish_user_paths — those were set up outside of gg.`,
	Args:         cobra.NoArgs,
	SilenceUsage: true,
	RunE:         runUninstall,
}

func init() {
	rootCmd.AddCommand(uninstallCmd)
	uninstallCmd.Flags().BoolVar(&uninstallAll, "all", false, "also remove the registry and every per-root index file")
	uninstallCmd.Flags().BoolVarP(&uninstallYes, "yes", "y", false, "skip the confirmation prompt")
	uninstallCmd.Flags().BoolVar(&uninstallDryRun, "dry-run", false, "print what would be removed without doing it")
}

type uninstallTarget struct {
	desc   string
	remove func() error
}

func runUninstall(cmd *cobra.Command, args []string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	var targets []uninstallTarget

	for _, f := range []string{
		filepath.Join(home, ".config", "fish", "functions", "gg.fish"),
		filepath.Join(home, ".config", "fish", "completions", "gg.fish"),
		filepath.Join(home, ".config", "fish", "conf.d", "gg.fish"),
	} {
		if _, err := os.Stat(f); err == nil {
			path := f
			targets = append(targets, uninstallTarget{
				desc:   "remove " + path,
				remove: func() error { return os.Remove(path) },
			})
		}
	}

	for _, rc := range []string{
		filepath.Join(home, ".bashrc"),
		filepath.Join(home, ".zshrc"),
		filepath.Join(home, ".config", "fish", "config.fish"),
	} {
		data, err := os.ReadFile(rc)
		if err != nil || !bytes.Contains(data, []byte(shellMarkerBegin)) {
			continue
		}
		path := rc
		targets = append(targets, uninstallTarget{
			desc:   "remove gg block from " + path,
			remove: func() error { return removeShellBlockFromFile(path) },
		})
	}

	if uninstallAll {
		regPath := filepath.Join(home, ".gg", "registry.json")
		reg, _ := loadRegistry()
		if _, err := os.Stat(regPath); err == nil {
			targets = append(targets, uninstallTarget{
				desc: "remove " + regPath,
				remove: func() error {
					if err := os.Remove(regPath); err != nil {
						return err
					}
					_ = os.Remove(filepath.Dir(regPath))
					return nil
				},
			})
		}
		for _, root := range reg.Roots {
			idx := filepath.Join(root, ".gg", "index.json")
			if _, err := os.Stat(idx); err == nil {
				idxPath := idx
				targets = append(targets, uninstallTarget{
					desc: "remove " + idxPath,
					remove: func() error {
						if err := os.Remove(idxPath); err != nil {
							return err
						}
						_ = os.Remove(filepath.Dir(idxPath))
						return nil
					},
				})
			}
		}
	}

	if len(targets) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "Nothing to remove.")
		return nil
	}

	fmt.Fprintln(cmd.OutOrStdout(), "Will perform:")
	for _, t := range targets {
		fmt.Fprintln(cmd.OutOrStdout(), "  "+t.desc)
	}

	if uninstallDryRun {
		return nil
	}

	if !uninstallYes {
		fmt.Fprint(cmd.OutOrStdout(), "Proceed? [y/N] ")
		reader := bufio.NewReader(os.Stdin)
		resp, _ := reader.ReadString('\n')
		resp = strings.TrimSpace(strings.ToLower(resp))
		if resp != "y" && resp != "yes" {
			fmt.Fprintln(cmd.OutOrStdout(), "Aborted.")
			return nil
		}
	}

	for _, t := range targets {
		if err := t.remove(); err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s: %v\n", t.desc, err)
		}
	}

	fmt.Fprintln(cmd.OutOrStdout(), "Done. Reload your shell or open a new tab.")
	if !uninstallAll {
		fmt.Fprintln(cmd.OutOrStdout(), "Run with --all to also remove the registry and index files.")
	}
	return nil
}

func removeShellBlockFromFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	cleaned := stripShellBlock(data)
	if bytes.Equal(cleaned, data) {
		return nil
	}
	return os.WriteFile(path, cleaned, 0o644)
}
