package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"
)

func newInitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init [path]",
		Short: "Initialize a new smerkle store",
		Long:  `Initialize a new .smerkle store in the specified directory (defaults to current directory).`,
		Args:  cobra.MaximumNArgs(1),
		RunE:  runInit,
	}

	return cmd
}

func runInit(cmd *cobra.Command, args []string) error {
	path := "."
	if len(args) > 0 {
		path = args[0]
	}

	storePath := filepath.Join(path, storeDir)

	s, err := openStore()
	if err != nil {
		return fmt.Errorf("failed to initialize store: %w", err)
	}
	defer func() { _ = s.Close() }()

	if outputJSON {
		result := map[string]string{
			"store": storePath,
		}
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		if err := enc.Encode(result); err != nil {
			return fmt.Errorf("encode json: %w", err)
		}
		return nil
	}

	if !quiet {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Initialized smerkle store at %s\n", storePath)
	}

	return nil
}
