package main

import (
	"context"
	"os"
	"syscall"

	"github.com/charmbracelet/fang"
)

var version = "dev"

func main() {
	rootCmd := newRootCmd()
	rootCmd.AddCommand(
		newInitCmd(),
		newHashCmd(),
		newStatusCmd(),
		newDiffCmd(),
		newCatTreeCmd(),
		newCatBlobCmd(),
		newStatsCmd(),
	)

	if err := fang.Execute(context.Background(), rootCmd,
		fang.WithVersion(version),
		fang.WithColorSchemeFunc(fang.AnsiColorScheme),
		fang.WithNotifySignal(os.Interrupt, syscall.SIGTERM),
	); err != nil {
		os.Exit(1)
	}
}
