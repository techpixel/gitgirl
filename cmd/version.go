package cmd

import (
	"runtime/debug"
)

// Version is the gg version string. Override with:
//
//	go build -ldflags "-X gitgirl/cmd.Version=v0.1.0"
var Version = "dev"

func init() {
	rootCmd.Version = versionString()
	rootCmd.SetVersionTemplate("gg {{.Version}}\n")
}

func versionString() string {
	v := Version
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return v
	}
	for _, s := range info.Settings {
		if s.Key == "vcs.revision" && len(s.Value) >= 7 {
			return v + " (" + s.Value[:7] + ")"
		}
	}
	return v
}
