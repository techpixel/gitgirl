package cmd

import (
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
)

const refreshStaleThreshold = 60 * time.Second

var refreshIfStale bool

var refreshCmd = &cobra.Command{
	Use:   "refresh",
	Short: "Refresh all registered indexes",
	Long: `Re-scan every registered root and rewrite its <root>/.gg/index.json.

With --if-stale, skip the work if every index file was updated within
the last 60 seconds. The shell-init background invocation uses this so
that opening many shells in quick succession doesn't pile up redundant
reindex jobs.`,
	Args:         cobra.NoArgs,
	SilenceUsage: true,
	RunE:         runRefresh,
}

func init() {
	rootCmd.AddCommand(refreshCmd)
	refreshCmd.Flags().BoolVar(&refreshIfStale, "if-stale", false, "skip if every index was refreshed within the last minute")
}

func runRefresh(cmd *cobra.Command, args []string) error {
	if refreshIfStale && !indexesAreStale(refreshStaleThreshold) {
		return nil
	}
	refreshAllIndexes(cmd.ErrOrStderr())
	return nil
}

func indexesAreStale(threshold time.Duration) bool {
	reg, err := loadRegistry()
	if err != nil {
		return true
	}
	if len(reg.Roots) == 0 {
		return false
	}
	for _, root := range reg.Roots {
		info, err := os.Stat(filepath.Join(root, ".gg", "index.json"))
		if err != nil {
			return true
		}
		if time.Since(info.ModTime()) > threshold {
			return true
		}
	}
	return false
}
