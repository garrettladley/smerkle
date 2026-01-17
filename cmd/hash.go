package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/garrettladley/smerkle/internal/ignore"
	"github.com/garrettladley/smerkle/internal/walker"
	"github.com/spf13/cobra"
)

var (
	hashConcurrency int
	hashIgnoreFile  string
	hashNoCache     bool
)

func newHashCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "hash [path]",
		Short: "Compute Merkle tree hash of a directory",
		Long:  `Recursively hash a directory and store the resulting Merkle tree in the store.`,
		Args:  cobra.MaximumNArgs(1),
		RunE:  runHash,
	}

	cmd.Flags().IntVarP(&hashConcurrency, "concurrency", "c", 0, "number of concurrent workers (0 = NumCPU)")
	cmd.Flags().StringVarP(&hashIgnoreFile, "ignore-file", "i", "", "custom ignore file path")
	cmd.Flags().BoolVar(&hashNoCache, "no-cache", false, "disable hash caching")

	return cmd
}

func runHash(cmd *cobra.Command, args []string) error {
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

	if hashConcurrency > 0 {
		opts = append(opts, walker.WithConcurrency(hashConcurrency))
	}

	if hashIgnoreFile != "" {
		ign, err := ignore.NewFromFile(hashIgnoreFile)
		if err != nil {
			return fmt.Errorf("failed to load ignore file: %w", err)
		}
		opts = append(opts, walker.WithIgnorer(ign))
	}

	result, err := walker.Walk(context.Background(), path, s, opts...)
	if err != nil {
		return fmt.Errorf("failed to walk directory: %w", err)
	}

	if outputJSON {
		output := map[string]any{
			"hash": result.Hash.String(),
		}
		if len(result.Errors) > 0 {
			errs := make([]string, len(result.Errors))
			for i, e := range result.Errors {
				errs[i] = e.Error()
			}
			output["errors"] = errs
		}
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		if err := enc.Encode(output); err != nil {
			return fmt.Errorf("encode json: %w", err)
		}
		return nil
	}

	_, _ = fmt.Fprintln(cmd.OutOrStdout(), result.Hash.String())

	if !quiet && len(result.Errors) > 0 {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "\nWarnings (%d errors encountered):\n", len(result.Errors))
		for _, e := range result.Errors {
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "  %s\n", e.Error())
		}
	}

	return nil
}
