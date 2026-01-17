package main

import (
	"github.com/spf13/cobra"
)

var (
	storeDir   string
	outputJSON bool
	quiet      bool
)

func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "smerkle",
		Short: "Merkle tree based directory hashing tool",
		Long: `smerkle computes Merkle tree hashes of directories, allowing you to
efficiently detect changes between directory snapshots.`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	cmd.PersistentFlags().StringVarP(&storeDir, "store", "s", ".smerkle", "path to the smerkle store directory")
	cmd.PersistentFlags().BoolVarP(&outputJSON, "json", "j", false, "output in JSON format")
	cmd.PersistentFlags().BoolVarP(&quiet, "quiet", "q", false, "suppress non-essential output")

	return cmd
}
