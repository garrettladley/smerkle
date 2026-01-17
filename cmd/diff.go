package main

import (
	"encoding/json"
	"fmt"

	"github.com/garrettladley/smerkle/internal/diff"
	"github.com/spf13/cobra"
)

var (
	diffNoRecursive bool
	diffTypeFilter  string
	diffNameOnly    bool
)

func newDiffCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "diff <old-hash> <new-hash>",
		Short: "Compare two stored tree hashes",
		Long:  `Compare two tree hashes stored in the smerkle store and show the differences.`,
		Args:  cobra.ExactArgs(2),
		RunE:  runDiff,
	}

	cmd.Flags().BoolVar(&diffNoRecursive, "no-recursive", false, "only compare top-level entries")
	cmd.Flags().StringVarP(&diffTypeFilter, "type", "t", "", "filter by change type: added, deleted, modified, type_change")
	cmd.Flags().BoolVar(&diffNameOnly, "name-only", false, "only show paths, no change type prefix")

	return cmd
}

func runDiff(cmd *cobra.Command, args []string) error {
	oldHash, err := parseHash(args[0])
	if err != nil {
		return fmt.Errorf("invalid old hash: %w", err)
	}

	newHash, err := parseHash(args[1])
	if err != nil {
		return fmt.Errorf("invalid new hash: %w", err)
	}

	s, err := openStore()
	if err != nil {
		return fmt.Errorf("failed to open store: %w", err)
	}
	defer func() { _ = s.Close() }()

	opts := diff.Options{
		Recursive: !diffNoRecursive,
	}

	result, err := diff.Diff(s, oldHash, newHash, opts)
	if err != nil {
		return fmt.Errorf("failed to diff trees: %w", err)
	}

	changes := filterChanges(result.Changes)

	if outputJSON {
		return outputDiffJSON(cmd, changes)
	}

	outputDiffText(cmd, changes)
	return nil
}

func filterChanges(changes []diff.Change) []diff.Change {
	if diffTypeFilter == "" {
		return changes
	}

	var ct diff.ChangeType
	switch diffTypeFilter {
	case "added":
		ct = diff.ChangeAdded
	case "deleted":
		ct = diff.ChangeDeleted
	case "modified":
		ct = diff.ChangeModified
	case "type_change":
		ct = diff.ChangeTypeChange
	default:
		return changes
	}

	var filtered []diff.Change
	for _, c := range changes {
		if c.Type == ct {
			filtered = append(filtered, c)
		}
	}
	return filtered
}

func outputDiffJSON(cmd *cobra.Command, changes []diff.Change) error {
	type jsonChange struct {
		Type    string `json:"type"`
		Path    string `json:"path"`
		OldHash string `json:"old_hash,omitempty"`
		NewHash string `json:"new_hash,omitempty"`
		OldMode string `json:"old_mode,omitempty"`
		NewMode string `json:"new_mode,omitempty"`
	}

	jsonChanges := make([]jsonChange, 0, len(changes))
	for _, c := range changes {
		jc := jsonChange{
			Type: c.Type.String(),
			Path: c.Path,
		}
		if c.OldEntry != nil {
			jc.OldHash = c.OldEntry.Hash.String()
			jc.OldMode = c.OldEntry.Mode.String()
		}
		if c.NewEntry != nil {
			jc.NewHash = c.NewEntry.Hash.String()
			jc.NewMode = c.NewEntry.Mode.String()
		}
		jsonChanges = append(jsonChanges, jc)
	}

	output := map[string]any{
		"changes": jsonChanges,
		"count":   len(jsonChanges),
	}

	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	if err := enc.Encode(output); err != nil {
		return fmt.Errorf("encode json: %w", err)
	}
	return nil
}

func outputDiffText(cmd *cobra.Command, changes []diff.Change) {
	for _, c := range changes {
		if diffNameOnly {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), c.Path)
		} else {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\n", changeTypePrefix(c.Type), c.Path)
		}
	}
}
