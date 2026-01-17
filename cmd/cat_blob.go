package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"unicode/utf8"

	"github.com/spf13/cobra"
)

func newCatBlobCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cat-blob <hash>",
		Short: "Display blob contents",
		Long:  `Display the contents of a blob object stored in the smerkle store.`,
		Args:  cobra.ExactArgs(1),
		RunE:  runCatBlob,
	}

	return cmd
}

func runCatBlob(cmd *cobra.Command, args []string) error {
	hash, err := parseHash(args[0])
	if err != nil {
		return err
	}

	s, err := openStore()
	if err != nil {
		return fmt.Errorf("failed to open store: %w", err)
	}
	defer func() { _ = s.Close() }()

	blob, err := s.GetBlob(hash)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("blob not found: %s", hash)
		}
		return fmt.Errorf("failed to get blob: %w", err)
	}

	if outputJSON {
		output := map[string]any{
			"hash": hash.String(),
			"size": len(blob.Content),
		}

		if utf8.Valid(blob.Content) {
			output["content"] = string(blob.Content)
			output["encoding"] = "utf8"
		} else {
			output["content"] = base64.StdEncoding.EncodeToString(blob.Content)
			output["encoding"] = "base64"
		}

		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		if err := enc.Encode(output); err != nil {
			return fmt.Errorf("encode json: %w", err)
		}
		return nil
	}

	if _, err := cmd.OutOrStdout().Write(blob.Content); err != nil {
		return fmt.Errorf("write output: %w", err)
	}
	return nil
}
