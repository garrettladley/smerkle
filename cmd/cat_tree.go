package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/garrettladley/smerkle/internal/object"
	"github.com/spf13/cobra"
)

var (
	catTreeRecursive bool
	catTreeLong      bool
)

func newCatTreeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cat-tree <hash>",
		Short: "Display tree contents",
		Long:  `Display the contents of a tree object stored in the smerkle store.`,
		Args:  cobra.ExactArgs(1),
		RunE:  runCatTree,
	}

	cmd.Flags().BoolVarP(&catTreeRecursive, "recursive", "r", false, "show subtrees recursively")
	cmd.Flags().BoolVarP(&catTreeLong, "long", "l", false, "show size and mode details")

	return cmd
}

func runCatTree(cmd *cobra.Command, args []string) error {
	hash, err := parseHash(args[0])
	if err != nil {
		return err
	}

	s, err := openStore()
	if err != nil {
		return fmt.Errorf("failed to open store: %w", err)
	}
	defer func() { _ = s.Close() }()

	if outputJSON {
		return catTreeJSON(cmd, s, hash, "")
	}

	return catTreeText(cmd, s, hash, "", 0)
}

type treeGetter interface {
	GetTree(object.Hash) (*object.Tree, error)
}

func catTreeJSON(cmd *cobra.Command, s treeGetter, hash object.Hash, prefix string) error {
	tree, err := s.GetTree(hash)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("tree not found: %s", hash)
		}
		return fmt.Errorf("failed to get tree: %w", err)
	}

	type jsonEntry struct {
		Name    string      `json:"name"`
		Path    string      `json:"path,omitempty"`
		Mode    string      `json:"mode"`
		Size    int64       `json:"size,omitempty"`
		Hash    string      `json:"hash"`
		Entries []jsonEntry `json:"entries,omitempty"`
	}

	var buildEntries func(t *object.Tree, prefix string) ([]jsonEntry, error)
	buildEntries = func(t *object.Tree, prefix string) ([]jsonEntry, error) {
		entries := make([]jsonEntry, 0, len(t.Entries))
		for _, e := range t.Entries {
			path := e.Name
			if prefix != "" {
				path = prefix + "/" + e.Name
			}

			je := jsonEntry{
				Name: e.Name,
				Path: path,
				Mode: e.Mode.String(),
				Hash: e.Hash.String(),
			}
			if e.Mode.IsFile() {
				je.Size = e.Size
			}

			if catTreeRecursive && e.Mode == object.ModeDirectory {
				subTree, err := s.GetTree(e.Hash)
				if err != nil {
					return nil, fmt.Errorf("failed to get subtree %s: %w", e.Hash, err)
				}
				subEntries, err := buildEntries(subTree, path)
				if err != nil {
					return nil, err
				}
				je.Entries = subEntries
			}

			entries = append(entries, je)
		}
		return entries, nil
	}

	entries, err := buildEntries(tree, prefix)
	if err != nil {
		return err
	}

	output := map[string]any{
		"hash":    hash.String(),
		"entries": entries,
	}

	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	if err := enc.Encode(output); err != nil {
		return fmt.Errorf("encode json: %w", err)
	}
	return nil
}

func catTreeText(cmd *cobra.Command, s treeGetter, hash object.Hash, prefix string, depth int) error {
	tree, err := s.GetTree(hash)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("tree not found: %s", hash)
		}
		return fmt.Errorf("failed to get tree: %w", err)
	}

	for _, e := range tree.Entries {
		path := e.Name
		if prefix != "" {
			path = prefix + "/" + e.Name
		}

		if catTreeLong {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s %10d %s %s\n", modeCode(e.Mode), e.Size, e.Hash, path)
		} else {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s %s %s\n", modeCode(e.Mode), e.Hash, path)
		}

		if catTreeRecursive && e.Mode == object.ModeDirectory {
			if err := catTreeText(cmd, s, e.Hash, path, depth+1); err != nil {
				return err
			}
		}
	}

	return nil
}

func modeCode(m object.Mode) string {
	switch m {
	case object.ModeRegular:
		return "100644"
	case object.ModeExecutable:
		return "100755"
	case object.ModeDirectory:
		return "040000"
	case object.ModeSymlink:
		return "120000"
	default:
		return "000000"
	}
}
