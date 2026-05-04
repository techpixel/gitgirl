/*
Copyright © 2026 NAME HERE <EMAIL ADDRESS>

*/
package cmd

import (
	"os"

	"github.com/spf13/cobra"
)



// rootCmd represents the base command when called without any subcommands
var skipIndex bool

var rootCmd = &cobra.Command{
	Use:   "gg",
	Short: "gitgirl - manage many git repositories at once",
	Long:  `gitgirl (gg) indexes and operates over folders that contain many git repositories.`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if skipIndex {
			return nil
		}
		switch cmd.Name() {
		case "init", "deindex", "default", "completion", "help", "__complete", "__completeNoDesc":
			return nil
		}
		refreshAllIndexes(cmd.ErrOrStderr())
		return nil
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().BoolVar(&skipIndex, "no-index", false, "skip auto-reindexing before this command")
}


