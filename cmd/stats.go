package main

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

func newStatsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stats",
		Short: "Show store statistics",
		Long:  `Display statistics about the smerkle store including object count and index size.`,
		Args:  cobra.NoArgs,
		RunE:  runStats,
	}

	return cmd
}

func runStats(cmd *cobra.Command, args []string) error {
	s, err := openStore()
	if err != nil {
		return fmt.Errorf("failed to open store: %w", err)
	}
	defer func() { _ = s.Close() }()

	stats := s.Stats()

	if outputJSON {
		output := map[string]int{
			"objects": stats.ObjectCount,
			"index":   stats.IndexSize,
		}
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		if err := enc.Encode(output); err != nil {
			return fmt.Errorf("encode json: %w", err)
		}
		return nil
	}

	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Objects: %d\n", stats.ObjectCount)
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Index:   %d\n", stats.IndexSize)

	return nil
}
