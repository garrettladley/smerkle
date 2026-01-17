package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/garrettladley/smerkle/internal/diff"
	"github.com/garrettladley/smerkle/internal/walker"
	"github.com/spf13/cobra"
)

var (
	statusBase        string
	statusConcurrency int
)

func newStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status [path]",
		Short: "Show changes since baseline",
		Long:  `Compute the current directory hash and compare it against a baseline hash to show changes.`,
		Args:  cobra.MaximumNArgs(1),
		RunE:  runStatus,
	}

	cmd.Flags().StringVarP(&statusBase, "base", "b", "", "baseline hash to compare against (required)")
	_ = cmd.MarkFlagRequired("base")
	cmd.Flags().IntVarP(&statusConcurrency, "concurrency", "c", 0, "number of concurrent workers (0 = NumCPU)")

	return cmd
}

func runStatus(cmd *cobra.Command, args []string) error {
	baseHash, err := parseHash(statusBase)
	if err != nil {
		return fmt.Errorf("invalid base hash: %w", err)
	}

	path := "."
	if len(args) > 0 {
		path = args[0]
	}

	s, err := openStore()
	if err != nil {
		return fmt.Errorf("failed to open store: %w", err)
	}
	defer func() { _ = s.Close() }()

	var opts []walker.Option
	if statusConcurrency > 0 {
		opts = append(opts, walker.WithConcurrency(statusConcurrency))
	}

	result, err := walker.Walk(context.Background(), path, s, opts...)
	if err != nil {
		return fmt.Errorf("failed to walk directory: %w", err)
	}

	currentHash := result.Hash

	if currentHash == baseHash {
		if outputJSON {
			output := map[string]any{
				"base_hash":    baseHash.String(),
				"current_hash": currentHash.String(),
				"changed":      false,
				"changes":      []any{},
			}
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			if err := enc.Encode(output); err != nil {
				return fmt.Errorf("encode json: %w", err)
			}
			return nil
		}

		if !quiet {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "No changes")
		}
		return nil
	}

	diffResult, err := diff.DiffDefault(s, baseHash, currentHash)
	if err != nil {
		return fmt.Errorf("failed to diff: %w", err)
	}

	if outputJSON {
		return outputStatusJSON(cmd, baseHash, currentHash, diffResult.Changes)
	}

	outputStatusText(cmd, diffResult.Changes)
	return nil
}

func outputStatusJSON(cmd *cobra.Command, baseHash, currentHash interface{ String() string }, changes []diff.Change) error {
	type jsonChange struct {
		Type    string `json:"type"`
		Path    string `json:"path"`
		OldHash string `json:"old_hash,omitempty"`
		NewHash string `json:"new_hash,omitempty"`
	}

	jsonChanges := make([]jsonChange, 0, len(changes))
	for _, c := range changes {
		jc := jsonChange{
			Type: c.Type.String(),
			Path: c.Path,
		}
		if c.OldEntry != nil {
			jc.OldHash = c.OldEntry.Hash.String()
		}
		if c.NewEntry != nil {
			jc.NewHash = c.NewEntry.Hash.String()
		}
		jsonChanges = append(jsonChanges, jc)
	}

	output := map[string]any{
		"base_hash":    baseHash.String(),
		"current_hash": currentHash.String(),
		"changed":      len(jsonChanges) > 0,
		"changes":      jsonChanges,
	}

	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	if err := enc.Encode(output); err != nil {
		return fmt.Errorf("encode json: %w", err)
	}
	return nil
}

func outputStatusText(cmd *cobra.Command, changes []diff.Change) {
	for _, c := range changes {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\n", changeTypePrefix(c.Type), c.Path)
	}
}
